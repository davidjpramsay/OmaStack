package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omastack/internal/model"
	"omastack/internal/redact"
)

func TestProjectMetadataUpdatePreservesCurrentServicesAndOrder(t *testing.T) {
	d, dir := readinessFixture(t)
	stale := d.store.Get().Projects[0]
	created, err := d.createService(rawParams(t, servicePayload{ProjectID: stale.ID, Service: model.Service{Name: "new", WorkingDirectory: dir, Command: model.CommandSpec{Executable: "/usr/bin/true"}}}))
	if err != nil {
		t.Fatal(err)
	}
	id := created.(map[string]string)["id"]
	if err := d.systemd.Start(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := d.store.Update(func(c *model.Config) error {
		c.Projects[0].Order = 4
		c.Projects[0].Services[0].Name = "concurrently edited"
		c.Projects[0].Services[0].Environment = map[string]model.EnvValue{"TOKEN": {Value: "new-secret", Secret: true}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stale.Name = "renamed"
	if _, err := d.updateProject(rawParams(t, projectPayload{Project: stale})); err != nil {
		t.Fatal(err)
	}
	current := d.store.Get().Projects[0]
	if current.Name != "renamed" || current.Order != 4 || len(current.Services) != 3 || current.Services[0].Name != "concurrently edited" || current.Services[0].Environment["TOKEN"].Value != "new-secret" {
		t.Fatalf("stale metadata overwrote definitions: %+v", current)
	}
	if _, _, ok := d.store.Get().FindService(id); !ok {
		t.Fatal("active service lost")
	}
}

func TestSecretKeepReferencesPreserveToggleAndRenameWithoutMaskAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name, destination string
		value             model.EnvValue
		want              string
		secret, fail      bool
	}{
		{"keep", "TOKEN", model.EnvValue{KeepFrom: "TOKEN", Secret: true}, "original", true, false},
		{"plain", "TOKEN", model.EnvValue{KeepFrom: "TOKEN"}, "original", false, false},
		{"rename", "RENAMED", model.EnvValue{KeepFrom: "TOKEN", Secret: true}, "original", true, false},
		{"literal mask", "TOKEN", model.EnvValue{Value: redact.Mask, Secret: true}, redact.Mask, true, false},
		{"replacement", "TOKEN", model.EnvValue{Value: "replacement", Secret: true}, "replacement", true, false},
		{"invalid reference", "TOKEN", model.EnvValue{KeepFrom: "MISSING"}, "original", true, true},
		{"ambiguous reference", "TOKEN", model.EnvValue{KeepFrom: "TOKEN", Value: "replacement"}, "original", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := readinessFixture(t)
			_, service, _ := d.store.Get().FindService(readinessDB)
			service.Environment = map[string]model.EnvValue{"TOKEN": {Value: "original", Secret: true}}
			if _, err := d.updateService(rawParams(t, servicePayload{Service: *service})); err != nil {
				t.Fatal(err)
			}
			service.Environment = map[string]model.EnvValue{tc.destination: tc.value}
			_, err := d.updateService(rawParams(t, servicePayload{Service: *service}))
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
			_, stored, _ := d.store.Get().FindService(readinessDB)
			key := tc.destination
			if tc.fail {
				key = "TOKEN"
			}
			value := stored.Environment[key]
			if value.Value != tc.want || value.Secret != tc.secret || value.KeepFrom != "" {
				t.Fatalf("stored=%+v", value)
			}
		})
	}
}

func dockerLifecycleFixture(t *testing.T) (*Daemon, string) {
	t.Helper()
	d, dir := readinessFixture(t)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "FIXTURE_DIR/docker-calls"
state=$(/usr/bin/cat "FIXTURE_DIR/container-state")
case "$*" in
 *" ps --all --format json "*) printf '[{"ID":"fixture","State":"%s"}]\n' "$state" ;;
 "inspect --format {{json .State}} fixture") printf '{"Status":"%s","StartedAt":"2026-10-07T00:00:00Z","Health":{"Status":"healthy"}}\n' "$state" ;;
 *" stop --timeout "*)
   [ ! -f "FIXTURE_DIR/fail-stop" ] || exit 1
   printf exited > "FIXTURE_DIR/container-state" ;;
 *" up --no-start "*)
   [ ! -f "FIXTURE_DIR/fail-recreate" ] || exit 1
   printf created > "FIXTURE_DIR/container-state" ;;
 *" config --no-interpolate --format json") /usr/bin/cat >/dev/null; printf '{"services":{"api":{}}}' ;;
 *) exit 77 ;;
esac
`
	writeFixture(t, filepath.Join(dir, "docker"), strings.ReplaceAll(script, "FIXTURE_DIR", dir), 0o700)
	writeFixture(t, filepath.Join(dir, "container-state"), "exited", 0o600)
	// Preserve fake unit state, and model the container started by its supervisor.
	unitScript, err := os.ReadFile(filepath.Join(dir, "systemctl"))
	if err != nil {
		t.Fatal(err)
	}
	unitScript = []byte(strings.Replace(string(unitScript), `start) : > "$active_file"`, `start) printf running > "`+filepath.Join(dir, "container-state")+`"; : > "$active_file"`, 1))
	writeFixture(t, filepath.Join(dir, "systemctl"), string(unitScript), 0o700)
	if err := d.store.Update(func(c *model.Config) error {
		s := &c.Projects[0].Services[1]
		s.Docker = &model.DockerSpec{ComposeFile: filepath.Join(dir, "compose.yaml"), ProjectName: "fixture", Service: "api"}
		s.Dependencies = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "compose.yaml"), "services: {}", 0o600)
	return d, dir
}

func TestDockerRecreatePreservesStoppedStateAndRestartsManagedRunningService(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped", true: "running"}[running], func(t *testing.T) {
			d, dir := dockerLifecycleFixture(t)
			if running {
				if err := d.systemd.Start(context.Background(), readinessAPI); err != nil {
					t.Fatal(err)
				}
			}
			_, err := d.dockerAction(context.Background(), rawParams(t, dockerActionPayload{ServiceID: readinessAPI, Action: "recreate"}))
			if err != nil {
				t.Fatal(err)
			}
			state, _ := os.ReadFile(filepath.Join(dir, "container-state"))
			want := "created"
			if running {
				want = "running"
			}
			if string(state) != want {
				t.Fatalf("state=%s want=%s", state, want)
			}
			unit, err := d.systemd.Show(context.Background(), readinessAPI)
			if err != nil || (unit.ActiveState == "active") != running {
				t.Fatalf("unit=%+v err=%v", unit, err)
			}
			calls, _ := os.ReadFile(filepath.Join(dir, "docker-calls"))
			if !strings.Contains(string(calls), "up --no-start --no-build --force-recreate --no-deps api") {
				t.Fatal(string(calls))
			}
		})
	}
}

func TestDockerStopAndKillReachUnmanagedContainerAndDeletionFailsClosed(t *testing.T) {
	for _, action := range []string{"stop", "kill"} {
		t.Run(action, func(t *testing.T) {
			d, dir := dockerLifecycleFixture(t)
			writeFixture(t, filepath.Join(dir, "container-state"), "running", 0o600)
			if _, err := d.deleteService(context.Background(), rawParams(t, serviceIDPayload{ServiceID: readinessAPI})); err == nil {
				t.Fatal("deleted running unmanaged container")
			}
			if _, err := d.dockerAction(context.Background(), rawParams(t, dockerActionPayload{ServiceID: readinessAPI, Action: "recreate"})); err == nil {
				t.Fatal("recreated unmanaged container")
			}
			if err := d.performAction(context.Background(), action, readinessAPI); err != nil {
				t.Fatal(err)
			}
			calls, _ := os.ReadFile(filepath.Join(dir, "docker-calls"))
			if action == "kill" && !strings.Contains(string(calls), "stop --timeout 0 api") {
				t.Fatal("force kill did not force Docker shutdown")
			}
			if _, err := d.deleteService(context.Background(), rawParams(t, serviceIDPayload{ServiceID: readinessAPI})); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDockerStopAndRecreateFailuresCannotAuthorizeDeletionOrRestart(t *testing.T) {
	d, dir := dockerLifecycleFixture(t)
	if err := d.systemd.Start(context.Background(), readinessAPI); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "fail-stop"), "1", 0o600)
	if err := d.performAction(context.Background(), "stop", readinessAPI); err == nil {
		t.Fatal("failed Docker stop reported success")
	}
	if err := d.ensureStopped(context.Background(), readinessAPI); err == nil {
		t.Fatal("failed stop authorized replacement")
	}
	if err := os.Remove(filepath.Join(dir, "fail-stop")); err != nil {
		t.Fatal(err)
	}
	if err := d.systemd.Start(context.Background(), readinessAPI); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "fail-recreate"), "1", 0o600)
	if _, err := d.dockerAction(context.Background(), rawParams(t, dockerActionPayload{ServiceID: readinessAPI, Action: "recreate"})); err == nil {
		t.Fatal("failed recreate reported success")
	}
	unit, _ := d.systemd.Show(context.Background(), readinessAPI)
	if unit.ActiveState == "active" {
		t.Fatal("failed recreate restarted service")
	}
}

func TestDockerExecutionEditsRequireStoppedOldAndNewIdentities(t *testing.T) {
	d, dir := dockerLifecycleFixture(t)
	if err := d.systemd.Start(context.Background(), readinessAPI); err != nil {
		t.Fatal(err)
	}
	_, current, _ := d.store.Get().FindService(readinessAPI)
	current.Name = "Metadata is safe"
	if _, err := d.updateService(rawParams(t, servicePayload{Service: *current})); err != nil {
		t.Fatal(err)
	}
	current.Docker.ProjectName = "different-project"
	if _, err := d.updateService(rawParams(t, servicePayload{Service: *current})); err == nil {
		t.Fatal("changed active Docker identity")
	}
	_, stored, _ := d.store.Get().FindService(readinessAPI)
	if stored.Docker.ProjectName != "fixture" {
		t.Fatal("lost original container identity")
	}
	if err := d.systemd.Stop(context.Background(), readinessAPI); err != nil {
		t.Fatal(err)
	}
	if _, err := d.updateService(rawParams(t, servicePayload{Service: *current})); err == nil {
		t.Fatal("changed identity while orphan container running")
	}
	writeFixture(t, filepath.Join(dir, "container-state"), "exited", 0o600)
	if _, err := d.updateService(rawParams(t, servicePayload{Service: *current})); err != nil {
		t.Fatal(err)
	}
}

func TestDockerActionsInvalidateDisplayPoll(t *testing.T) {
	d, _ := dockerLifecycleFixture(t)
	d.lastDockerPoll[readinessAPI] = time.Now()
	if err := d.performAction(context.Background(), "stop", readinessAPI); err != nil {
		t.Fatal(err)
	}
	observed := d.observeServices(context.Background(), d.store.Get())[readinessAPI]
	if !observed.dockerPolled || observed.dockerRevision == 0 {
		t.Fatal("post-action Docker status remained cached")
	}
}

func TestOrphanWarningSurvivesCachedReconciliationAndClearsAfterStop(t *testing.T) {
	d, dir := dockerLifecycleFixture(t)
	writeFixture(t, filepath.Join(dir, "container-state"), "running", 0o600)
	d.reconcile(context.Background())
	for i := 0; i < 3; i++ {
		d.reconcile(context.Background())
		got := d.Snapshot().Runtime[readinessAPI]
		if got.Status != model.StatusUnhealthy || !strings.Contains(got.LastError, "outside OmaStack") {
			t.Fatalf("cached orphan warning lost: %+v", got)
		}
	}
	if err := d.performAction(context.Background(), "stop", readinessAPI); err != nil {
		t.Fatal(err)
	}
	d.reconcile(context.Background())
	if got := d.Snapshot().Runtime[readinessAPI]; got.Status != model.StatusStopped {
		t.Fatalf("post-stop=%+v", got)
	}
}

func TestDockerInspectionErrorRemainsVisibleBetweenPolls(t *testing.T) {
	d, dir := dockerLifecycleFixture(t)
	writeFixture(t, filepath.Join(dir, "docker"), "#!/bin/sh\nexit 77\n", 0o700)
	d.reconcile(context.Background())
	for i := 0; i < 2; i++ {
		d.reconcile(context.Background())
		got := d.Snapshot().Runtime[readinessAPI]
		if !got.Stale || got.LastError == "" {
			t.Fatalf("lost inspection error: %+v", got)
		}
	}
}
