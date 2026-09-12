package supervise

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/redact"
)

func TestCommandArgumentsAreNotShellJoined(t *testing.T) {
	service := model.Service{Command: model.CommandSpec{Executable: "/usr/bin/printf", Arguments: []string{"%s", "$(touch /tmp/never)", "a; b"}}}
	command, args, err := commandFor(service)
	if err != nil {
		t.Fatal(err)
	}
	if command != "/usr/bin/printf" || !reflect.DeepEqual(args, service.Command.Arguments) {
		t.Fatalf("command=%q args=%q", command, args)
	}
	service.Shell = &model.ShellSpec{Enabled: true, Shell: "/bin/sh", Command: "printf ok | cat"}
	_, args, _ = commandFor(service)
	if !reflect.DeepEqual(args, []string{"-c", "printf ok | cat"}) {
		t.Fatalf("shell args=%q", args)
	}
}

func TestComposeSupervisorOwnsOnlySelectedService(t *testing.T) {
	command, args, err := commandFor(model.Service{Docker: &model.DockerSpec{ComposeFile: "/tmp/compose.yaml", ProjectName: "fixture", Service: "web"}, GracefulStopSeconds: 300})
	want := []string{"compose", "-f", "/tmp/compose.yaml", "--project-name", "fixture", "up", "--no-color", "--no-build", "--no-deps", "--timeout", "300", "web"}
	if err != nil || command != "docker" || !reflect.DeepEqual(args, want) {
		t.Fatalf("command=%s args=%v err=%v", command, args, err)
	}
}

func TestComposeShutdownBudgetIncludesCLICompletion(t *testing.T) {
	s := model.Service{GracefulStopSeconds: 300}
	if shutdownWait(s) != 300*time.Second {
		t.Fatal("host grace changed")
	}
	s.Docker = &model.DockerSpec{}
	if shutdownWait(s) != 305*time.Second {
		t.Fatal("container stop races supervisor timeout")
	}
}

func TestRunOnceStopsProcessGroupOnCancellation(t *testing.T) {
	dir := t.TempDir()
	resolved := paths.Paths{RuntimeServicesDir: dir}
	service := model.Service{ID: "22222222-2222-4222-8222-222222222222", Name: "sleep", Command: model.CommandSpec{Executable: "/usr/bin/sleep", Arguments: []string{"30"}}, WorkingDirectory: dir, StopSignal: "SIGTERM", GracefulStopSeconds: 1}
	record := RuntimeRecord{ServiceID: service.ID}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := runOnce(ctx, resolved, service, &record, time.Now().UTC()); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	childPID := 0
	for childPID == 0 && time.Now().Before(deadline) {
		if persisted, err := ReadRuntime(dir, service.ID); err == nil {
			childPID = persisted.PID
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("child never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runOnce did not stop")
	}
	err := syscall.Kill(childPID, 0)
	if err == nil || (!errors.Is(err, syscall.ESRCH) && !errors.Is(err, os.ErrProcessDone)) {
		t.Fatalf("child still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, service.ID+".json")); err != nil {
		t.Fatal("runtime record missing")
	}
}

func TestEnvironmentFileSecretsAreCollectedIncludingShortValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("ONE=x\nTWO=yz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, secrets, err := BuildEnvironmentWithSecrets(nil, path, map[string]model.EnvValue{"INLINE": {Value: "q", Secret: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"x", "yz", "q"} {
		found := false
		for _, value := range secrets {
			found = found || value == expected
		}
		if !found {
			t.Fatalf("secret %q missing from %#v", expected, secrets)
		}
	}
}

func TestEnvironmentInheritanceUsesSafeAllowlist(t *testing.T) {
	base := []string{
		"PATH=/usr/bin", "LANG=en_AU.UTF-8", "LC_TIME=C", "XDG_RUNTIME_DIR=/run/user/1000",
		"DISPLAY=:1", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus",
		"AWS_SECRET_ACCESS_KEY=ambient-secret", "GITHUB_TOKEN=ambient-token",
		"SSH_AUTH_SOCK=/run/user/1000/ssh-agent", "LD_PRELOAD=/tmp/injected.so",
	}
	inline := map[string]model.EnvValue{
		"GITHUB_TOKEN": {Value: "explicit-token", Secret: true},
		"APP_MODE":     {Value: "development"},
	}
	environment, secrets, err := BuildEnvironmentWithSecrets(base, "", inline)
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, expected := range []string{"PATH=/usr/bin", "LANG=en_AU.UTF-8", "LC_TIME=C", "XDG_RUNTIME_DIR=/run/user/1000", "DISPLAY=:1", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus", "GITHUB_TOKEN=explicit-token", "APP_MODE=development"} {
		if !strings.Contains(joined, "\n"+expected+"\n") {
			t.Errorf("expected environment entry %q in %#v", expected, environment)
		}
	}
	for _, rejected := range []string{"AWS_SECRET_ACCESS_KEY=", "SSH_AUTH_SOCK=", "LD_PRELOAD="} {
		if strings.Contains(joined, "\n"+rejected) {
			t.Errorf("unsafe ambient environment inherited: %q in %#v", rejected, environment)
		}
	}
	if !reflect.DeepEqual(secrets, []string{"explicit-token"}) {
		t.Fatalf("secrets = %#v", secrets)
	}
}

func TestCopyStreamRedactsSecretsBeforeJournalOutput(t *testing.T) {
	var output bytes.Buffer
	var group sync.WaitGroup
	group.Add(1)
	copyStream(&group, &output, "stdout", strings.NewReader("token=super-secret\nnormal\n"), []string{"super-secret"})
	group.Wait()
	if strings.Contains(output.String(), "super-secret") || !strings.Contains(output.String(), redact.Mask) {
		t.Fatalf("output=%q", output.String())
	}
}
