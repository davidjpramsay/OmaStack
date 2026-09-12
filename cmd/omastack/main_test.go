package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omastack/internal/control"
	"omastack/internal/paths"
)

func setTestXDG(t *testing.T) paths.Paths {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(base, "runtime"))
	resolved, err := paths.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestRequestParamsPrivateStdin(t *testing.T) {
	secret := `{"service":{"environment":{"TOKEN":{"value":"sentinel-秘密\nvalue","secret":true}}}}`
	for _, input := range []string{secret, secret + "\n"} {
		got, err := requestParams([]string{"service.update", "-"}, strings.NewReader(input))
		if err != nil || string(got) != secret {
			t.Fatalf("stdin round trip: %q %v", got, err)
		}
	}
	for _, input := range []string{"", "{", secret + " junk\n", `"` + strings.Repeat("x", control.MaxMessageBytes/2) + `"`} {
		if _, err := requestParams([]string{"service.update", "-"}, strings.NewReader(input)); err == nil {
			t.Fatal("invalid/oversized stdin accepted")
		}
	}
	for _, args := range [][]string{{"ping"}, {"ping", "{}"}} {
		got, err := requestParams(args, strings.NewReader(""))
		if err != nil || string(got) != "{}" {
			t.Fatalf("legacy request: %q %v", got, err)
		}
	}
}

func TestRunInformationalAndArgumentValidation(t *testing.T) {
	setTestXDG(t)
	for _, args := range [][]string{{}, {"version"}, {"help"}, {"--help"}} {
		if err := run(args); err != nil {
			t.Errorf("run(%q): %v", args, err)
		}
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"unknown"}, "unknown command"},
		{[]string{"supervise"}, "usage"},
		{[]string{"start"}, "usage"},
		{[]string{"kill"}, "usage"},
		{[]string{"import", "backup.json"}, "usage"},
		{[]string{"docker", "unknown", "service"}, "docker action"},
		{[]string{"logs"}, "usage"},
		{[]string{"logs", "service", "--json", "--follow"}, "cannot be combined"},
		{[]string{"request"}, "usage"},
		{[]string{"request", "ping", "{"}, "params-json"},
	} {
		if err := run(test.args); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("run(%q) error = %v, want %q", test.args, err, test.want)
		}
	}
}

func TestClientTimeoutsAndDurations(t *testing.T) {
	resolved := setTestXDG(t)
	if got := client(resolved); got.Timeout != 30*time.Second || got.Socket != resolved.SocketFile {
		t.Fatalf("client = %#v", got)
	}
	for method, want := range map[string]time.Duration{
		"status": 30 * time.Second, "start": 6 * time.Minute, "restart": 12*time.Hour + 10*time.Second,
		"docker.action": 6 * time.Minute, "stop": 12*time.Hour + 10*time.Second, "kill": 2 * time.Minute, "config.import": 2 * time.Minute,
	} {
		if got := clientForMethod(resolved, method).Timeout; got != want {
			t.Errorf("%s timeout = %v, want %v", method, got, want)
		}
	}
	for seconds, want := range map[int64]string{0: "-", 9: "9s", 61: "1m1s", 3661: "1h1m", 90000: "1d1h"} {
		if got := duration(seconds); got != want {
			t.Errorf("duration(%d) = %q, want %q", seconds, got, want)
		}
	}
	if err := requestCommand(context.Background(), resolved, []string{"ping", "{"}); err == nil {
		t.Fatal("invalid request JSON accepted")
	}
}

func TestOpenPanelUsesSupportedSummonRoute(t *testing.T) {
	directory := t.TempDir()
	arguments := filepath.Join(directory, "arguments")
	script := `#!/bin/sh
printf '%s\n' "$*" > "$OMASTACK_OPEN_TEST_ARGS"
`
	path := filepath.Join(directory, "omarchy-shell")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMASTACK_OPEN_TEST_ARGS", arguments)
	if err := openPanel(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "shell summon david.omastack {}" {
		t.Fatalf("summon arguments = %q", data)
	}
}
