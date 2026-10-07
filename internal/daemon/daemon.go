package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"omastack/internal/control"
	"omastack/internal/docker"
	"omastack/internal/health"
	"omastack/internal/model"
	"omastack/internal/monitor"
	"omastack/internal/paths"
	"omastack/internal/proxy"
	"omastack/internal/redact"
	"omastack/internal/store"
	"omastack/internal/supervise"
	"omastack/internal/systemd"
)

type Daemon struct {
	paths         paths.Paths
	store         *store.ConfigStore
	systemd       systemd.Manager
	healthChecker *health.Checker
	healthStates  *health.Registry
	proxy         *proxy.Manager

	mu                     sync.RWMutex
	operations             sync.Mutex
	activeOpMu             sync.Mutex
	activeOpCancel         context.CancelFunc
	snapshot               model.Snapshot
	samplers               map[string]*monitor.Sampler
	lastPortPoll           map[string]time.Time
	lastDockerPoll         map[string]time.Time
	dockerRevision         map[string]uint64
	dockerObservedRevision map[string]uint64
	dockerErrors           map[string]string
	dockerStates           map[string]docker.ContainerState
	healthRunning          map[string]uint64
	autostartError         string
	previousStatus         map[string]model.ServiceStatus
	panelOpen              bool
	listener               net.Listener
	workers                chan struct{}
	logWorkers             chan struct{}
}

func New(resolved paths.Paths) (*Daemon, error) {
	if err := resolved.Ensure(); err != nil {
		return nil, err
	}
	configStore, err := store.OpenConfig(resolved.ConfigFile)
	if err != nil {
		return nil, err
	}
	d := &Daemon{
		paths:                  resolved,
		store:                  configStore,
		systemd:                systemd.Manager{Timeout: 30 * time.Second},
		healthChecker:          health.NewChecker(8),
		healthStates:           health.NewRegistry(),
		samplers:               map[string]*monitor.Sampler{},
		lastPortPoll:           map[string]time.Time{},
		lastDockerPoll:         map[string]time.Time{},
		dockerRevision:         map[string]uint64{},
		dockerObservedRevision: map[string]uint64{},
		dockerErrors:           map[string]string{},
		dockerStates:           map[string]docker.ContainerState{},
		healthRunning:          map[string]uint64{},
		previousStatus:         map[string]model.ServiceStatus{},
		workers:                make(chan struct{}, 32),
		logWorkers:             make(chan struct{}, 2),
	}
	d.proxy = proxy.New(d.resolveProxyPort)
	return d, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.configureProxy(); err != nil { /* reported through diagnostics */
	}
	listener, err := listen(d.paths.SocketFile)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.listener = listener
	d.mu.Unlock()
	defer func() {
		listener.Close()
		_ = os.Remove(d.paths.SocketFile)
		// Keep definitions visible, but never present a stopped daemon's last
		// observation as a live connection during the freshness grace period.
		offline := d.Snapshot()
		offline.Connected = false
		offline.BackendPID = 0
		if data, err := json.MarshalIndent(offline, "", "  "); err == nil {
			_ = store.AtomicWrite(d.paths.SnapshotFile, append(data, '\n'), 0o600)
		}
		_ = d.proxy.Close()
	}()

	acceptErrors := make(chan error, 1)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				acceptErrors <- err
				return
			}
			select {
			case d.workers <- struct{}{}:
				go func() { defer func() { <-d.workers }(); d.handleConnection(ctx, connection) }()
			default:
				_ = control.Encode(connection, control.Failure("busy", "busy", "too many concurrent requests"))
				connection.Close()
			}
		}
	}()

	d.reconcile(ctx)
	poll := time.NewTicker(d.pollInterval())
	defer poll.Stop()
	// The listener and reconciliation loop must remain available while a
	// dependency-aware autostart waits for health (and for Stop to cancel it).
	go d.startAutostart(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-acceptErrors:
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		case <-poll.C:
			d.reconcile(ctx)
			interval := d.pollInterval()
			poll.Reset(interval)
		}
	}
}

func listen(path string) (net.Listener, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing unsafe control socket path %s", path)
		}
		connection, dialErr := net.DialTimeout("unix", path, 250*time.Millisecond)
		if dialErr == nil {
			connection.Close()
			return nil, errors.New("another OmaStack daemon is already listening")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func (d *Daemon) handleConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err := control.SameUser(connection); err != nil {
		_ = control.Encode(connection, control.Failure("unknown", "unauthorized", err.Error()))
		return
	}
	request, err := control.Decode(connection)
	if err != nil {
		_ = control.Encode(connection, control.Failure("unknown", "invalid-request", err.Error()))
		return
	}
	_ = connection.SetDeadline(time.Now().Add(methodTimeout(request.Method) + 10*time.Second))
	response := d.handle(ctx, request)
	_ = control.Encode(connection, response)
}

func (d *Daemon) pollInterval() time.Duration {
	seconds := d.store.Get().Settings.PollIntervalSeconds
	if seconds < 1 || seconds > 60 {
		seconds = 2
	}
	d.mu.RLock()
	panelOpen := d.panelOpen
	d.mu.RUnlock()
	if !panelOpen {
		seconds *= 3
		if seconds > 60 {
			seconds = 60
		}
	}
	return time.Duration(seconds) * time.Second
}

func (d *Daemon) Snapshot() model.Snapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	copy := d.snapshot
	copy.Projects = append([]model.Project{}, d.snapshot.Projects...)
	copy.Runtime = make(map[string]model.ServiceRuntime, len(d.snapshot.Runtime))
	for key, value := range d.snapshot.Runtime {
		copy.Runtime[key] = value
	}
	return copy
}

func (d *Daemon) reconcile(ctx context.Context) {
	config := d.store.Get()
	secrets := redact.Secrets(config)
	previous := d.Snapshot()
	runtimes := make(map[string]model.ServiceRuntime)
	diagnostics := []model.Diagnostic{}
	now := time.Now().UTC()
	unitMissing := false
	observations := d.observeServices(ctx, config)

	for _, project := range config.Projects {
		for _, service := range project.Services {
			runtime := model.ServiceRuntime{ServiceID: service.ID, ProjectID: project.ID, Status: model.StatusStopped}
			observed, observedOK := observations[service.ID]
			unit, unitErr := observed.unit, observed.unitErr
			unitKnown := unitErr == nil
			if !observedOK || unitErr != nil {
				unitMissing = true
				if prior, ok := previous.Runtime[service.ID]; ok {
					runtime = prior
				}
				runtime.Stale = true
				runtime.LastError = "Status refresh unavailable; showing last known state"
				runtimes[service.ID] = runtime
				continue
			} else {
				runtime.Status = statusFor(unit.ActiveState, unit.SubState)
			}
			record, recordErr := supervise.ReadRuntime(d.paths.RuntimeServicesDir, service.ID)
			if recordErr == nil {
				runtime.PID = record.PID
				runtime.ExitCode = record.ExitCode
				runtime.Signal = record.Signal
				runtime.RestartCount = record.RestartCount
				runtime.LastError = redact.Text(record.LastError, secrets)
				runtime.LastStarted = record.StartedAt
				runtime.LastStopped = record.StoppedAt
				if record.StartedAt != nil && record.PID > 0 {
					runtime.UptimeSeconds = int64(now.Sub(*record.StartedAt).Seconds())
				}
				if !unitKnown && runtime.Status == model.StatusStopped && record.ExitCode != nil && *record.ExitCode != 0 {
					runtime.Status = model.StatusCrashed
				}
			}
			if unitKnown {
				applyUnitOutcome(&runtime, unit)
			}
			cleanUnitStop := unitKnown && runtime.Status == model.StatusStopped && runtime.ExitCode != nil && *runtime.ExitCode == 0
			if prior, ok := previous.Runtime[service.ID]; ok {
				runtime.History = append([]model.MetricsPoint{}, prior.History...)
				runtime.Ports = append([]int{}, prior.Ports...)
			}
			if service.Docker != nil {
				if observed.dockerPolled {
					d.lastDockerPoll[service.ID] = time.Now()
					if observed.dockerErr != nil {
						d.dockerErrors[service.ID] = redact.Text(observed.dockerErr.Error(), secrets)
					} else {
						delete(d.dockerErrors, service.ID)
						d.dockerObservedRevision[service.ID] = observed.dockerRevision
						d.dockerStates[service.ID] = observed.container
					}
				}
				if message := d.dockerErrors[service.ID]; message != "" {
					runtime.LastError, runtime.Stale = message, true
				}
				state := d.dockerStates[service.ID]
				runtime.ContainerID, runtime.ContainerName = state.ID, state.Name
				runtime.ContainerState, runtime.ContainerHealth = state.State, state.Health
				runtime.CPU, runtime.MemoryMB = state.CPU, state.MemoryMB
				if len(state.Published) > 0 {
					runtime.Ports = append([]int{}, state.Published...)
				}
				// A cached pre-stop observation cannot override the current unit
				// outcome. Fresh and revision-valid cached observations still
				// expose orphaned containers between Docker polls.
				if !cleanUnitStop || (observed.dockerRevision == d.dockerObservedRevision[service.ID] && d.dockerErrors[service.ID] == "") {
					applyDockerOutcome(&runtime, state, cleanUnitStop)
				}
				if state.State == "running" {
					runtime.History = append(runtime.History, model.MetricsPoint{At: now, CPU: runtime.CPU, MemoryMB: runtime.MemoryMB})
					capacity := config.Settings.HistorySamples
					if len(runtime.History) > capacity {
						runtime.History = append([]model.MetricsPoint{}, runtime.History[len(runtime.History)-capacity:]...)
					}
				}
			}
			if service.Docker == nil && record.PID > 0 && (runtime.Status == model.StatusRunning || runtime.Status == model.StatusStarting) {
				sampler := d.samplers[service.ID]
				if sampler == nil {
					sampler = monitor.NewSampler()
					d.samplers[service.ID] = sampler
				}
				d.mu.RLock()
				panelOpen := d.panelOpen
				d.mu.RUnlock()
				portInterval := 30 * time.Second
				if panelOpen {
					portInterval = 5 * time.Second
				}
				includePorts := time.Since(d.lastPortPoll[service.ID]) >= portInterval
				aggregate, sampleErr := sampler.Sample(record.PID, includePorts)
				if sampleErr == nil {
					runtime.CPU, runtime.MemoryMB = aggregate.CPU, aggregate.MemoryMB
					if includePorts {
						runtime.Ports = aggregate.Ports
						d.lastPortPoll[service.ID] = time.Now()
					}
					runtime.History = append(runtime.History, model.MetricsPoint{At: now, CPU: runtime.CPU, MemoryMB: runtime.MemoryMB})
					capacity := config.Settings.HistorySamples
					if len(runtime.History) > capacity {
						runtime.History = append([]model.MetricsPoint{}, runtime.History[len(runtime.History)-capacity:]...)
					}
				}
			}
			if service.Health != nil && (runtime.Status == model.StatusRunning || runtime.Status == model.StatusStarting) {
				key := runIdentity(service, unit, record)
				if service.Docker != nil {
					key = dockerRunIdentity(key, d.dockerStates[service.ID])
				}
				hstate, _ := d.healthStates.Observe(service.ID, key)
				runtime.Health = hstate.Status
				if hstate.Status == "unhealthy" {
					runtime.Status = model.StatusUnhealthy
					runtime.LastError = redact.Text(hstate.LastError, secrets)
				}
				d.scheduleHealth(ctx, project.Name, service, record.StartedAt)
			} else {
				d.healthStates.Observe(service.ID, "")
			}
			d.notifyTransition(config.Settings.Notifications, project.Name, service.Name, d.previousStatus[service.ID], runtime.Status)
			d.previousStatus[service.ID] = runtime.Status
			runtimes[service.ID] = runtime
		}
	}
	if unitMissing {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "warning", Code: "systemd-units", Message: "Some statuses could not refresh: check installed user units, systemd responsiveness and diagnostics"})
	}
	if err := d.configureProxy(); err != nil {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "error", Code: "proxy", Message: err.Error()})
	}
	if value := d.proxy.LastError(); value != "" {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "error", Code: "proxy-runtime", Message: value})
	}
	d.mu.RLock()
	autostartError := d.autostartError
	d.mu.RUnlock()
	if autostartError != "" {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "error", Code: "autostart", Message: autostartError})
	}

	snapshot := model.Snapshot{
		Version: 1, GeneratedAt: now, Connected: true, BackendPID: os.Getpid(),
		Settings: config.Settings, Projects: redact.Config(config).Projects, Runtime: runtimes, Routes: d.proxy.Routes(), Diagnostics: diagnostics,
	}
	d.mu.Lock()
	d.snapshot = snapshot
	d.mu.Unlock()
	if data, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
		_ = store.AtomicWrite(d.paths.SnapshotFile, append(data, '\n'), 0o600)
	}
}

func applyDockerOutcome(runtime *model.ServiceRuntime, state docker.ContainerState, cleanUnitStop bool) {
	if runtime.Status == model.StatusCrashed && (state.State == "running" || state.State == "restarting" || state.State == "created") {
		// A surviving Docker-owned container does not repair failed supervision.
		return
	}
	switch state.State {
	case "running":
		if cleanUnitStop {
			runtime.Status = model.StatusUnhealthy
			runtime.LastError = "Container is running outside OmaStack supervision; stop it before editing or deleting"
		} else {
			runtime.Status = model.StatusRunning
		}
	case "restarting", "created":
		if cleanUnitStop && state.State == "created" {
			runtime.Status = model.StatusStopped
		} else if cleanUnitStop {
			runtime.Status = model.StatusUnhealthy
			runtime.LastError = "Container is restarting outside OmaStack supervision"
		} else {
			runtime.Status = model.StatusStarting
		}
	case "exited", "dead":
		if cleanUnitStop {
			// docker compose commonly reports 137 after the supervisor forwards
			// an intentional stop. The successfully stopped systemd unit is the
			// authoritative lifecycle outcome in that case.
			zero := 0
			runtime.ExitCode = &zero
			runtime.Signal = ""
			runtime.LastError = ""
			runtime.Status = model.StatusStopped
		} else if state.ExitCode != 0 {
			code := state.ExitCode
			runtime.ExitCode = &code
			runtime.Status = model.StatusCrashed
		} else if runtime.Status != model.StatusCrashed {
			runtime.Status = model.StatusStopped
		}
	}
	if state.Health == "unhealthy" && state.State == "running" {
		runtime.Status = model.StatusUnhealthy
	}
}

func statusFor(active, sub string) model.ServiceStatus {
	switch active {
	case "activating":
		return model.StatusStarting
	case "deactivating":
		return model.StatusStopping
	case "failed":
		return model.StatusCrashed
	case "active":
		if sub == "running" || sub == "exited" {
			return model.StatusRunning
		}
		return model.StatusStarting
	default:
		return model.StatusStopped
	}
}

func applyUnitOutcome(runtime *model.ServiceRuntime, unit systemd.UnitState) {
	if runtime.Status == model.StatusRunning || runtime.Status == model.StatusStarting || runtime.Status == model.StatusStopping {
		return
	}

	// systemd is authoritative once the unit is no longer active. A stale
	// runtime file can otherwise retain the child PID after SIGKILL or make an
	// intentional SIGTERM shutdown look like a crash.
	runtime.PID = 0
	runtime.UptimeSeconds = 0
	if runtime.Status == model.StatusStopped && unit.Result == "success" {
		zero := 0
		runtime.ExitCode = &zero
		runtime.Signal = ""
		runtime.LastError = ""
		return
	}
	if runtime.Status != model.StatusCrashed {
		return
	}

	switch unit.ExecMainCode {
	case 1: // CLD_EXITED
		code := unit.ExecMainStatus
		runtime.ExitCode = &code
		runtime.Signal = ""
	case 2, 3: // CLD_KILLED, CLD_DUMPED
		runtime.ExitCode = nil
		runtime.Signal = systemSignalName(unit.ExecMainStatus)
	}
	if runtime.Signal == "" && unit.Result == "signal" && unit.ExecMainStatus > 0 {
		runtime.ExitCode = nil
		runtime.Signal = systemSignalName(unit.ExecMainStatus)
	}
	if runtime.LastError == "" {
		detail := unit.Result
		if detail == "" {
			detail = "failed"
		}
		if runtime.Signal != "" {
			detail += " (" + runtime.Signal + ")"
		} else if runtime.ExitCode != nil {
			detail += fmt.Sprintf(" (exit %d)", *runtime.ExitCode)
		}
		runtime.LastError = "systemd result: " + detail
	}
}

func systemSignalName(value int) string {
	names := map[int]string{
		1: "SIGHUP", 2: "SIGINT", 3: "SIGQUIT", 4: "SIGILL", 6: "SIGABRT",
		7: "SIGBUS", 8: "SIGFPE", 9: "SIGKILL", 11: "SIGSEGV", 13: "SIGPIPE",
		14: "SIGALRM", 15: "SIGTERM",
	}
	if name := names[value]; name != "" {
		return name
	}
	return fmt.Sprintf("signal %d", value)
}

func (d *Daemon) scheduleHealth(ctx context.Context, projectName string, service model.Service, started *time.Time) {
	if service.Health == nil {
		return
	}
	d.mu.Lock()
	// One outstanding job per service, including identity checks and semaphore
	// waiting. A new generation cannot accumulate obsolete queued goroutines.
	if _, ok := d.healthRunning[service.ID]; ok {
		d.mu.Unlock()
		return
	}
	d.healthRunning[service.ID] = 1
	d.mu.Unlock()
	go func() {
		defer func() { d.mu.Lock(); delete(d.healthRunning, service.ID); d.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		current, key, currentStarted, err := d.currentRun(ctx, service.ID)
		if err != nil || key == "" || current.Health == nil {
			return
		}
		check := current.Health
		state, version := d.healthStates.Observe(service.ID, key)
		if currentStarted != nil && time.Since(*currentStarted) < time.Duration(check.StartGraceSeconds)*time.Second {
			return
		}
		if !state.LastChecked.IsZero() && time.Since(state.LastChecked) < time.Duration(check.IntervalSeconds)*time.Second {
			return
		}
		policy, policyErr := supervise.ForService(current)
		if policyErr != nil {
			err = policyErr
		} else {
			err = d.healthChecker.Check(ctx, *check, policy)
		}
		_, after, _, identityErr := d.currentRun(ctx, service.ID)
		if ctx.Err() == nil && identityErr == nil && after == key {
			next := health.Transition(state, err == nil, check.Retries, errorText(err))
			d.healthStates.Commit(service.ID, version, next)
		}
		// Notifications are emitted once by the settings-aware reconciliation
		// transition path, never directly by an asynchronous probe.
	}()
}

func (d *Daemon) notifyTransition(settings model.NotificationSettings, project, service string, previous, next model.ServiceStatus) {
	if previous == "" || previous == next {
		return
	}
	label := project + " / " + service
	if next == model.StatusCrashed && settings.Crashed {
		d.notify("critical", "OmaStack service crashed", label+" stopped unexpectedly")
	}
	if next == model.StatusUnhealthy && settings.Unhealthy {
		d.notify("normal", "OmaStack service unhealthy", label+" failed its health check")
	}
	if (previous == model.StatusCrashed || previous == model.StatusUnhealthy) && next == model.StatusRunning && settings.Recovered {
		d.notify("normal", "OmaStack service recovered", label+" is running")
	}
}

func (d *Daemon) notify(urgency, title, body string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "notify-send", "--app-name=OmaStack", "--urgency="+urgency, title, body).Run()
}

func (d *Daemon) configureProxy() error {
	config := d.store.Get()
	return d.proxy.Configure(config.Settings.Proxy, config.Projects)
}

func (d *Daemon) resolveProxyPort(serviceID string, configured int) (int, bool) {
	snapshot := d.Snapshot()
	runtime, ok := snapshot.Runtime[serviceID]
	if !ok || runtime.Stale || (runtime.Status != model.StatusRunning && runtime.Status != model.StatusUnhealthy) {
		return 0, false
	}
	if configured > 0 {
		return configured, true
	}
	if len(runtime.Ports) == 0 {
		return 0, false
	}
	return runtime.Ports[0], true
}

func (d *Daemon) startAutostart(ctx context.Context) {
	// RuntimeDir is scoped to the user-manager session. Updating/restarting
	// only the daemon must not revive apps the user deliberately stopped.
	marker := filepath.Join(d.paths.RuntimeDir, "autostart-attempted")
	if _, err := os.Lstat(marker); err == nil {
		return
	} else if !os.IsNotExist(err) {
		d.setAutostartError(err)
		return
	}
	if err := store.AtomicWrite(marker, []byte("1\n"), 0o600); err != nil {
		d.setAutostartError(err)
		return
	}
	operationCtx, finish, err := d.beginSerializedOperation(ctx, "start")
	if err != nil {
		d.setAutostartError(err)
		return
	}
	defer finish()
	config := d.store.Get()
	var targets []string
	for _, project := range config.Projects {
		for _, service := range project.Services {
			if service.Autostart {
				targets = append(targets, service.ID)
			}
		}
	}
	if len(targets) > 0 {
		d.setAutostartError(d.startServices(operationCtx, config.AllServices(), targets))
	}
}

func (d *Daemon) setAutostartError(err error) {
	d.mu.Lock()
	d.autostartError = redact.Text(errorText(err), redact.Secrets(d.store.Get()))
	d.mu.Unlock()
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(strings.Fields(err.Error()), " ")
	if len(value) > 512 {
		value = value[:509] + "…"
	}
	return value
}
