package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omastack/internal/model"
)

const testProjectID = "11111111-1111-4111-8111-111111111111"
const testServiceID = "22222222-2222-4222-8222-222222222222"

func validProject() model.Project {
	return model.Project{ID: testProjectID, Name: "Example", Services: []model.Service{{
		ID: testServiceID, Name: "api", Command: model.CommandSpec{Executable: "/usr/bin/true"},
		WorkingDirectory: "/tmp", Restart: model.RestartPolicy{Mode: "never"}, StopSignal: "SIGTERM", GracefulStopSeconds: 10,
	}}}
}

func TestDecodeConfigMigratesVersionZero(t *testing.T) {
	legacy := `{"projects":[{"id":"` + testProjectID + `","name":"Example","order":0,"services":[{"id":"` + testServiceID + `","name":"api","command":{"executable":"/usr/bin/true"},"workingDirectory":"/tmp","restart":{"mode":"never"},"stopSignal":"SIGTERM","gracefulStopSeconds":10}]}]}`
	config, err := DecodeConfig([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != model.CurrentConfigVersion {
		t.Fatalf("version = %d", config.Version)
	}
	if config.Settings.HistorySamples != 60 || len(config.Projects) != 1 {
		t.Fatalf("migration lost defaults or projects: %#v", config)
	}
}

func TestDecodeConfigRejectsUnknownFields(t *testing.T) {
	config := model.DefaultConfig()
	config.Projects = []model.Project{validProject()}
	data := `{"version":1,"settings":{"pollIntervalSeconds":2,"historySamples":60,"logBufferLines":2000,"notifications":{"unhealthy":true,"crashed":true,"recovered":true},"proxy":{"enabled":false,"listenHost":"127.0.0.1","httpPort":8088,"httpsPort":8448}},"projects":[],"surprise":true}`
	if _, err := DecodeConfig([]byte(data)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestConfigStoreWritesMode0600AndPreservesOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := OpenConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(config *model.Config) error { config.Projects = []model.Project{validProject()}; return nil }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if err := store.Update(func(config *model.Config) error { config.Settings.PollIntervalSeconds = 0; return nil }); err == nil {
		t.Fatal("invalid update succeeded")
	}
	if store.Get().Settings.PollIntervalSeconds == 0 {
		t.Fatal("failed update mutated live config")
	}
}

func TestOpenConfigRefusesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenConfig(path); err == nil {
		t.Fatal("config symlink accepted")
	}
}

func TestFullStackExampleParses(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "full-stack", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Projects) != 1 || len(config.Projects[0].Services) != 4 {
		t.Fatalf("example shape = %#v", config.Projects)
	}
}
