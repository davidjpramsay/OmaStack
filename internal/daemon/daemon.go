package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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

	mu             sync.RWMutex
	operations     sync.Mutex
	activeOpMu     sync.Mutex
	activeOpCancel context.CancelFunc
	snapshot       model.Snapshot
	samplers       map[string]*monitor.Sampler
	lastPortPoll   map[string]time.Time
	lastDockerPoll map[string]time.Time
	dockerStates   map[string]docker.ContainerState
	healthRunning  map[string]bool
	previousStatus map[string]model.ServiceStatus
	panelOpen      bool
	listener       net.Listener
	workers        chan struct{}
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
		paths:          resolved,
		store:          configStore,
		systemd:        systemd.Manager{Timeout: 30 * time.Second},
		healthChecker:  health.NewChecker(8),
		healthStates:   health.NewRegistry(),
		samplers:       map[string]*monitor.Sampler{},
		lastPortPoll:   map[string]time.Time{},
		lastDockerPoll: map[string]time.Time{},
		dockerStates:   map[string]docker.ContainerState{},
		healthRunning:  map[string]bool{},
		previousStatus: map[string]model.ServiceStatus{},
		workers:        make(chan struct{}, 32),
	}
	d.proxy = proxy.New(d.resolveProxyPort)
	return d, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.configureProxy(); err != nil { /* reported through diagnostics */
	}
	d.reconcile(ctx)
	d.startAutostart(ctx)
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
		_ = os.Remove(d.paths.SnapshotFile)
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

	poll := time.NewTicker(d.pollInterval())
	defer poll.Stop()
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

	for _, project := range config.Projects {
		for _, service := range project.Services {
			runtime := model.ServiceRuntime{ServiceID: service.ID, ProjectID: project.ID, Status: model.StatusStopped}
			unit, unitErr := d.systemd.Show(ctx, service.ID)
			unitKnown := unitErr == nil
			if unitErr != nil {
				unitMissing = true
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
				d.mu.RLock()
				panelOpen := d.panelOpen
				d.mu.RUnlock()
				dockerInterval := 30 * time.Second
				if panelOpen {
					dockerInterval = 5 * time.Second
				}
				if time.Since(d.lastDockerPoll[service.ID]) >= dockerInterval {
					state, inspectErr := docker.Inspect(ctx, service.Docker.ComposeFile, service.Docker.ProjectName, service.Docker.Service)
					d.lastDockerPoll[service.ID] = time.Now()
					if inspectErr != nil {
						runtime.LastError = redact.Text(inspectErr.Error(), secrets)
					} else {
						d.dockerStates[service.ID] = state
					}
				}
				state := d.dockerStates[service.ID]
				runtime.ContainerID, runtime.ContainerName = state.ID, state.Name
				runtime.ContainerState, runtime.ContainerHealth = state.State, state.Health
				runtime.CPU, runtime.MemoryMB = state.CPU, state.MemoryMB
				if len(state.Published) > 0 {
					runtime.Ports = append([]int{}, state.Published...)
				}
				applyDockerOutcome(&runtime, state, cleanUnitStop)
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
				hstate := d.healthStates.Get(service.ID)
				runtime.Health = hstate.Status
				if hstate.Status == "unhealthy" {
					runtime.Status = model.StatusUnhealthy
					runtime.LastError = redact.Text(hstate.LastError, secrets)
				}
				d.scheduleHealth(ctx, project.Name, service, record.StartedAt)
			}
			d.notifyTransition(config.Settings.Notifications, project.Name, service.Name, d.previousStatus[service.ID], runtime.Status)
			d.previousStatus[service.ID] = runtime.Status
			runtimes[service.ID] = runtime
		}
	}
	if unitMissing {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "warning", Code: "systemd-units", Message: "OmaStack user units are not installed or not yet visible to systemd"})
	}
	if err := d.configureProxy(); err != nil {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "error", Code: "proxy", Message: err.Error()})
	}
	if value := d.proxy.LastError(); value != "" {
		diagnostics = append(diagnostics, model.Diagnostic{Level: "error", Code: "proxy-runtime", Message: value})
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
	switch state.State {
	case "running":
		runtime.Status = model.StatusRunning
	case "restarting", "created":
		runtime.Status = model.StatusStarting
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
	check := service.Health
	if check == nil {
		return
	}
	state := d.healthStates.Get(service.ID)
	if started != nil && time.Since(*started) < time.Duration(check.StartGraceSeconds)*time.Second {
		return
	}
	if !state.LastChecked.IsZero() && time.Since(state.LastChecked) < time.Duration(check.IntervalSeconds)*time.Second {
		return
	}
	d.mu.Lock()
	if d.healthRunning[service.ID] {
		d.mu.Unlock()
		return
	}
	d.healthRunning[service.ID] = true
	d.mu.Unlock()
	go func() {
		err := d.healthChecker.Check(ctx, *check)
		previous := d.healthStates.Get(service.ID)
		next := health.Transition(previous, err == nil, check.Retries, errorText(err))
		d.healthStates.Set(service.ID, next)
		d.mu.Lock()
		d.healthRunning[service.ID] = false
		d.mu.Unlock()
		if previous.Status == "unhealthy" && next.Status == "healthy" {
			d.notify("normal", "OmaStack service recovered", projectName+" / "+service.Name+" is healthy")
		}
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
	if !ok || (runtime.Status != model.StatusRunning && runtime.Status != model.StatusUnhealthy) {
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
	config := d.store.Get()
	for _, project := range config.Projects {
		for _, service := range project.Services {
			if service.Autostart {
				_ = d.systemd.Start(ctx, service.ID)
			}
		}
	}
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
