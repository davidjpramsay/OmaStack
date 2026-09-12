package validate

import (
	"fmt"
	"strings"
	"testing"

	"omastack/internal/model"
)

func TestLocalDomainValidation(t *testing.T) {
	for _, valid := range []string{"frontend.myapp.test", "api.localhost", "a-b.example.test."} {
		if !LocalHostname(valid) {
			t.Errorf("rejected %q", valid)
		}
	}
	for _, invalid := range []string{"example.com", "-bad.test", "bad_.test", "../api.test", "api.test:8080"} {
		if LocalHostname(invalid) {
			t.Errorf("accepted %q", invalid)
		}
	}
}

func TestCommandAndRouteValidation(t *testing.T) {
	if err := Command(model.CommandSpec{Executable: "/usr/bin/printf", Arguments: []string{"$(touch /tmp/nope)", "a; b"}}); err != nil {
		t.Fatal("arguments should be opaque: ", err)
	}
	if err := Command(model.CommandSpec{Executable: "/usr/bin/printf", Arguments: []string{"bad\x00arg"}}); err == nil {
		t.Fatal("NUL argument accepted")
	}
	service := model.Service{ID: "22222222-2222-4222-8222-222222222222", Name: "web", Command: model.CommandSpec{Executable: "/usr/bin/true"}, WorkingDirectory: "/tmp", StopSignal: "SIGTERM", GracefulStopSeconds: 10, Restart: model.RestartPolicy{Mode: "never"}, Route: &model.Route{Hostname: "web.app.test", TargetPort: 3000, HTTPS: true}}
	if err := Service(&service); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("unavailable HTTPS accepted: %v", err)
	}
}

func TestHTTPSHealthCannotDisableCertificateVerification(t *testing.T) {
	health := model.HealthCheck{Type: "https", IntervalSeconds: 10, TimeoutSeconds: 3, Retries: 2, HTTP: &model.HTTPCheck{URL: "https://api.localhost/health", ExpectedStatus: 200, SkipTLSVerify: true}}
	if err := Health(&health); err == nil || !strings.Contains(err.Error(), "cannot be disabled") {
		t.Fatalf("insecure HTTPS accepted: %v", err)
	}
}

func TestConfigurationCountAndSerializedSizeBounds(t *testing.T) {
	tooMany := model.DefaultConfig()
	for index := 0; index < 65; index++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		tooMany.Projects = append(tooMany.Projects, model.Project{ID: id, Name: "project", Services: []model.Service{}})
	}
	if err := Config(tooMany); err == nil || !strings.Contains(err.Error(), "64 projects") {
		t.Fatalf("project count bound missing: %v", err)
	}

	large := model.DefaultConfig()
	projectID, _ := NewID()
	serviceID, _ := NewID()
	environment := map[string]model.EnvValue{}
	for index := 0; index < 9; index++ {
		environment[fmt.Sprintf("VALUE_%d", index)] = model.EnvValue{Value: strings.Repeat("x", 65536)}
	}
	large.Projects = []model.Project{{ID: projectID, Name: "large", Services: []model.Service{{
		ID: serviceID, Name: "service", Command: model.CommandSpec{Executable: "/usr/bin/true"}, WorkingDirectory: "/tmp",
		Environment: environment, Restart: model.RestartPolicy{Mode: "never"}, StopSignal: "SIGTERM", GracefulStopSeconds: 10,
	}}}}
	if err := Config(large); err == nil || !strings.Contains(err.Error(), "512 KiB") {
		t.Fatalf("serialized size bound missing: %v", err)
	}
}

func TestDockerServiceDoesNotRequireHostCommand(t *testing.T) {
	serviceID, _ := NewID()
	service := model.Service{
		ID: serviceID, Name: "postgres", Docker: &model.DockerSpec{ComposeFile: "/tmp/compose.yaml", Service: "postgres"},
		Restart: model.RestartPolicy{Mode: "never"}, StopSignal: "SIGTERM", GracefulStopSeconds: 10,
	}
	if err := Service(&service); err != nil {
		t.Fatalf("Docker service rejected: %v", err)
	}
	service.Shell = &model.ShellSpec{Enabled: true, Shell: "/bin/sh", Command: "true"}
	if err := Service(&service); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("Docker and shell combination accepted: %v", err)
	}
}

func TestConfigRejectsCyclesAndMissingReadiness(t *testing.T) {
	c := model.DefaultConfig()
	p, _ := NewID()
	a, _ := NewID()
	b, _ := NewID()
	service := func(id string) model.Service {
		return model.Service{ID: id, Name: id, WorkingDirectory: "/tmp", Command: model.CommandSpec{Executable: "/usr/bin/true"}, Restart: model.RestartPolicy{Mode: "never"}, StopSignal: "SIGTERM", GracefulStopSeconds: 10}
	}
	c.Projects = []model.Project{{ID: p, Name: "graph", Services: []model.Service{service(a), service(b)}}}
	c.Projects[0].Services[0].Dependencies = []model.Dependency{{ServiceID: b, Condition: "started"}}
	if err := Config(c); err != nil {
		t.Fatal(err)
	}
	c.Projects[0].Services[1].Dependencies = []model.Dependency{{ServiceID: a, Condition: "started"}}
	if err := Config(c); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	c.Projects[0].Services[1].Dependencies = nil
	c.Projects[0].Services[0].Dependencies[0].Condition = "healthy"
	if err := Config(c); err == nil || !strings.Contains(err.Error(), "no health check") {
		t.Fatalf("missing readiness accepted: %v", err)
	}
	c.Projects[0].Services[1].Docker = &model.DockerSpec{ComposeFile: "/tmp/compose.yaml", Service: "db"}
	if err := Config(c); err != nil {
		t.Fatalf("native Docker readiness rejected: %v", err)
	}
}

func TestRouteUniquenessUsesCanonicalHostname(t *testing.T) {
	for _, alternate := range []string{"APP.localhost.", " app.localhost "} {
		c := model.DefaultConfig()
		p, _ := NewID()
		a, _ := NewID()
		b, _ := NewID()
		service := model.Service{ID: a, Name: "a", Command: model.CommandSpec{Executable: "/usr/bin/true"}, WorkingDirectory: "/tmp", Restart: model.RestartPolicy{Mode: "never"}, StopSignal: "SIGTERM", GracefulStopSeconds: 10, Route: &model.Route{Hostname: "app.localhost"}}
		other := service
		other.ID = b
		other.Route = &model.Route{Hostname: alternate}
		c.Projects = []model.Project{{ID: p, Name: "routes", Services: []model.Service{service, other}}}
		if err := Config(c); err == nil || !strings.Contains(err.Error(), "route") {
			t.Fatalf("equivalent hostname accepted: %q %v", alternate, err)
		}
	}
}
