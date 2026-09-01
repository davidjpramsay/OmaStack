package health

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"omastack/internal/model"
)

type State struct {
	Status      string
	Failures    int
	LastError   string
	LastChecked time.Time
}

func Transition(previous State, success bool, retries int, message string) State {
	next := previous
	next.LastChecked = time.Now().UTC()
	if success {
		next.Status, next.Failures, next.LastError = "healthy", 0, ""
		return next
	}
	next.Failures++
	next.LastError = message
	if retries < 1 {
		retries = 1
	}
	if next.Failures >= retries {
		next.Status = "unhealthy"
	} else {
		next.Status = "checking"
	}
	return next
}

type Checker struct{ semaphore chan struct{} }

var newHTTPClient = func() *http.Client {
	return &http.Client{Transport: &http.Transport{}}
}

func NewChecker(maxConcurrent int) *Checker {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if maxConcurrent > 32 {
		maxConcurrent = 32
	}
	return &Checker{semaphore: make(chan struct{}, maxConcurrent)}
}

func (c *Checker) Check(ctx context.Context, check model.HealthCheck) error {
	select {
	case c.semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.semaphore }()
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch check.Type {
	case "http", "https":
		return checkHTTP(callCtx, check.HTTP)
	case "tcp":
		return checkTCP(callCtx, check.TCP)
	case "command":
		return checkCommand(callCtx, check.Command)
	default:
		return errors.New("unsupported health-check type")
	}
}

func checkHTTP(ctx context.Context, spec *model.HTTPCheck) error {
	if spec == nil {
		return errors.New("missing HTTP check")
	}
	client := newHTTPClient()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	expected := spec.ExpectedStatus
	if expected == 0 {
		expected = 200
	}
	if response.StatusCode != expected {
		return fmt.Errorf("expected HTTP %d, got %d", expected, response.StatusCode)
	}
	if spec.ResponseContains == "" && spec.ResponseRegex == "" {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if spec.ResponseContains != "" && !bytes.Contains(body, []byte(spec.ResponseContains)) {
		return errors.New("response text did not match")
	}
	if spec.ResponseRegex != "" {
		pattern, err := regexp.Compile(spec.ResponseRegex)
		if err != nil {
			return err
		}
		if !pattern.Match(body) {
			return errors.New("response regular expression did not match")
		}
	}
	return nil
}

func checkTCP(ctx context.Context, spec *model.TCPCheck) error {
	if spec == nil {
		return errors.New("missing TCP check")
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(spec.Host, fmt.Sprint(spec.Port)))
	if err == nil {
		_ = connection.Close()
	}
	return err
}

func checkCommand(ctx context.Context, spec *model.CommandSpec) error {
	if spec == nil {
		return errors.New("missing command check")
	}
	cmd := exec.CommandContext(ctx, spec.Executable, spec.Arguments...)
	if err := cmd.Run(); err != nil {
		// Command output is unclassified and can contain secrets from project
		// tooling. Report only the bounded process result.
		return fmt.Errorf("health command failed: %w", err)
	}
	return nil
}

type Registry struct {
	mu     sync.Mutex
	states map[string]State
}

func NewRegistry() *Registry                   { return &Registry{states: map[string]State{}} }
func (r *Registry) Get(id string) State        { r.mu.Lock(); defer r.mu.Unlock(); return r.states[id] }
func (r *Registry) Set(id string, state State) { r.mu.Lock(); r.states[id] = state; r.mu.Unlock() }
