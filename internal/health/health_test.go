package health

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"omastack/internal/model"
)

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

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
