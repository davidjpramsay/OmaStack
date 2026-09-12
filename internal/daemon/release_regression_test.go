package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"omastack/internal/control"
	"omastack/internal/deps"
	"omastack/internal/health"
	"omastack/internal/logs"
	"omastack/internal/model"
	"omastack/internal/supervise"
)

func writeFixture(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("fixture condition timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func installComposeFixture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "compose.yaml")
	writeFixture(t, path, "services: {}\n", 0o600)
	t.Setenv("OMASTACK_COMPOSE_MODEL", filepath.Join(dir, "model.json"))
	t.Setenv("OMASTACK_CONTAINER_STATE", filepath.Join(dir, "container.json"))
	writeFixture(t, filepath.Join(dir, "docker"), `#!/bin/sh
case "$*" in
  *" config --no-interpolate --format json") /usr/bin/cat >/dev/null; /usr/bin/cat "$OMASTACK_COMPOSE_MODEL" ;;
  *" ps --all --format json "*) printf '%s\n' '[{"ID":"container-1","State":"running"}]' ;;
  "inspect --format {{json .State}} container-1") /usr/bin/cat "$OMASTACK_CONTAINER_STATE" ;;
  *) exit 77 ;;
esac
`, 0o700)
	writeFixture(t, os.Getenv("OMASTACK_CONTAINER_STATE"), `{"Status":"running","StartedAt":"2026-09-08T00:00:00Z","Health":{"Status":"healthy"}}`, 0o600)
	return path
}

func TestComposeDependenciesAreManagedBeforeSelectedStart(t *testing.T) {
	d, dir := readinessFixture(t)
	compose := installComposeFixture(t, dir)
	writeFixture(t, os.Getenv("OMASTACK_COMPOSE_MODEL"), `{"services":{"db":{},"api":{"depends_on":{"db":{"condition":"service_healthy"},"optional":{"condition":"service_started","required":false}}}}}`, 0o600)
	services := d.store.Get().AllServices()
	for id, name := range map[string]string{readinessDB: "db", readinessAPI: "api"} {
		s := services[id]
		s.Docker = &model.DockerSpec{ComposeFile: compose, ProjectName: "fixture", Service: name}
		s.Dependencies = nil
		services[id] = s
	}
	graph, err := managedComposeGraph(context.Background(), services, []string{readinessAPI})
	if err != nil {
		t.Fatal(err)
	}
	order, err := deps.StartupOrder(graph, []string{readinessAPI})
	if err != nil || !reflect.DeepEqual(order, []string{readinessDB, readinessAPI}) {
		t.Fatalf("order=%v err=%v", order, err)
	}
	if graph[readinessAPI].Dependencies[0].Condition != "healthy" || len(services[readinessAPI].Dependencies) != 0 {
		t.Fatal("readiness weakened or source mutated")
	}
	delete(services, readinessDB)
	if _, err := managedComposeGraph(context.Background(), services, []string{readinessAPI}); err == nil || !strings.Contains(err.Error(), "add Compose dependency") {
		t.Fatalf("unmanaged required dependency accepted: %v", err)
	}
}

func TestComposeCyclesAndUnrelatedFiles(t *testing.T) {
	d, dir := readinessFixture(t)
	compose := installComposeFixture(t, dir)
	writeFixture(t, os.Getenv("OMASTACK_COMPOSE_MODEL"), `{"services":{"db":{"depends_on":{"api":{"condition":"service_started"}}},"api":{"depends_on":{"db":{"condition":"service_completed_successfully"}}}}}`, 0o600)
	services := d.store.Get().AllServices()
	for id, name := range map[string]string{readinessDB: "db", readinessAPI: "api"} {
		s := services[id]
		s.Docker = &model.DockerSpec{ComposeFile: compose, Service: name}
		s.Dependencies = nil
		services[id] = s
	}
	graph, err := managedComposeGraph(context.Background(), services, []string{readinessAPI})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = deps.StartupOrder(graph, []string{readinessAPI}); err == nil {
		t.Fatal("native cycle accepted")
	}
	s := services[readinessAPI]
	s.Docker = nil
	s.Dependencies = nil
	services[readinessAPI] = s
	s = services[readinessDB]
	s.Docker.ComposeFile = "/missing/unrelated/compose.yaml"
	services[readinessDB] = s
	if _, err = managedComposeGraph(context.Background(), services, []string{readinessAPI}); err != nil {
		t.Fatalf("unrelated file blocked host start: %v", err)
	}
}

func TestDockerCompletionAndNativeHealth(t *testing.T) {
	d, dir := readinessFixture(t)
	compose := installComposeFixture(t, dir)
	if err := d.store.Update(func(c *model.Config) error {
		c.Projects[0].Services[0].Docker = &model.DockerSpec{ComposeFile: compose, Service: "db"}
		c.Projects[0].Services[0].Health = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.systemd.Start(context.Background(), readinessDB); err != nil {
		t.Fatal(err)
	}
	if err := d.waitHealthy(context.Background(), readinessDB, time.Second); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"unhealthy", ""} {
		writeFixture(t, os.Getenv("OMASTACK_CONTAINER_STATE"), fmt.Sprintf(`{"Status":"running","StartedAt":"2026-09-08T00:00:00Z","Health":{"Status":%q}}`, status), 0o600)
		if err := d.waitHealthy(context.Background(), readinessDB, time.Second); err == nil {
			t.Fatalf("accepted native health %q", status)
		}
	}
	svc := d.store.Get().AllServices()[readinessDB]
	for _, code := range []int{0, 1} {
		writeFixture(t, os.Getenv("OMASTACK_CONTAINER_STATE"), fmt.Sprintf(`{"Status":"exited","StartedAt":"2026-09-08T00:00:00Z","ExitCode":%d}`, code), 0o600)
		err := waitDockerCondition(context.Background(), svc, "docker_completed")
		if (err == nil) != (code == 0) {
			t.Fatalf("completion code=%d err=%v", code, err)
		}
	}
}

func TestAsyncHealthRejectsOldRunsAndDoesNotQueueGenerations(t *testing.T) {
	for _, containerRestart := range []bool{false, true} {
		t.Run(fmt.Sprint("docker=", containerRestart), func(t *testing.T) {
			d, dir := readinessFixture(t)
			d.healthChecker = health.NewChecker(1)
			probe := filepath.Join(dir, "probe")
			writeFixture(t, probe, `#!/bin/sh
printf 'probe\n' >> "$OMASTACK_PROBE_CALLS"
while [ ! -f "$OMASTACK_PROBE_RELEASE" ]; do /usr/bin/sleep 0.01; done
`, 0o700)
			t.Setenv("OMASTACK_PROBE_CALLS", filepath.Join(dir, "probe-calls"))
			t.Setenv("OMASTACK_PROBE_RELEASE", filepath.Join(dir, "release"))
			compose := ""
			if containerRestart {
				compose = installComposeFixture(t, dir)
			}
			if err := d.store.Update(func(c *model.Config) error {
				s := &c.Projects[0].Services[0]
				s.Health.Command.Executable = probe
				s.Health.TimeoutSeconds = 3
				s.Health.IntervalSeconds = 3
				if containerRestart {
					s.Docker = &model.DockerSpec{ComposeFile: compose, Service: "db"}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := d.systemd.Start(context.Background(), readinessDB); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			service := d.store.Get().AllServices()[readinessDB]
			d.scheduleHealth(ctx, "Fixture", service, nil)
			eventually(t, func() bool { _, err := os.Stat(os.Getenv("OMASTACK_PROBE_CALLS")); return err == nil })
			for i := 0; i < 20; i++ {
				if containerRestart {
					// Same container ID and supervisor; only the container start changes.
					writeFixture(t, os.Getenv("OMASTACK_CONTAINER_STATE"), fmt.Sprintf(`{"Status":"running","StartedAt":"2026-09-08T00:01:%02dZ","Health":{"Status":"healthy"}}`, i), 0o600)
				} else {
					started := time.Now().Add(time.Duration(i) * time.Second)
					if err := supervise.WriteRuntime(d.paths.RuntimeServicesDir, supervise.RuntimeRecord{ServiceID: readinessDB, SupervisorPID: 700, PID: 800 + i, StartedAt: &started}); err != nil {
						t.Fatal(err)
					}
				}
				d.scheduleHealth(ctx, "Fixture", service, nil)
			}
			writeFixture(t, os.Getenv("OMASTACK_PROBE_RELEASE"), "go", 0o600)
			eventually(t, func() bool { d.mu.RLock(); defer d.mu.RUnlock(); return len(d.healthRunning) == 0 })
			calls, _ := os.ReadFile(os.Getenv("OMASTACK_PROBE_CALLS"))
			if strings.Count(string(calls), "probe") != 1 {
				t.Fatalf("obsolete work accumulated: %s", calls)
			}
			if d.healthStates.Get(readinessDB).Status == "healthy" {
				t.Fatal("earlier run's result committed")
			}
		})
	}
}

func TestDuplicateProjectDoesNotReuseRoutes(t *testing.T) {
	d, _ := readinessFixture(t)
	if err := d.store.Update(func(c *model.Config) error {
		c.Projects[0].Services[0].Route = &model.Route{Hostname: "db.localhost", TargetPort: 8080}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	project := d.store.Get().Projects[0]
	result, err := d.duplicateProject(rawParams(t, projectIDPayload{ProjectID: project.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]string)["notice"] == "" {
		t.Fatal("route omission was silent")
	}
	config := d.store.Get()
	if config.Projects[0].Services[0].Route == nil || config.Projects[1].Services[0].Route != nil {
		t.Fatal("duplicate route collision or original changed")
	}
}

func TestLogsMergeNewestAcrossServicesAndReportByteLimit(t *testing.T) {
	d, dir := readinessFixture(t)
	writeFixture(t, filepath.Join(dir, "journalctl"), `#!/bin/sh
printf '%s\n' "$*" >> "$OMASTACK_LOG_CALLS"
case "$*" in
  *33333333-3333-4333-8333-333333333333*) /usr/bin/cat "$OMASTACK_LOG_API" ;;
  *) /usr/bin/cat "$OMASTACK_LOG_DB" ;;
esac
`, 0o700)
	t.Setenv("OMASTACK_LOG_CALLS", filepath.Join(dir, "log-calls"))
	t.Setenv("OMASTACK_LOG_DB", filepath.Join(dir, "db-logs"))
	t.Setenv("OMASTACK_LOG_API", filepath.Join(dir, "api-logs"))
	journal := func(start, count int, message string) string {
		var b strings.Builder
		for i := start + count - 1; i >= start; i-- {
			line, _ := json.Marshal(map[string]string{"__REALTIME_TIMESTAMP": fmt.Sprint(i * 1000000), "MESSAGE": "stdout\t" + message})
			b.Write(line)
			b.WriteByte('\n')
		}
		return b.String()
	}
	writeFixture(t, os.Getenv("OMASTACK_LOG_DB"), journal(1, 20, "old"), 0o600)
	writeFixture(t, os.Getenv("OMASTACK_LOG_API"), journal(100, 20, "new"), 0o600)
	value, err := d.readLogs(context.Background(), control.LogsParams{Target: "all", Lines: 10, Metadata: true})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(logs.Result)
	if len(result.Entries) != 10 || result.Entries[0].Timestamp.Unix() != 110 {
		t.Fatalf("merge lost busy source: %+v", result)
	}
	writeFixture(t, os.Getenv("OMASTACK_LOG_API"), journal(100, 200, strings.Repeat("<>&", 4000)), 0o600)
	value, err = d.readLogs(context.Background(), control.LogsParams{Target: "all", Lines: 50000, Metadata: true})
	if err != nil {
		t.Fatal(err)
	}
	result = value.(logs.Result)
	encoded, _ := json.Marshal(result.Entries)
	if !result.Truncated || result.Notice == "" || len(encoded) > control.MaxMessageBytes/2 {
		t.Fatalf("limit not reported/bounded: %d %+v", len(encoded), result.Truncated)
	}
	if result.Entries[len(result.Entries)-1].Timestamp.Unix() != 299 {
		t.Fatal("newest record dropped")
	}
	calls, _ := os.ReadFile(os.Getenv("OMASTACK_LOG_CALLS"))
	if !strings.Contains(string(calls), "--lines=50000") {
		t.Fatal("configured line limit ignored")
	}
}

func TestPollingTimeoutPreserves128PriorStates(t *testing.T) {
	d, dir := readinessFixture(t)
	writeFixture(t, filepath.Join(dir, "systemctl"), "#!/bin/sh\nexec /usr/bin/sleep 30\n", 0o700)
	if err := d.store.Update(func(c *model.Config) error {
		base := c.Projects[0].Services[0]
		base.Health = nil
		c.Projects[0].Services = nil
		for i := 1; i <= 64; i++ {
			s := base
			s.ID = fmt.Sprintf("%08x-2222-4222-8222-222222222222", i)
			c.Projects[0].Services = append(c.Projects[0].Services, s)
		}
		other := model.Project{ID: "44444444-4444-4444-8444-444444444444", Name: "Second fixture"}
		for i := 65; i <= 128; i++ {
			s := base
			s.ID = fmt.Sprintf("%08x-2222-4222-8222-222222222222", i)
			other.Services = append(other.Services, s)
		}
		c.Projects = append(c.Projects, other)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d.snapshot.Runtime = map[string]model.ServiceRuntime{}
	for _, s := range d.store.Get().AllServices() {
		d.snapshot.Runtime[s.ID] = model.ServiceRuntime{ServiceID: s.ID, Status: model.StatusRunning, MemoryMB: 123}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	d.reconcile(ctx)
	if time.Since(started) > time.Second {
		t.Fatal("poll cycle did not honor deadline")
	}
	for _, runtime := range d.Snapshot().Runtime {
		if !runtime.Stale || runtime.Status != model.StatusRunning || runtime.MemoryMB != 123 {
			t.Fatalf("invented stop from timeout: %+v", runtime)
		}
	}
}
