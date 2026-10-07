package supervise

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"omastack/internal/model"
	"omastack/internal/paths"
)

func TestServicePATHResolvesLaunchWithoutMutatingAmbientEnvironment(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "fixture-service")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin")
	for _, fromFile := range []bool{false, true} {
		service := model.Service{ID: "11111111-1111-4111-8111-111111111111", Command: model.CommandSpec{Executable: "fixture-service"}, WorkingDirectory: dir}
		if fromFile {
			service.EnvironmentFile = filepath.Join(dir, "private.env")
			if err := os.WriteFile(service.EnvironmentFile, []byte("PATH="+dir+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else {
			service.Environment = map[string]model.EnvValue{"PATH": {Value: dir}}
		}
		code, _, err := runOnce(context.Background(), paths.Paths{RuntimeServicesDir: dir}, service, &RuntimeRecord{ServiceID: service.ID}, time.Now())
		if code != 0 || err != nil {
			t.Fatalf("fromFile=%t code=%d err=%v", fromFile, code, err)
		}
		if os.Getenv("PATH") != "/usr/bin" {
			t.Fatal("ambient PATH changed")
		}
	}
}

func TestExecutableResolutionRejectsRelativePATHButAllowsExplicitPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", ":/usr/bin"} {
		if _, err := ResolveExecutable("fixture", []string{"PATH=" + path}, dir); !errors.Is(err, exec.ErrDot) {
			t.Fatalf("relative PATH %q: %v", path, err)
		}
	}
	cmd, err := (Execution{Environment: []string{}, Directory: dir}).Command(context.Background(), "./fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutable("fixture", []string{}, dir); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("empty PATH: %v", err)
	}
}
