package health

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omastack/internal/model"
	"omastack/internal/supervise"
)

func TestProbeTimeoutIncludesSemaphoreQueue(t *testing.T) {
	checker := NewChecker(1)
	checker.semaphore <- struct{}{}
	defer func() { <-checker.semaphore }()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := checker.Check(ctx, model.HealthCheck{Type: "command", TimeoutSeconds: 1, Command: &model.CommandSpec{Executable: "/usr/bin/true"}})
	if err == nil || ctx.Err() != nil || time.Since(started) > 2*time.Second {
		t.Fatalf("queue outlived check budget: %v (%v)", err, time.Since(started))
	}
}

func TestHealthTransitions(t *testing.T) {
	state := Transition(State{}, false, 2, "down")
	if state.Status != "checking" || state.Failures != 1 {
		t.Fatalf("first failure = %#v", state)
	}
	state = Transition(state, false, 2, "down")
	if state.Status != "unhealthy" {
		t.Fatalf("second failure = %#v", state)
	}
	state = Transition(state, true, 2, "")
	if state.Status != "healthy" || state.Failures != 0 || state.LastError != "" {
		t.Fatalf("recovery = %#v", state)
	}
}

func TestRegistryRejectsPreviousRunAndInFlightResults(t *testing.T) {
	r := NewRegistry()
	_, old := r.Observe("api", "run-one")
	if !r.Commit("api", old, State{Status: "healthy"}) {
		t.Fatal("current result rejected")
	}
	state, current := r.Observe("api", "run-two")
	if state.Status != "" {
		t.Fatal("old health survived new run")
	}
	for _, status := range []string{"healthy", "unhealthy"} {
		if r.Commit("api", old, State{Status: status}) {
			t.Fatal("stale in-flight result accepted")
		}
	}
	r.Reset("api")
	if r.Commit("api", current, State{Status: "healthy"}) {
		t.Fatal("stopped service accepted result")
	}
}

func TestHTTPHealthCheckStatusAndBody(t *testing.T) {
	previous := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("ready=yes")), Header: make(http.Header), Request: request}, nil
		})}
	}
	defer func() { newHTTPClient = previous }()
	check := model.HealthCheck{Type: "http", TimeoutSeconds: 2, HTTP: &model.HTTPCheck{URL: "http://127.0.0.1/health", ExpectedStatus: 204, ResponseContains: "ready=yes"}}
	if err := NewChecker(2).Check(context.Background(), check); err != nil {
		t.Fatal(err)
	}
	check.HTTP.ResponseContains = "missing"
	if err := NewChecker(2).Check(context.Background(), check); err == nil {
		t.Fatal("mismatched body passed")
	}
}

func TestCommandHealthDoesNotSurfaceOutput(t *testing.T) {
	check := model.CommandSpec{Executable: "/bin/sh", Arguments: []string{"-c", "printf super-secret >&2; exit 1"}}
	err := checkCommand(context.Background(), &check)
	if err == nil || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("unsafe command-health error: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func TestCommandProbeUsesServiceEnvironmentDirectoryAndPATH(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fake-ambient-credential")
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte("#!/bin/sh\ntest -z \"$AWS_SECRET_ACCESS_KEY\" && test \"$EXPLICIT\" = configured && test -f marker\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	service := model.Service{WorkingDirectory: dir, Environment: map[string]model.EnvValue{"PATH": {Value: dir}, "EXPLICIT": {Value: "configured"}}}
	policy, err := supervise.ForService(service)
	if err != nil {
		t.Fatal(err)
	}
	check := model.HealthCheck{Type: "command", TimeoutSeconds: 2, Command: &model.CommandSpec{Executable: "probe"}}
	if err := NewChecker(1).Check(context.Background(), check, policy); err != nil {
		t.Fatal(err)
	}
	defaultProbe := model.CommandSpec{Executable: "/bin/sh", Arguments: []string{"-c", "test -z \"$AWS_SECRET_ACCESS_KEY\""}}
	if err := checkCommand(context.Background(), &defaultProbe); err != nil {
		t.Fatal(err)
	}
}

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
