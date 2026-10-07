package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"omastack/internal/bounded"
	"omastack/internal/model"
	"omastack/internal/securefile"
	"omastack/internal/supervise"
	"omastack/internal/validate"
)

type Availability struct {
	Installed        bool   `json:"installed"`
	DaemonRunning    bool   `json:"daemonRunning"`
	ComposeAvailable bool   `json:"composeAvailable"`
	Error            string `json:"error,omitempty"`
}

type ImportedService struct {
	Name         string              `json:"name"`
	Command      []string            `json:"command,omitempty"`
	Ports        []string            `json:"ports,omitempty"`
	Dependencies []ComposeDependency `json:"dependencies,omitempty"`
}

type ComposeDependency struct {
	Service   string `json:"service"`
	Condition string `json:"condition"`
	Required  bool   `json:"required"`
}

type ContainerState struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Health    string    `json:"health"`
	ExitCode  int       `json:"exitCode"`
	Published []int     `json:"publishedPorts"`
	CPU       float64   `json:"cpu"`
	MemoryMB  float64   `json:"memoryMb"`
	StartedAt time.Time `json:"startedAt"`
}

type composePS struct {
	ID         string `json:"ID"`
	Name       string `json:"Name"`
	State      string `json:"State"`
	Health     string `json:"Health"`
	ExitCode   int    `json:"ExitCode"`
	Publishers []struct {
		PublishedPort int `json:"PublishedPort"`
	} `json:"Publishers"`
}

func execution(options []supervise.Execution) supervise.Execution {
	if len(options) > 0 {
		return options[0]
	}
	return supervise.Execution{}
}

func Detect(ctx context.Context) Availability {
	if _, err := exec.LookPath("docker"); err != nil {
		return Availability{}
	}
	result := Availability{Installed: true}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if output, err := runCombined(probeCtx, 1<<20, "docker", "version", "--format", "{{.Server.Version}}"); err == nil && strings.TrimSpace(string(output)) != "" {
		result.DaemonRunning = true
	} else if err != nil {
		result.Error = concise(output, err)
	}
	composeCtx, composeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer composeCancel()
	_, err := runCombined(composeCtx, 1<<20, "docker", "compose", "version")
	result.ComposeAvailable = err == nil
	return result
}

func ImportCompose(ctx context.Context, composeFile string, options ...supervise.Execution) ([]ImportedService, error) {
	if !filepath.IsAbs(composeFile) || filepath.Clean(composeFile) != composeFile {
		return nil, errors.New("compose path must be absolute and normalized")
	}
	composeData, err := securefile.ReadRegular(composeFile, 8<<20, false)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Feed verified bytes over stdin so a rename cannot swap the file between
	// validation and Compose opening it. Non-interpolation prevents project
	// environment values from appearing in the discovery result. Setting the
	// working directory preserves relative-path semantics for the input file.
	policy := execution(options)
	policy.Directory = filepath.Dir(composeFile)
	cmd, err := policy.Command(callCtx, "docker", composeConfigArguments()...)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(composeData)
	output := bounded.NewBuffer(16 << 20)
	stderr := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = output, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config: %s", concise(stderr.Bytes(), err))
	}
	var document struct {
		Services map[string]struct {
			Command   any `json:"command"`
			DependsOn map[string]struct {
				Condition string `json:"condition"`
				Required  *bool  `json:"required"`
			} `json:"depends_on"`
			Ports []struct {
				Published string `json:"published"`
				Target    int    `json:"target"`
			} `json:"ports"`
		} `json:"services"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse Compose model: %w", err)
	}
	result := make([]ImportedService, 0, len(document.Services))
	for name, service := range document.Services {
		entry := ImportedService{Name: name}
		for dependency, options := range service.DependsOn {
			condition := options.Condition
			if condition == "" {
				condition = "service_started"
			}
			entry.Dependencies = append(entry.Dependencies, ComposeDependency{Service: dependency, Condition: condition, Required: options.Required == nil || *options.Required})
		}
		sort.Slice(entry.Dependencies, func(i, j int) bool { return entry.Dependencies[i].Service < entry.Dependencies[j].Service })
		switch command := service.Command.(type) {
		case string:
			entry.Command = []string{command}
		case []any:
			for _, value := range command {
				if text, ok := value.(string); ok {
					entry.Command = append(entry.Command, text)
				}
			}
		}
		for _, port := range service.Ports {
			entry.Ports = append(entry.Ports, fmt.Sprintf("%s:%d", port.Published, port.Target))
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func composeConfigArguments() []string {
	return []string{"compose", "-f", "-", "config", "--no-interpolate", "--format", "json"}
}

func Inspect(ctx context.Context, composeFile, projectName, service string, options ...supervise.Execution) (ContainerState, error) {
	return inspect(ctx, composeFile, projectName, service, true, execution(options))
}

func InspectHealth(ctx context.Context, composeFile, projectName, service string, options ...supervise.Execution) (ContainerState, error) {
	return inspect(ctx, composeFile, projectName, service, false, execution(options))
}

func inspect(ctx context.Context, composeFile, projectName, service string, stats bool, policy supervise.Execution) (ContainerState, error) {
	args := []string{"compose", "-f", composeFile}
	if projectName != "" {
		args = append(args, "--project-name", projectName)
	}
	args = append(args, "ps", "--all", "--format", "json", service)
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := runWithPolicy(callCtx, policy, 8<<20, "docker", args...)
	if err != nil {
		return ContainerState{}, fmt.Errorf("docker compose ps: %s", concise(output, err))
	}
	records, err := decodePS(output)
	if err != nil {
		return ContainerState{}, err
	}
	if len(records) == 0 {
		return ContainerState{State: "stopped"}, nil
	}
	record := records[0]
	result := ContainerState{ID: record.ID, Name: record.Name, State: strings.ToLower(record.State), Health: strings.ToLower(record.Health), ExitCode: record.ExitCode}
	// Compose's process can outlive several container replacements/restarts.
	// Read actual container state so readiness is tied to this container run.
	if result.ID != "" {
		output, err := runWithPolicy(callCtx, policy, 64<<10, "docker", "inspect", "--format", "{{json .State}}", result.ID)
		if err != nil {
			return ContainerState{}, fmt.Errorf("docker inspect: %s", concise(output, err))
		}
		var state struct {
			Status    string    `json:"Status"`
			StartedAt time.Time `json:"StartedAt"`
			ExitCode  int       `json:"ExitCode"`
			Health    struct {
				Status string `json:"Status"`
			} `json:"Health"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(output), &state); err != nil {
			return ContainerState{}, fmt.Errorf("parse Docker state: %w", err)
		}
		result.State, result.Health, result.ExitCode, result.StartedAt = strings.ToLower(state.Status), strings.ToLower(state.Health.Status), state.ExitCode, state.StartedAt
	}
	for _, publisher := range record.Publishers {
		if publisher.PublishedPort > 0 {
			result.Published = append(result.Published, publisher.PublishedPort)
		}
	}
	sort.Ints(result.Published)
	if len(result.Published) > 256 {
		result.Published = result.Published[:256]
	}
	if stats && result.ID != "" && result.State == "running" {
		result.CPU, result.MemoryMB, _ = inspectStats(callCtx, result.ID, policy)
	}
	return result, nil
}

func inspectStats(ctx context.Context, containerID string, policy supervise.Execution) (float64, float64, error) {
	output, err := runWithPolicy(ctx, policy, 1<<20, "docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)
	if err != nil {
		return 0, 0, fmt.Errorf("docker stats: %s", concise(output, err))
	}
	var stats struct {
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &stats); err != nil {
		return 0, 0, fmt.Errorf("parse docker stats: %w", err)
	}
	cpu, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(stats.CPUPerc), "%"), 64)
	if err != nil {
		return 0, 0, err
	}
	memoryText := strings.TrimSpace(strings.Split(stats.MemUsage, "/")[0])
	memory, err := parseMemoryMB(memoryText)
	return cpu, memory, err
}

func parseMemoryMB(value string) (float64, error) {
	value = strings.TrimSpace(value)
	units := []struct {
		suffix string
		factor float64
	}{{"GiB", 1024}, {"GB", 1000}, {"MiB", 1}, {"MB", 1}, {"KiB", 1.0 / 1024}, {"kB", 1.0 / 1000}, {"B", 1.0 / (1024 * 1024)}}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(value, unit.suffix))
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil {
				return 0, err
			}
			return parsed * unit.factor, nil
		}
	}
	return 0, fmt.Errorf("unknown memory unit %q", value)
}

func decodePS(output []byte) ([]composePS, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var records []composePS
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		return records, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	for scanner.Scan() {
		var record composePS
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func OpenTerminal(ctx context.Context, binary, serviceID, configFile string) error {
	if !filepath.IsAbs(binary) || !filepath.IsAbs(configFile) || !validate.ID(serviceID) {
		return errors.New("invalid Docker terminal command")
	}
	// Terminal/session brokers can restore the complete user-manager environment.
	// Resolve service policy inside the terminal, without credentials in argv.
	args := []string{binary, "docker-shell", serviceID, configFile}
	launcher := "xdg-terminal-exec"
	launcherArgs := args
	if _, err := exec.LookPath("uwsm-app"); err == nil {
		launcher, launcherArgs = "uwsm-app", append([]string{"--", "xdg-terminal-exec"}, args...)
	}
	command, err := (supervise.Execution{}).Command(ctx, launcher, launcherArgs...)
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open Docker terminal: %w", err)
	}
	return command.Process.Release()
}

func RunShell(ctx context.Context, service model.Service) error {
	if service.Docker == nil {
		return errors.New("docker service not found")
	}
	policy, err := supervise.ForService(service)
	if err != nil {
		return err
	}
	args := append(composeArguments(service.Docker), "exec", service.Docker.Service, "/bin/sh")
	command, err := policy.Command(ctx, "docker", args...)
	if err != nil {
		return err
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func Action(ctx context.Context, composeFile, projectName, service, action string, options ...supervise.Execution) error {
	allowed := map[string][]string{
		"start":   {"up", "--detach", "--no-build", "--no-deps"},
		"stop":    {"stop"},
		"restart": {"restart", "--no-deps"},
		"rebuild": {"build"},
		// --no-start and --no-deps preserve stopped state and separate ownership.
		"recreate": {"up", "--no-start", "--no-build", "--force-recreate", "--no-deps"},
	}
	verb, ok := allowed[action]
	if !ok {
		return errors.New("unsupported Docker action")
	}
	args := []string{"compose", "-f", composeFile}
	if projectName != "" {
		args = append(args, "--project-name", projectName)
	}
	args = append(args, verb...)
	args = append(args, service)
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	output, err := runWithPolicy(callCtx, execution(options), 2<<20, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker compose %s: %s", action, concise(output, err))
	}
	return nil
}

func concise(output []byte, err error) string {
	// Docker/Compose diagnostic streams may interpolate values from project
	// environment files. Never surface that unclassified output through UI,
	// notifications or the local API.
	_ = output
	value := "command failed"
	if err != nil {
		value = strings.Join(strings.Fields(err.Error()), " ")
	}
	if len(value) > 512 {
		value = value[:509] + "…"
	}
	return value
}

func runCombined(ctx context.Context, limit int, executable string, args ...string) ([]byte, error) {
	return runWithPolicy(ctx, supervise.Execution{}, limit, executable, args...)
}

func runWithPolicy(ctx context.Context, policy supervise.Execution, limit int, executable string, args ...string) ([]byte, error) {
	output := bounded.NewBuffer(limit)
	command, err := policy.Command(ctx, executable, args...)
	if err != nil {
		return nil, err
	}
	command.Stdout, command.Stderr = output, output
	err = command.Run()
	return append([]byte{}, output.Bytes()...), err
}
