package systemd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testServiceID = "22222222-2222-4222-8222-222222222222"

func installFakeSystemctl(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OMASTACK_SYSTEMCTL_TEST_LOG"
case "$2" in
  show)
    printf '%s\n' 'ActiveState=inactive' 'SubState=dead' 'MainPID=0' 'Result=success' 'ExecMainCode=1' 'ExecMainStatus=0' 'NRestarts=3'
    ;;
  explode)
    echo 'controlled failure' >&2
    exit 1
    ;;
esac
`
	path := filepath.Join(directory, "systemctl")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMASTACK_SYSTEMCTL_TEST_LOG", logPath)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestUnitNameValidation(t *testing.T) {
	unit, err := UnitName(testServiceID)
	if err != nil || unit != "omastack-service@"+testServiceID+".service" {
		t.Fatalf("unit=%q err=%v", unit, err)
	}
	if _, err := UnitName("not-an-id"); err == nil {
		t.Fatal("invalid service ID accepted")
	}
}

func TestManagerActionsAndShow(t *testing.T) {
	logPath := installFakeSystemctl(t)
	manager := Manager{Timeout: time.Second}
	ctx := context.Background()
	for name, action := range map[string]func(context.Context, string) error{
		"start": manager.Start, "stop": manager.Stop, "restart": manager.Restart, "reset": manager.ResetFailed, "kill": manager.ForceKill,
	} {
		if err := action(ctx, testServiceID); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	state, err := manager.Show(ctx, testServiceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveState != "inactive" || state.SubState != "dead" || state.Result != "success" || state.NRestarts != 3 || state.ExecMainCode != 1 {
		t.Fatalf("state = %#v", state)
	}
	if !manager.IsEnabled(ctx, "omastackd.service") || manager.IsEnabled(ctx, "unexpected.service") {
		t.Fatal("unit allowlist not enforced")
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--user start", "--user stop", "--user restart", "--user reset-failed", "--user kill", "--user show", "--user is-enabled"} {
		if !strings.Contains(string(calls), expected) {
			t.Errorf("missing %q in calls:\n%s", expected, calls)
		}
	}
}

func TestManagerSurfacesBoundedSystemctlError(t *testing.T) {
	installFakeSystemctl(t)
	_, err := (Manager{Timeout: time.Second}).run(context.Background(), "explode")
	if err == nil || !strings.Contains(err.Error(), "controlled failure") {
		t.Fatalf("error = %v", err)
	}
	if integer("invalid") != 0 || integer("42") != 42 {
		t.Fatal("integer parsing changed")
	}
}
