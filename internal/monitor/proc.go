package monitor

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProcSample struct {
	PID      int
	PPID     int
	Ticks    uint64
	RSSBytes uint64
}

type Aggregate struct {
	CPU      float64
	MemoryMB float64
	PIDs     []int
	Ports    []int
}

type Sampler struct {
	procRoot   string
	cgroupRoot string
	previous   map[int]ProcSample
	previousAt time.Time
}

func NewSampler() *Sampler {
	return &Sampler{
		procRoot:   "/proc",
		cgroupRoot: "/sys/fs/cgroup",
		previous:   map[int]ProcSample{},
	}
}

func (s *Sampler) Sample(rootPID int, includePorts bool) (Aggregate, error) {
	now := time.Now()
	all, err := ReadProcesses(s.procRoot)
	if err != nil {
		return Aggregate{}, err
	}
	pids := Descendants(all, rootPID)
	var ticks, rssBytes uint64
	for _, pid := range pids {
		sample := all[pid]
		ticks += sample.Ticks
		rssBytes += sample.RSSBytes
	}
	memoryBytes := rssBytes
	if current, currentErr := readCgroupMemory(s.procRoot, s.cgroupRoot, rootPID); currentErr == nil {
		memoryBytes = current
	} else if pss, pssErr := readProcessTreePSS(s.procRoot, pids); pssErr == nil {
		memoryBytes = pss
	}
	var previousTicks uint64
	for _, pid := range pids {
		previousTicks += s.previous[pid].Ticks
	}
	cpu := 0.0
	if !s.previousAt.IsZero() && ticks >= previousTicks {
		seconds := now.Sub(s.previousAt).Seconds()
		if seconds > 0 {
			cpu = float64(ticks-previousTicks) / 100.0 / seconds * 100.0
		}
	}
	s.previous, s.previousAt = all, now
	result := Aggregate{CPU: cpu, MemoryMB: float64(memoryBytes) / (1024 * 1024), PIDs: pids}
	if includePorts {
		result.Ports, _ = ListeningPorts(s.procRoot, pids)
	}
	return result, nil
}

// readCgroupMemory returns the current memory charged to the cgroup containing
// pid. On cgroup v2 this accounts shared pages once at the service boundary,
// unlike summing the RSS of every process in a Chromium-style process tree.
func readCgroupMemory(procRoot, cgroupRoot string, pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return 0, err
	}
	cgroupPath, err := parseCgroupV2Path(string(data))
	if err != nil {
		return 0, err
	}
	relative := strings.TrimPrefix(cgroupPath, "/")
	data, err = os.ReadFile(filepath.Join(cgroupRoot, filepath.FromSlash(relative), "memory.current"))
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse cgroup memory.current: %w", err)
	}
	return value, nil
}

func parseCgroupV2Path(content string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" {
			continue
		}
		path := parts[2]
		if !strings.HasPrefix(path, "/") || filepath.Clean(path) != path {
			return "", errors.New("invalid cgroup v2 path")
		}
		return path, nil
	}
	return "", errors.New("cgroup v2 entry not found")
}

// readProcessTreePSS is the compatibility fallback when cgroup-v2 accounting
// is unavailable. PSS apportions shared pages between processes instead of
// charging the complete shared mapping to every process as RSS does.
func readProcessTreePSS(procRoot string, pids []int) (uint64, error) {
	if len(pids) == 0 {
		return 0, errors.New("empty process tree")
	}
	var total uint64
	for _, pid := range pids {
		bytes, err := readProcessPSS(procRoot, pid)
		if err != nil {
			return 0, err
		}
		if ^uint64(0)-total < bytes {
			return 0, errors.New("process tree PSS overflow")
		}
		total += bytes
	}
	return total, nil
}

func readProcessPSS(procRoot string, pid int) (uint64, error) {
	file, err := os.Open(filepath.Join(procRoot, strconv.Itoa(pid), "smaps_rollup"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "Pss:" {
			continue
		}
		kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse process %d PSS: %w", pid, err)
		}
		if kilobytes > ^uint64(0)/1024 {
			return 0, fmt.Errorf("process %d PSS overflow", pid)
		}
		return kilobytes * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("process %d PSS not found", pid)
}

func ReadProcesses(procRoot string) (map[int]ProcSample, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	result := map[int]ProcSample{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			continue
		}
		sample, err := ParseProcStat(pid, string(stat), os.Getpagesize())
		if err == nil {
			result[pid] = sample
		}
	}
	return result, nil
}

func ParseProcStat(pid int, line string, pageSize int) (ProcSample, error) {
	closeAt := strings.LastIndex(line, ")")
	if closeAt < 0 || closeAt+2 >= len(line) {
		return ProcSample{}, errors.New("malformed proc stat")
	}
	fields := strings.Fields(line[closeAt+2:])
	if len(fields) < 22 {
		return ProcSample{}, errors.New("short proc stat")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return ProcSample{}, err
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return ProcSample{}, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return ProcSample{}, err
	}
	rssPages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return ProcSample{}, err
	}
	if rssPages < 0 {
		rssPages = 0
	}
	return ProcSample{PID: pid, PPID: ppid, Ticks: utime + stime, RSSBytes: uint64(rssPages) * uint64(pageSize)}, nil
}

func Descendants(all map[int]ProcSample, root int) []int {
	result := []int{}
	queue := []int{root}
	seen := map[int]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		if _, ok := all[pid]; !ok {
			continue
		}
		result = append(result, pid)
		for candidate, sample := range all {
			if sample.PPID == pid {
				queue = append(queue, candidate)
			}
		}
	}
	sort.Ints(result)
	return result
}

func ListeningPorts(procRoot string, pids []int) ([]int, error) {
	inodes := map[string]bool{}
	for _, pid := range pids {
		entries, err := os.ReadDir(filepath.Join(procRoot, strconv.Itoa(pid), "fd"))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			target, err := os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "fd", entry.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
				inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
			}
		}
	}
	ports := map[int]bool{}
	for _, name := range []string{"tcp", "tcp6"} {
		file, err := os.Open(filepath.Join(procRoot, "net", name))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			port, inode, listening, ok := ParseTCPLine(scanner.Text())
			if ok && listening && inodes[inode] {
				ports[port] = true
			}
		}
		file.Close()
	}
	result := make([]int, 0, len(ports))
	for port := range ports {
		result = append(result, port)
	}
	sort.Ints(result)
	if len(result) > 256 {
		result = result[:256]
	}
	return result, nil
}

func ParseTCPLine(line string) (port int, inode string, listening bool, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 || fields[0] == "sl" {
		return 0, "", false, false
	}
	_, portHex, found := strings.Cut(fields[1], ":")
	if !found {
		return 0, "", false, false
	}
	parsed, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return 0, "", false, false
	}
	return int(parsed), fields[9], fields[3] == "0A", true
}
