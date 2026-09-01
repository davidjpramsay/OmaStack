package supervise

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/redact"
	"omastack/internal/store"
	"omastack/internal/validate"
)

func Run(ctx context.Context, resolved paths.Paths, serviceID string) error {
	if !validate.ID(serviceID) {
		return errors.New("invalid service id")
	}
	configStore, err := store.OpenConfig(resolved.ConfigFile)
	if err != nil {
		return err
	}
	_, service, ok := configStore.Get().FindService(serviceID)
	if !ok {
		return fmt.Errorf("service %s not found", serviceID)
	}

	record := RuntimeRecord{ServiceID: serviceID, SupervisorPID: os.Getpid()}
	_ = WriteRuntime(resolved.RuntimeServicesDir, record)
	attempt := 0
	lastStarted := time.Time{}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		started := time.Now().UTC()
		if service.Restart.ResetAfterSeconds > 0 && !lastStarted.IsZero() && started.Sub(lastStarted) > time.Duration(service.Restart.ResetAfterSeconds)*time.Second {
			attempt = 0
		}
		lastStarted = started
		exitCode, signalName, runErr := runOnce(ctx, resolved, *service, &record, started)
		if ctx.Err() != nil {
			// A supervisor cancellation is the expected systemd stop path. The
			// child may report a terminating signal, but this is not a crash.
			exitCode = 0
			signalName = ""
			record.LastError = ""
		} else if runErr != nil {
			record.LastError = safeError(runErr)
		}
		stopped := time.Now().UTC()
		record.PID = 0
		record.StoppedAt = &stopped
		record.ExitCode = &exitCode
		record.Signal = signalName
		_ = WriteRuntime(resolved.RuntimeServicesDir, record)
		if ctx.Err() != nil {
			return nil
		}
		if !service.Restart.ShouldRestart(exitCode, attempt) {
			if runErr != nil {
				return runErr
			}
			return nil
		}
		attempt++
		record.RestartCount++
		_ = WriteRuntime(resolved.RuntimeServicesDir, record)
		delay := time.Duration(service.Restart.DelaySeconds) * time.Second
		if delay < time.Second {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

func runOnce(ctx context.Context, resolved paths.Paths, service model.Service, record *RuntimeRecord, started time.Time) (int, string, error) {
	command, args, err := commandFor(service)
	if err != nil {
		return 127, "", err
	}
	environment, secrets, err := BuildEnvironmentWithSecrets(os.Environ(), service.EnvironmentFile, service.Environment)
	if err != nil {
		return 126, "", err
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = service.WorkingDirectory
	cmd.Env = environment
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 126, "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 126, "", err
	}
	if err := cmd.Start(); err != nil {
		return exitCode(err), "", err
	}

	record.PID = cmd.Process.Pid
	record.StartedAt = &started
	record.StoppedAt = nil
	record.ExitCode = nil
	record.Signal = ""
	record.LastError = ""
	_ = WriteRuntime(resolved.RuntimeServicesDir, *record)

	var outputWG sync.WaitGroup
	outputWG.Add(2)
	go copyStream(&outputWG, os.Stdout, "stdout", stdout, secrets)
	go copyStream(&outputWG, os.Stderr, "stderr", stderr, secrets)

	terminated := make(chan os.Signal, 2)
	signal.Notify(terminated, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(terminated)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-waited:
	case received := <-terminated:
		forward := signalFor(service.StopSignal)
		if received == syscall.SIGKILL {
			forward = syscall.SIGKILL
		}
		_ = syscall.Kill(-cmd.Process.Pid, forward)
		waitErr = waitWithTimeout(waited, cmd.Process.Pid, time.Duration(service.GracefulStopSeconds)*time.Second)
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, signalFor(service.StopSignal))
		waitErr = waitWithTimeout(waited, cmd.Process.Pid, time.Duration(service.GracefulStopSeconds)*time.Second)
	}
	outputWG.Wait()
	code := exitCode(waitErr)
	return code, exitSignal(waitErr), waitErr
}

func commandFor(service model.Service) (string, []string, error) {
	if service.Docker != nil {
		args := []string{"compose", "-f", service.Docker.ComposeFile}
		if service.Docker.ProjectName != "" {
			args = append(args, "--project-name", service.Docker.ProjectName)
		}
		args = append(args, "up", "--no-color", "--no-build", service.Docker.Service)
		return "docker", args, nil
	}
	if service.Shell != nil && service.Shell.Enabled {
		return service.Shell.Shell, []string{"-c", service.Shell.Command}, nil
	}
	if service.Command.Executable == "" {
		return "", nil, errors.New("empty executable")
	}
	return service.Command.Executable, append([]string{}, service.Command.Arguments...), nil
}

func copyStream(wg *sync.WaitGroup, destination io.Writer, stream string, source io.Reader, secrets []string) {
	defer wg.Done()
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		fmt.Fprintf(destination, "%s\t%s\n", stream, redact.Text(scanner.Text(), secrets))
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintf(os.Stderr, "omastack-log\t%s stream error: %v\n", stream, err)
	}
}

func waitWithTimeout(waited <-chan error, pid int, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	select {
	case err := <-waited:
		return err
	case <-time.After(timeout):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return <-waited
	}
}

func signalFor(name string) syscall.Signal {
	switch name {
	case "SIGINT":
		return syscall.SIGINT
	case "SIGHUP":
		return syscall.SIGHUP
	case "SIGQUIT":
		return syscall.SIGQUIT
	default:
		return syscall.SIGTERM
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return 127
	}
	return 1
}

func exitSignal(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return ""
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return ""
	}
	return status.Signal().String()
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(text) > 512 {
		text = text[:509] + "…"
	}
	return text
}

func SortedEnvironment(values []string) []string {
	copy := append([]string{}, values...)
	sort.Strings(copy)
	return copy
}
