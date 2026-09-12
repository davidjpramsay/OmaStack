package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProcStatHandlesSpacesInCommand(t *testing.T) {
	line := "42 (worker with spaces) S 1 0 0 0 0 0 0 0 0 0 10 5 0 0 0 0 0 0 0 0 25"
	sample, err := ParseProcStat(42, line, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if sample.PPID != 1 || sample.Ticks != 15 || sample.RSSBytes != 25*4096 {
		t.Fatalf("sample=%#v", sample)
	}
}

func TestListeningPortsFromSyntheticProc(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "123", "fd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[98765]", filepath.Join(root, "123", "fd", "4")); err != nil {
		t.Fatal(err)
	}
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n   0: 0100007F:0BB8 00000000:0000 0A 0:0 0:0 00000000 1000 0 98765\n"
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ports, err := ListeningPorts(root, []int{123})
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0] != 3000 {
		t.Fatalf("ports=%v", ports)
	}
}

func TestReadCgroupMemory(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(root, "proc")
	cgroupRoot := filepath.Join(root, "cgroup")
	pidDirectory := filepath.Join(procRoot, "123")
	memoryDirectory := filepath.Join(cgroupRoot, "user.slice", "omastack-service.scope")
	for _, directory := range []string{pidDirectory, memoryDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pidDirectory, "cgroup"), []byte("2:cpu:/legacy\n0::/user.slice/omastack-service.scope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memoryDirectory, "memory.current"), []byte("1004535808\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readCgroupMemory(procRoot, cgroupRoot, 123)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1004535808 {
		t.Fatalf("memory.current = %d", got)
	}
}

func TestParseCgroupV2PathRejectsUnsafeOrMissingPaths(t *testing.T) {
	for _, content := range []string{
		"0::/../../outside\n",
		"0::relative/path\n",
		"5:memory:/legacy\n",
	} {
		if _, err := parseCgroupV2Path(content); err == nil {
			t.Fatalf("accepted cgroup content %q", content)
		}
	}
}

func TestSamplerMemoryAccountingOrder(t *testing.T) {
	const mebibyte = 1024 * 1024
	tests := []struct {
		name             string
		cgroupBytes      *uint64
		omitChildPSS     bool
		expectedMemoryMB float64
	}{
		{name: "cgroup preferred", cgroupBytes: uint64Pointer(12 * mebibyte), expectedMemoryMB: 12},
		{name: "PSS fallback", expectedMemoryMB: 7},
		{name: "RSS last resort", omitChildPSS: true, expectedMemoryMB: float64(3000*os.Getpagesize()) / mebibyte},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			procRoot := filepath.Join(root, "proc")
			cgroupRoot := filepath.Join(root, "cgroup")
			writeSyntheticProcess(t, procRoot, 100, 1, 1000, 3*1024)
			writeSyntheticProcess(t, procRoot, 101, 100, 2000, 4*1024)
			if test.omitChildPSS {
				if err := os.Remove(filepath.Join(procRoot, "101", "smaps_rollup")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(procRoot, "100", "cgroup"), []byte("0::/user.slice/omastack.scope\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.cgroupBytes != nil {
				directory := filepath.Join(cgroupRoot, "user.slice", "omastack.scope")
				if err := os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "memory.current"), []byte(fmt.Sprintf("%d\n", *test.cgroupBytes)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sampler := &Sampler{procRoot: procRoot, cgroupRoot: cgroupRoot, previous: map[int]ProcSample{}}
			got, err := sampler.Sample(100, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.MemoryMB != test.expectedMemoryMB {
				t.Fatalf("MemoryMB = %v, want %v", got.MemoryMB, test.expectedMemoryMB)
			}
		})
	}
}

func writeSyntheticProcess(t *testing.T, procRoot string, pid, ppid, rssPages, pssKilobytes int) {
	t.Helper()
	directory := filepath.Join(procRoot, fmt.Sprintf("%d", pid))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	fields := make([]string, 22)
	for index := range fields {
		fields[index] = "0"
	}
	fields[0] = "S"
	fields[1] = fmt.Sprintf("%d", ppid)
	fields[11] = "10"
	fields[12] = "5"
	fields[21] = fmt.Sprintf("%d", rssPages)
	stat := fmt.Sprintf("%d (synthetic worker) %s\n", pid, strings.Join(fields, " "))
	if err := os.WriteFile(filepath.Join(directory, "stat"), []byte(stat), 0o600); err != nil {
		t.Fatal(err)
	}
	smaps := fmt.Sprintf("00400000-00401000 r--p 00000000 00:00 0 [rollup]\nRss: 999999 kB\nPss: %d kB\nPss_Dirty: 0 kB\n", pssKilobytes)
	if err := os.WriteFile(filepath.Join(directory, "smaps_rollup"), []byte(smaps), 0o600); err != nil {
		t.Fatal(err)
	}
}

func uint64Pointer(value uint64) *uint64 { return &value }
