package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"omastack/internal/control"
	"omastack/internal/docker"
	"omastack/internal/logs"
	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/redact"
	"omastack/internal/systemd"
)

func TestStatusReconciliation(t *testing.T) {
	cases := []struct {
		active, sub string
		want        model.ServiceStatus
	}{
		{"activating", "start", model.StatusStarting}, {"active", "running", model.StatusRunning},
		{"deactivating", "stop", model.StatusStopping}, {"failed", "failed", model.StatusCrashed}, {"inactive", "dead", model.StatusStopped},
	}
	for _, test := range cases {
		if got := statusFor(test.active, test.sub); got != test.want {
			t.Errorf("%s/%s = %s", test.active, test.sub, got)
		}
	}
}

func TestApplyUnitOutcomeCleanStopOverridesTerminatedRecord(t *testing.T) {
	code := -1
	runtime := model.ServiceRuntime{Status: model.StatusStopped, PID: 1234, UptimeSeconds: 42, ExitCode: &code, Signal: "terminated", LastError: "signal: terminated"}
	applyUnitOutcome(&runtime, systemd.UnitState{ActiveState: "inactive", SubState: "dead", Result: "success"})
	if runtime.Status != model.StatusStopped || runtime.PID != 0 || runtime.UptimeSeconds != 0 || runtime.ExitCode == nil || *runtime.ExitCode != 0 || runtime.Signal != "" || runtime.LastError != "" {
		t.Fatalf("clean stop runtime = %#v", runtime)
	}
}

func TestApplyUnitOutcomeForceKillUsesSystemdSignal(t *testing.T) {
	runtime := model.ServiceRuntime{Status: model.StatusCrashed, PID: 1234, UptimeSeconds: 42}
	applyUnitOutcome(&runtime, systemd.UnitState{ActiveState: "failed", SubState: "failed", Result: "signal", ExecMainCode: 2, ExecMainStatus: 9})
	if runtime.PID != 0 || runtime.UptimeSeconds != 0 || runtime.ExitCode != nil || runtime.Signal != "SIGKILL" || runtime.LastError != "systemd result: signal (SIGKILL)" {
		t.Fatalf("killed runtime = %#v", runtime)
	}
}

func TestApplyDockerOutcomePreservesIntentionalUnitStop(t *testing.T) {
	zero := 0
	runtime := model.ServiceRuntime{Status: model.StatusStopped, ExitCode: &zero}
	applyDockerOutcome(&runtime, docker.ContainerState{State: "exited", ExitCode: 137}, true)
	if runtime.Status != model.StatusStopped || runtime.ExitCode == nil || *runtime.ExitCode != 0 || runtime.Signal != "" || runtime.LastError != "" {
		t.Fatalf("intentional Docker stop runtime = %#v", runtime)
	}
}

func TestApplyDockerOutcomeReportsUnexpectedContainerExit(t *testing.T) {
	runtime := model.ServiceRuntime{Status: model.StatusStopped}
	applyDockerOutcome(&runtime, docker.ContainerState{State: "exited", ExitCode: 137}, false)
	if runtime.Status != model.StatusCrashed || runtime.ExitCode == nil || *runtime.ExitCode != 137 {
		t.Fatalf("unexpected Docker exit runtime = %#v", runtime)
	}
}

func TestApplyDockerOutcomeDoesNotHideSupervisorFailure(t *testing.T) {
	runtime := model.ServiceRuntime{Status: model.StatusCrashed, LastError: "systemd result: exit-code"}
	applyDockerOutcome(&runtime, docker.ContainerState{State: "exited", ExitCode: 0}, false)
	if runtime.Status != model.StatusCrashed || runtime.LastError == "" {
		t.Fatalf("supervisor failure runtime = %#v", runtime)
	}
}

func TestApplyDockerOutcomeUsesRunningHealth(t *testing.T) {
	runtime := model.ServiceRuntime{Status: model.StatusStopped}
	applyDockerOutcome(&runtime, docker.ContainerState{State: "running", Health: "unhealthy"}, false)
	if runtime.Status != model.StatusUnhealthy {
		t.Fatalf("unhealthy Docker runtime = %#v", runtime)
	}
}

func TestProjectDeletionIgnoresDependenciesInsideRemovedProject(t *testing.T) {
	config := model.DefaultConfig()
	config.Projects = []model.Project{
		{ID: "11111111-1111-4111-8111-111111111111", Name: "removed", Services: []model.Service{
			{ID: "22222222-2222-4222-8222-222222222222", Name: "db"},
			{ID: "33333333-3333-4333-8333-333333333333", Name: "api", Dependencies: []model.Dependency{{ServiceID: "22222222-2222-4222-8222-222222222222"}}},
		}},
		{ID: "44444444-4444-4444-8444-444444444444", Name: "kept", Services: []model.Service{{ID: "55555555-5555-4555-8555-555555555555", Name: "worker"}}},
	}
	removed := map[string]bool{"22222222-2222-4222-8222-222222222222": true, "33333333-3333-4333-8333-333333333333": true}
	if blocker := externalProjectDependency(config, config.Projects[0].ID, removed); blocker != "" {
		t.Fatalf("internal dependency blocked project deletion: %s", blocker)
	}
	config.Projects[1].Services[0].Dependencies = []model.Dependency{{ServiceID: "22222222-2222-4222-8222-222222222222"}}
	if blocker := externalProjectDependency(config, config.Projects[0].ID, removed); blocker != "worker" {
		t.Fatalf("external dependency blocker = %q", blocker)
	}
}

func TestBoundLogEntriesKeepsNewestWithinBudget(t *testing.T) {
	entries := []logs.Entry{
		{ServiceID: "one", Message: strings.Repeat("a", 400)},
		{ServiceID: "two", Message: strings.Repeat("b", 400)},
		{ServiceID: "three", Message: "newest"},
	}
	bounded := boundLogEntries(entries, 1300)
	if len(bounded) == 0 || bounded[len(bounded)-1].ServiceID != "three" {
		t.Fatalf("newest entry not retained: %#v", bounded)
	}
	total := 0
	for _, entry := range bounded {
		total += 256 + len(entry.ServiceID) + len(entry.Service) + len(entry.Stream) + len(entry.Message)
	}
	if total > 1300 {
		t.Fatalf("bounded log cost = %d", total)
	}
}

func TestSerializedOperationClassification(t *testing.T) {
	for _, method := range []string{"start", "config.import", "project.delete", "service.update", "docker.action", "cleanup"} {
		if !serializedOperation(method) {
			t.Errorf("%s should be serialized", method)
		}
	}
	for _, method := range []string{"status", "logs", "config.get", "doctor", "ui.visibility"} {
		if serializedOperation(method) {
			t.Errorf("%s should remain concurrent", method)
		}
	}
}

func TestStopAndKillInterruptLongOperations(t *testing.T) {
	for _, method := range []string{"stop", "kill", "restart"} {
		if !interruptsOperation(method) {
			t.Errorf("%s should interrupt", method)
		}
	}
	for _, method := range []string{"start", "config.import", "project.delete"} {
		if interruptsOperation(method) {
			t.Errorf("%s should not interrupt", method)
		}
	}
}

func TestStopCancelsActiveSerializedOperation(t *testing.T) {
	daemon := &Daemon{}
	startCtx, finishStart, err := daemon.beginSerializedOperation(context.Background(), "start")
	if err != nil {
		t.Fatal(err)
	}
	type acquired struct {
		finish func()
		err    error
	}
	stopAcquired := make(chan acquired, 1)
	go func() {
		_, finish, err := daemon.beginSerializedOperation(context.Background(), "stop")
		stopAcquired <- acquired{finish: finish, err: err}
	}()
	select {
	case <-startCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel active start")
	}
	finishStart()
	select {
	case result := <-stopAcquired:
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.finish()
	case <-time.After(time.Second):
		t.Fatal("stop did not acquire operation gate")
	}
}

func TestResolveTargetsRejectsAmbiguousNames(t *testing.T) {
	config := model.DefaultConfig()
	config.Projects = []model.Project{{ID: "11111111-1111-4111-8111-111111111111", Name: "one", Services: []model.Service{{ID: "22222222-2222-4222-8222-222222222222", Name: "api"}}}, {ID: "33333333-3333-4333-8333-333333333333", Name: "two", Services: []model.Service{{ID: "44444444-4444-4444-8444-444444444444", Name: "api"}}}}
	if _, err := resolveTargets(config, "api"); err == nil {
		t.Fatal("ambiguous target accepted")
	}
	got, err := resolveTargets(config, "one/api")
	if err != nil || !reflect.DeepEqual(got, []string{"22222222-2222-4222-8222-222222222222"}) {
		t.Fatalf("resolved=%v err=%v", got, err)
	}
}

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	base := t.TempDir()
	resolved := paths.Paths{
		ConfigDir: filepath.Join(base, "config"), ConfigFile: filepath.Join(base, "config", "config.json"), ExportDir: filepath.Join(base, "config", "exports"),
		RuntimeDir: filepath.Join(base, "runtime"), SocketFile: filepath.Join(base, "runtime", "control.sock"), SnapshotFile: filepath.Join(base, "runtime", "state.json"),
		StateDir: filepath.Join(base, "state"), RuntimeServicesDir: filepath.Join(base, "state", "services"), CacheDir: filepath.Join(base, "cache"),
	}
	instance, err := New(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func rawParams(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestConfigurationMutationWorkflow(t *testing.T) {
	instance := testDaemon(t)
	ctx := context.Background()

	ping := instance.handle(ctx, control.Request{ID: "ping", Method: "ping"})
	if !ping.OK {
		t.Fatalf("ping = %#v", ping)
	}
	unknown := instance.handle(ctx, control.Request{ID: "unknown", Method: "missing.method"})
	if unknown.OK || unknown.Error == nil {
		t.Fatalf("unknown method = %#v", unknown)
	}

	settings := model.DefaultConfig().Settings
	settings.LogBufferLines = 4321
	response := instance.handle(ctx, control.Request{ID: "settings", Method: "settings.update", Params: rawParams(t, settingsPayload{Settings: settings})})
	if !response.OK || instance.store.Get().Settings.LogBufferLines != 4321 {
		t.Fatalf("settings response=%#v config=%#v", response, instance.store.Get().Settings)
	}

	created, err := instance.createProject(rawParams(t, projectPayload{Project: model.Project{Name: "App <dev>", Icon: "󰆍"}}))
	if err != nil {
		t.Fatal(err)
	}
	projectID := created.(map[string]string)["id"]
	service := model.Service{
		Name: "API <local>", WorkingDirectory: t.TempDir(), Command: model.CommandSpec{Executable: "/usr/bin/true"},
		Environment: map[string]model.EnvValue{"TOKEN": {Value: "private-value", Secret: true}},
	}
	created, err = instance.createService(rawParams(t, servicePayload{ProjectID: projectID, Service: service}))
	if err != nil {
		t.Fatal(err)
	}
	serviceID := created.(map[string]string)["id"]
	project, storedService, ok := instance.store.Get().FindService(serviceID)
	if !ok || project.ID != projectID || storedService.StopSignal != "SIGTERM" || storedService.GracefulStopSeconds != 10 {
		t.Fatalf("stored service = %#v, project=%#v, ok=%v", storedService, project, ok)
	}

	updated := *storedService
	updated.Name = "API renamed"
	updated.Environment = map[string]model.EnvValue{"TOKEN": {Value: redact.Mask, Secret: true}}
	if _, err := instance.updateService(rawParams(t, servicePayload{Service: updated})); err != nil {
		t.Fatal(err)
	}
	_, storedService, _ = instance.store.Get().FindService(serviceID)
	if storedService.Environment["TOKEN"].Value != "private-value" {
		t.Fatalf("masked secret was not preserved: %#v", storedService.Environment)
	}

	duplicated, err := instance.duplicateProject(rawParams(t, projectIDPayload{ProjectID: projectID}))
	if err != nil {
		t.Fatal(err)
	}
	duplicateID := duplicated.(map[string]string)["id"]
	if _, err := instance.reorderProjects(rawParams(t, projectOrderPayload{ProjectIDs: []string{duplicateID, projectID}})); err != nil {
		t.Fatal(err)
	}
	if instance.store.Get().Projects[0].ID != duplicateID {
		t.Fatal("project reorder was not persisted")
	}

	exported, err := instance.exportConfig()
	if err != nil {
		t.Fatal(err)
	}
	exportPath := exported.(map[string]string)["path"]
	info, err := os.Stat(exportPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("export info=%v err=%v", info, err)
	}

	redacted := instance.handle(ctx, control.Request{ID: "config", Method: "config.get"})
	data, err := json.Marshal(redacted.Result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-value") || !strings.Contains(string(data), redact.Mask) {
		t.Fatalf("config.get leaked secret: %s", data)
	}
}

func TestCleanupAndParameterErrorClassification(t *testing.T) {
	instance := testDaemon(t)
	orphanID := "66666666-6666-4666-8666-666666666666"
	orphan := filepath.Join(instance.paths.RuntimeServicesDir, orphanID+".json")
	invalid := filepath.Join(instance.paths.RuntimeServicesDir, "not-an-id.json")
	for _, path := range []string{orphan, invalid} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := instance.cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.(map[string]any)["removed"].([]string)) != 1 {
		t.Fatalf("cleanup = %#v", result)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan remains: %v", err)
	}
	if _, err := os.Stat(invalid); err != nil {
		t.Fatalf("invalid record should be preserved: %v", err)
	}

	var target control.TargetParams
	if err := decodeParams(json.RawMessage(`{"target":"x","extra":true}`), &target); err == nil {
		t.Fatal("unknown parameter accepted")
	}
	for message, want := range map[string]string{
		"thing not found": "not-found", "ambiguous name": "ambiguous-target", "invalid value": "invalid-argument", "dependency cycle": "dependency-cycle", "boom": "operation-failed",
	} {
		if got := errorCode(errors.New(message)); got != want {
			t.Errorf("%q => %q, want %q", message, got, want)
		}
	}
}
