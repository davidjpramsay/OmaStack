package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omastack/internal/docker"
	"omastack/internal/install"
	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/systemd"
	"omastack/internal/validate"
)

// Explicit opt-in only: this creates one temporary runtime user unit and one
// uniquely named Compose project using an already-cached, pinned image. It does
// not replace installed units, restart the daemon, publish ports or pull images.
func TestNativeDockerLifecycleAcceptance(t *testing.T) {
	if os.Getenv("OMASTACK_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("set OMASTACK_NATIVE_ACCEPTANCE=1 on an Omarchy desktop to run isolated Docker/systemd acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := func(args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	image := command("docker", "image", "inspect", "alpine:3.22", "--format", "{{.Id}}")
	if !strings.HasPrefix(image, "sha256:") {
		t.Fatal("cached Alpine image missing")
	}
	id, err := validate.NewID()
	if err != nil {
		t.Fatal(err)
	}
	project := "omastackacceptance" + strings.ReplaceAll(id, "-", "")
	root := t.TempDir()
	configHome, stateHome, cacheHome, runtimeHome := filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "runtime")
	resolved := paths.Paths{ConfigDir: filepath.Join(configHome, "omastack"), ConfigFile: filepath.Join(configHome, "omastack", "config.json"), ExportDir: filepath.Join(configHome, "omastack", "exports"), StateDir: filepath.Join(stateHome, "omastack"), RuntimeServicesDir: filepath.Join(stateHome, "omastack", "services"), CacheDir: filepath.Join(cacheHome, "omastack"), RuntimeDir: filepath.Join(runtimeHome, "omastack"), SocketFile: filepath.Join(runtimeHome, "omastack", "control.sock"), SnapshotFile: filepath.Join(runtimeHome, "omastack", "state.json")}
	d, err := New(resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer d.proxy.Close()
	binary := filepath.Join(root, "omastack")
	build := exec.CommandContext(ctx, "go", "build", "-buildvcs=false", "-o", binary, "./cmd/omastack")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	compose := filepath.Join(root, "compose.yaml")
	writeFixture(t, compose, fmt.Sprintf("services:\n  api:\n    image: %s\n    pull_policy: never\n    network_mode: none\n    restart: unless-stopped\n    command: [\"/bin/sh\", \"-c\", \"trap 'exit 0' TERM; while :; do sleep 1; done\"]\n", image), 0o600)
	service := model.Service{ID: id, Name: "Native fixture", WorkingDirectory: root, StopSignal: "SIGTERM", GracefulStopSeconds: 2, Restart: model.RestartPolicy{Mode: "never"}, Docker: &model.DockerSpec{ComposeFile: compose, ProjectName: project, Service: "api"}}
	pid, err := validate.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.createProject(rawParams(t, projectPayload{Project: model.Project{ID: pid, Name: "Native acceptance", Services: []model.Service{service}}})); err != nil {
		t.Fatal(err)
	}
	unit, err := systemd.UnitName(id)
	if err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(fmt.Sprintf("/run/user/%d", os.Getuid()), "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(unitDir, unit)
	file, err := os.OpenFile(unitPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Test paths contain no shell syntax; systemd executes argv directly.
	unitData := fmt.Sprintf("[Unit]\nDescription=Isolated OmaStack acceptance fixture\n[Service]\nType=simple\nExecStart=/usr/bin/env XDG_CONFIG_HOME=%s XDG_STATE_HOME=%s XDG_CACHE_HOME=%s XDG_RUNTIME_DIR=%s %s supervise %s\nKillMode=mixed\nTimeoutStopSec=20s\nRestart=no\n", configHome, stateHome, cacheHome, runtimeHome, binary, id)
	_, writeErr := file.WriteString(unitData)
	closeErr := file.Close()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for _, args := range [][]string{{"systemctl", "--user", "stop", unit}, {"docker", "compose", "-f", compose, "--project-name", project, "down", "--remove-orphans"}} {
			if output, err := exec.CommandContext(cleanupCtx, args[0], args[1:]...).CombinedOutput(); err != nil {
				t.Errorf("fixture cleanup %s: %v %s", args[0], err, output)
			}
		}
		// A stopped runtime unit may already be garbage-collected after the
		// uninstall reload. Resetting a nonexistent failed state is unnecessary.
		_, _ = exec.CommandContext(cleanupCtx, "systemctl", "--user", "reset-failed", unit).CombinedOutput()
		if err := os.Remove(unitPath); err != nil {
			t.Error(err)
		}
		if output, err := exec.CommandContext(cleanupCtx, "systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
			t.Errorf("fixture reload: %v %s", err, output)
		}
	})
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write fixture unit: %v %v", writeErr, closeErr)
	}
	command("systemctl", "--user", "daemon-reload")
	state := func() docker.ContainerState {
		t.Helper()
		value, err := docker.Inspect(ctx, compose, project, "api")
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	waitRunning := func() {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if state().State == "running" {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("fixture never ran")
	}
	action := func(verb, target string) {
		t.Helper()
		if err := d.performAction(ctx, verb, target); err != nil {
			t.Fatal(err)
		}
	}
	assertStopped := func() {
		t.Helper()
		if err := d.ensureStopped(ctx, id); err != nil {
			t.Fatal(err)
		}
		d.reconcile(ctx)
		if got := d.Snapshot().Runtime[id]; got.Status != model.StatusStopped {
			t.Fatalf("stopped reconciliation: %+v", got)
		}
	}
	if _, err := d.dockerAction(ctx, rawParams(t, dockerActionPayload{ServiceID: id, Action: "recreate"})); err != nil {
		t.Fatal(err)
	}
	if got := state().State; got != "created" {
		t.Fatalf("stopped recreate launched container: %s", got)
	}
	assertStopped()
	action("start", id)
	waitRunning()
	if _, err := d.deleteService(ctx, rawParams(t, serviceIDPayload{ServiceID: id})); err == nil {
		t.Fatal("deleted managed running fixture")
	}
	if _, err := d.dockerAction(ctx, rawParams(t, dockerActionPayload{ServiceID: id, Action: "recreate"})); err != nil {
		t.Fatal(err)
	}
	waitRunning()
	action("stop", "all")
	assertStopped()
	action("start", id)
	waitRunning()
	action("kill", id)
	assertStopped()
	command("docker", "compose", "-f", compose, "--project-name", project, "start", "api")
	if _, err := d.deleteService(ctx, rawParams(t, serviceIDPayload{ServiceID: id})); err == nil {
		t.Fatal("deleted orphan running fixture")
	}
	// Exercise native container shutdown in Uninstall without touching the
	// real daemon: no daemon unit is placed in this temporary HOME.
	t.Setenv("HOME", filepath.Join(root, "uninstall-home"))
	if err := install.Uninstall(ctx, resolved, []string{id}, false); err != nil {
		t.Fatal(err)
	}
	assertStopped()
	if _, err := d.deleteProject(ctx, rawParams(t, projectIDPayload{ProjectID: pid})); err != nil {
		t.Fatal(err)
	}
	t.Log("stopped/running recreate, managed start, Stop All, Force kill, orphan deletion refusal and non-purge uninstall passed")
}
