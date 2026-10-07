package health

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"omastack/internal/model"
	"omastack/internal/supervise"
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

func (c *Checker) Check(ctx context.Context, check model.HealthCheck, options ...supervise.Execution) error {
	// Queueing is part of the probe's budget, not an unbounded prelude to it.
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(check.TimeoutSeconds)*time.Second)
	defer cancel()
	select {
	case c.semaphore <- struct{}{}:
	case <-callCtx.Done():
		return callCtx.Err()
	}
	defer func() { <-c.semaphore }()
	switch check.Type {
	case "http", "https":
		return checkHTTP(callCtx, check.HTTP)
	case "tcp":
		return checkTCP(callCtx, check.TCP)
	case "command":
		return checkCommand(callCtx, check.Command, options...)
	default:
		return errors.New("unsupported health-check type")
	}
}

func checkHTTP(ctx context.Context, spec *model.HTTPCheck) error {
	if spec == nil {
		return errors.New("missing HTTP check")
	}
	client := newHTTPClient()
	defer client.CloseIdleConnections()
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

func checkCommand(ctx context.Context, spec *model.CommandSpec, options ...supervise.Execution) error {
	if spec == nil {
		return errors.New("missing command check")
	}
	policy := supervise.Execution{}
	if len(options) > 0 {
		policy = options[0]
	}
	cmd, err := policy.Command(ctx, spec.Executable, spec.Arguments...)
	if err != nil {
		return err
	}
	if err := cmd.Run(); err != nil {
		// Command output is unclassified and can contain secrets from project
		// tooling. Report only the bounded process result.
		return fmt.Errorf("health command failed: %w", err)
	}
	return nil
}

type Registry struct {
	mu       sync.Mutex
	states   map[string]State
	runs     map[string]string
	versions map[string]uint64
}

func NewRegistry() *Registry {
	return &Registry{states: map[string]State{}, runs: map[string]string{}, versions: map[string]uint64{}}
}
func (r *Registry) Get(id string) State        { r.mu.Lock(); defer r.mu.Unlock(); return r.states[id] }
func (r *Registry) Set(id string, state State) { r.mu.Lock(); r.states[id] = state; r.mu.Unlock() }

// Observe ties cached health and in-flight results to one process/configuration.
func (r *Registry) Observe(id, run string) (State, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs[id] != run {
		r.runs[id] = run
		r.versions[id]++
		delete(r.states, id)
	}
	return r.states[id], r.versions[id]
}

func (r *Registry) Reset(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, id)
	delete(r.states, id)
	r.versions[id]++
}

func (r *Registry) Commit(id string, version uint64, state State) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.versions[id] != version {
		return false
	}
	r.states[id] = state
	return true
}
