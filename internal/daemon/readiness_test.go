package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omastack/internal/control"
	"omastack/internal/health"
	"omastack/internal/model"
	"omastack/internal/supervise"
	"omastack/internal/systemd"
)

const readinessDB = "22222222-2222-4222-8222-222222222222"
const readinessAPI = "33333333-3333-4333-8333-333333333333"

func readinessFixture(t *testing.T) (*Daemon, string) {
	t.Helper()
	d := testDaemon(t)
	dir := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OMASTACK_TEST_CALLS"
for unit do :; done
active_file="$OMASTACK_TEST_ACTIVE.$unit"
case "$2" in
  show)
    if [ -f "$active_file" ]; then
      printf '%s\n' 'ActiveState=active' 'SubState=running' 'MainPID=700' 'InvocationID=current'
    else
      printf '%s\n' 'ActiveState=inactive' 'Result=success'
    fi ;;
  start) : > "$active_file" ;;
  stop) /usr/bin/rm -f -- "$active_file" ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("OMASTACK_TEST_CALLS", filepath.Join(dir, "calls"))
	t.Setenv("OMASTACK_TEST_ACTIVE", filepath.Join(dir, "active"))
	db := model.Service{ID: readinessDB, Name: "db", Command: model.CommandSpec{Executable: "/usr/bin/true"}, WorkingDirectory: dir,
		Health: &model.HealthCheck{Type: "command", Command: &model.CommandSpec{Executable: "/usr/bin/true"}, IntervalSeconds: 1, TimeoutSeconds: 1, Retries: 1}}
	api := model.Service{ID: readinessAPI, Name: "api", Command: model.CommandSpec{Executable: "/usr/bin/true"}, WorkingDirectory: dir, Autostart: true, Dependencies: []model.Dependency{{ServiceID: readinessDB, Condition: "healthy"}}}
	_, err := d.createProject(rawParams(t, projectPayload{Project: model.Project{ID: "11111111-1111-4111-8111-111111111111", Name: "Fixture", Services: []model.Service{db, api}}}))
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute)
	if err := supervise.WriteRuntime(d.paths.RuntimeServicesDir, supervise.RuntimeRecord{ServiceID: readinessDB, SupervisorPID: 700, PID: 701, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	return d, dir
}

func TestReadinessNeverTrustsCachedHealthyOrUnhealthy(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "current-success", false: "old-success"}[success], func(t *testing.T) {
			d, _ := readinessFixture(t)
			if err := d.systemd.Start(context.Background(), readinessDB); err != nil {
				t.Fatal(err)
			}
			d.healthStates.Set(readinessDB, health.State{Status: "healthy", LastChecked: time.Now()})
			if success {
				d.healthStates.Set(readinessDB, health.State{Status: "unhealthy", LastError: "old failure"})
			} else {
				if err := d.store.Update(func(c *model.Config) error {
					c.Projects[0].Services[0].Health.Command.Executable = "/usr/bin/false"
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			err := d.waitHealthy(context.Background(), readinessDB, time.Second)
			if (err == nil) != success {
				t.Fatalf("success=%t err=%v", success, err)
			}
		})
	}
}

func TestAutostartIncludesDependenciesOncePerSession(t *testing.T) {
	d, dir := readinessFixture(t)
	d.startAutostart(context.Background())
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if !strings.Contains(string(calls), "start omastack-service@"+readinessDB) {
		t.Fatalf("dependency not started: %s", calls)
	}
	if !strings.Contains(string(calls), "start omastack-service@"+readinessAPI) {
		t.Fatalf("dependent not started: %s", calls)
	}
	if d.autostartError != "" {
		t.Fatal(d.autostartError)
	}
	before := string(calls)
	d.startAutostart(context.Background())
	calls, _ = os.ReadFile(filepath.Join(dir, "calls"))
	if string(calls) != before {
		t.Fatal("daemon restart retried autostart")
	}
}

func TestStopDoesNotStopSharedPrerequisite(t *testing.T) {
	d, dir := readinessFixture(t)
	if err := d.performAction(context.Background(), "stop", readinessAPI); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Contains(string(calls), "stop omastack-service@"+readinessDB) {
		t.Fatalf("stopped prerequisite: %s", calls)
	}
}

func TestRunIdentityRejectsStaleRecordAndChangesWithChildOrDefinition(t *testing.T) {
	started := time.Now()
	unit := systemd.UnitState{ActiveState: "active", MainPID: 10, InvocationID: "new"}
	record := supervise.RuntimeRecord{SupervisorPID: 9, PID: 20, StartedAt: &started}
	svc := model.Service{ID: readinessDB}
	if runIdentity(svc, unit, record) != "" {
		t.Fatal("stale supervisor record accepted")
	}
	record.SupervisorPID = 10
	before := runIdentity(svc, unit, record)
	if before == "" {
		t.Fatal("current record rejected")
	}
	record.PID++
	if runIdentity(svc, unit, record) == before {
		t.Fatal("child restart retained identity")
	}
	record.PID--
	svc.Health = &model.HealthCheck{Type: "command"}
	if runIdentity(svc, unit, record) == before {
		t.Fatal("edited health retained identity")
	}
}

func TestSettingsPatchesAreAtomicAndValidated(t *testing.T) {
	d := testDaemon(t)
	for _, raw := range []string{`{"notifications":{"crashed":false}}`, `{"notifications":{"unhealthy":false}}`} {
		response := d.handle(context.Background(), control.Request{ID: "patch", Method: "settings.patch", Params: []byte(raw)})
		if !response.OK {
			t.Fatal(response.Error)
		}
	}
	n := d.store.Get().Settings.Notifications
	if n.Crashed || n.Unhealthy || !n.Recovered {
		t.Fatalf("lost settings: %+v", n)
	}
	for _, raw := range []string{`{"proxy":{"listenHost":"0.0.0.0"}}`, `{"pollIntervalSeconds":0}`, `{"notifications":{"unknown":true}}`} {
		if _, err := d.patchSettings([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid patch %s", raw)
		}
	}
	if d.store.Get().Settings.Proxy.ListenHost != "127.0.0.1" {
		t.Fatal("invalid patch leaked")
	}
}
