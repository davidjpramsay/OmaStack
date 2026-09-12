package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestComposeDiscoveryUsesStdinWithoutInterpolation(t *testing.T) {
	want := []string{"compose", "-f", "-", "config", "--no-interpolate", "--format", "json"}
	if got := composeConfigArguments(); !reflect.DeepEqual(got, want) {
		t.Fatalf("compose discovery arguments = %#v", got)
	}
}

func installFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	logPath := filepath.Join(directory, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OMASTACK_DOCKER_TEST_LOG"
case "$*" in
  "version --format {{.Server.Version}}")
    echo '27.0.0'
    ;;
  "compose version")
    echo 'Docker Compose version v2'
    ;;
  *" config --no-interpolate --format json")
    cat >/dev/null
    echo '{"services":{"api":{"command":["serve","--port","3000"],"ports":[{"published":"3000","target":3000}]}}}'
    ;;
  *" ps --all --format json api")
    echo '[{"ID":"container-id","Name":"stack-api-1","State":"running","Health":"healthy","ExitCode":0,"Publishers":[{"PublishedPort":3000}]}]'
    ;;
  "inspect --format {{json .State}} container-id")
    echo '{"Status":"running","StartedAt":"2026-09-08T01:00:00Z","ExitCode":0,"Health":{"Status":"healthy"}}'
    ;;
  "stats --no-stream --format {{json .}} container-id")
    echo '{"CPUPerc":"2.5%","MemUsage":"32MiB / 1GiB"}'
    ;;
esac
`
	dockerPath := filepath.Join(directory, "docker")
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := `#!/bin/sh
printf '%s\n' "$*" >> "$OMASTACK_DOCKER_TEST_LOG"
`
	if err := os.WriteFile(filepath.Join(directory, "uwsm-app"), []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMASTACK_DOCKER_TEST_LOG", logPath)
	return directory, logPath
}

func TestDockerCommandWorkflows(t *testing.T) {
	directory, logPath := installFakeDocker(t)
	ctx := context.Background()
	availability := Detect(ctx)
	if !availability.Installed || !availability.DaemonRunning || !availability.ComposeAvailable || availability.Error != "" {
		t.Fatalf("availability = %#v", availability)
	}

	composePath := filepath.Join(directory, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  api:\n    image: fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportCompose(ctx, composePath)
	if err != nil || len(imported) != 1 || imported[0].Name != "api" || !reflect.DeepEqual(imported[0].Command, []string{"serve", "--port", "3000"}) || !reflect.DeepEqual(imported[0].Ports, []string{"3000:3000"}) {
		t.Fatalf("imported=%#v err=%v", imported, err)
	}
	state, err := Inspect(ctx, composePath, "stack", "api")
	if err != nil || state.ID != "container-id" || state.State != "running" || state.StartedAt.IsZero() || state.CPU != 2.5 || state.MemoryMB != 32 || !reflect.DeepEqual(state.Published, []int{3000}) {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	for _, action := range []string{"start", "stop", "restart", "rebuild", "recreate"} {
		if err := Action(ctx, composePath, "stack", "api", action); err != nil {
			t.Errorf("%s: %v", action, err)
		}
	}
	if err := Action(ctx, composePath, "stack", "api", "delete"); err == nil {
		t.Fatal("unsupported action accepted")
	}
	if err := OpenTerminal(ctx, composePath, "stack", "api"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	var calls []byte
	for time.Now().Before(deadline) {
		calls, _ = os.ReadFile(logPath)
		if strings.Contains(string(calls), "xdg-terminal-exec docker compose") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, expected := range []string{"config --no-interpolate", "--project-name stack ps --all", "stats --no-stream", "up --detach --no-build --no-deps api", "restart --no-deps api", "build api", "--force-recreate --no-deps api", "xdg-terminal-exec docker compose"} {
		if !strings.Contains(string(calls), expected) {
			t.Errorf("missing %q in calls:\n%s", expected, calls)
		}
	}
}

func TestDecodeComposePSArrayAndLines(t *testing.T) {
	array := []byte(`[{"ID":"abc","Name":"app-api-1","State":"running","Health":"healthy","ExitCode":0,"Publishers":[{"PublishedPort":3000}]}]`)
	records, err := decodePS(array)
	if err != nil || len(records) != 1 || records[0].Publishers[0].PublishedPort != 3000 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	lines := []byte("{\"ID\":\"a\",\"State\":\"running\"}\n{\"ID\":\"b\",\"State\":\"exited\"}\n")
	records, err = decodePS(lines)
	if err != nil || !reflect.DeepEqual([]string{records[0].ID, records[1].ID}, []string{"a", "b"}) {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

func TestDockerErrorDoesNotSurfaceUnclassifiedOutput(t *testing.T) {
	message := concise([]byte("password=super-secret"), errors.New("exit status 1"))
	if strings.Contains(message, "super-secret") || message != "exit status 1" {
		t.Fatalf("unsafe Docker error = %q", message)
	}
}

func TestDecodeComposePSRejectsMalformed(t *testing.T) {
	if _, err := decodePS([]byte(`{"ID":`)); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}

func TestParseDockerMemory(t *testing.T) {
	for input, want := range map[string]float64{"31.5MiB": 31.5, "1.5GiB": 1536, "1024KiB": 1} {
		got, err := parseMemoryMB(input)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", input, got, err, want)
		}
	}
}
