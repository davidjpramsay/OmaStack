package supervise

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"omastack/internal/model"
)

// Execution is shared by launches, probes and Docker tooling. A nil Environment
// means the filtered session baseline, never exec.Cmd's ambient inheritance.
type Execution struct {
	Environment []string
	Directory   string
}

func ForService(service model.Service) (Execution, error) {
	environment, err := BuildEnvironment(os.Environ(), service.EnvironmentFile, service.Environment)
	return Execution{Environment: environment, Directory: service.WorkingDirectory}, err
}

func (e Execution) Command(ctx context.Context, executable string, args ...string) (*exec.Cmd, error) {
	environment := e.Environment
	if environment == nil {
		var err error
		environment, err = BuildEnvironment(os.Environ(), "", nil)
		if err != nil {
			return nil, err
		}
	}
	path, err := ResolveExecutable(executable, environment, e.Directory)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, path, args...)
	command.Env = environment
	command.Dir = e.Directory
	return command, nil
}

// ResolveExecutable uses the child's PATH without changing the process-wide
// environment. Relative PATH results retain Go's ErrDot protection; an explicit
// ./command remains an intentional, supported choice.
func ResolveExecutable(executable string, environment []string, directory string) (string, error) {
	if strings.ContainsRune(executable, filepath.Separator) {
		return executable, nil
	}
	var path string
	for _, entry := range environment {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			path = value
		}
	}
	for _, part := range filepath.SplitList(path) {
		if part == "" {
			part = "."
		}
		candidate := filepath.Join(part, executable)
		check := candidate
		if !filepath.IsAbs(candidate) && directory != "" {
			check = filepath.Join(directory, candidate)
		}
		info, err := os.Stat(check)
		if err != nil || !info.Mode().IsRegular() || syscall.Access(check, 1) != nil {
			continue
		}
		if !filepath.IsAbs(candidate) {
			return "", &exec.Error{Name: executable, Err: exec.ErrDot}
		}
		return candidate, nil
	}
	return "", &exec.Error{Name: executable, Err: exec.ErrNotFound}
}
