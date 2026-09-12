package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"omastack/internal/control"
	"omastack/internal/deps"
	"omastack/internal/docker"
	"omastack/internal/logs"
	"omastack/internal/model"
	"omastack/internal/redact"
	"omastack/internal/securefile"
	"omastack/internal/store"
	"omastack/internal/validate"
)

type projectPayload struct {
	Project model.Project `json:"project"`
}
type projectIDPayload struct {
	ProjectID string `json:"projectId"`
}
type projectOrderPayload struct {
	ProjectIDs []string `json:"projectIds"`
}
type servicePayload struct {
	ProjectID string        `json:"projectId"`
	Service   model.Service `json:"service"`
}
type serviceIDPayload struct {
	ServiceID string `json:"serviceId"`
}
type dockerImportPayload struct {
	ComposeFile string `json:"composeFile"`
}
type dockerActionPayload struct {
	ServiceID string `json:"serviceId"`
	Action    string `json:"action"`
}
type visibilityPayload struct {
	Open bool `json:"open"`
}
type settingsPayload struct {
	Settings model.Settings `json:"settings"`
}
type configImportPayload struct {
	Path    string `json:"path"`
	Replace bool   `json:"replace"`
}

func (d *Daemon) handle(ctx context.Context, request control.Request) control.Response {
	ctx, cancel := context.WithTimeout(ctx, methodTimeout(request.Method))
	defer cancel()
	if serializedOperation(request.Method) {
		operationCtx, finish, err := d.beginSerializedOperation(ctx, request.Method)
		if err != nil {
			return control.Failure(request.ID, "operation-canceled", err.Error())
		}
		defer finish()
		ctx = operationCtx
	}
	var result any
	var err error
	switch request.Method {
	case "ping":
		result = map[string]any{"status": "ok", "pid": os.Getpid()}
	case "status", "list":
		result = d.Snapshot()
	case "start", "stop", "restart", "kill":
		var params control.TargetParams
		err = decodeParams(request.Params, &params)
		if err == nil {
			err = d.performAction(ctx, request.Method, params.Target)
		}
		result = map[string]string{"status": "ok"}
	case "logs":
		var params control.LogsParams
		err = decodeParams(request.Params, &params)
		if err == nil {
			result, err = d.readLogs(ctx, params)
		}
	case "config.get":
		result = redact.Config(d.store.Get())
	case "config.export":
		result, err = d.exportConfig()
	case "config.import":
		result, err = d.importConfig(ctx, request.Params)
	case "settings.update":
		var params settingsPayload
		err = decodeParams(request.Params, &params)
		if err == nil {
			err = d.store.Update(func(config *model.Config) error { config.Settings = params.Settings; return nil })
		}
		result = map[string]string{"status": "saved"}
	case "settings.patch":
		result, err = d.patchSettings(request.Params)
	case "ui.visibility":
		var params visibilityPayload
		err = decodeParams(request.Params, &params)
		if err == nil {
			d.mu.Lock()
			d.panelOpen = params.Open
			d.mu.Unlock()
		}
		result = map[string]bool{"open": params.Open}
	case "project.create":
		result, err = d.createProject(request.Params)
	case "project.update":
		result, err = d.updateProject(request.Params)
	case "project.duplicate":
		result, err = d.duplicateProject(request.Params)
	case "project.delete":
		result, err = d.deleteProject(ctx, request.Params)
	case "project.reorder":
		result, err = d.reorderProjects(request.Params)
	case "service.create":
		result, err = d.createService(request.Params)
	case "service.update":
		result, err = d.updateService(request.Params)
	case "service.delete":
		result, err = d.deleteService(ctx, request.Params)
	case "docker.status":
		result = docker.Detect(ctx)
	case "docker.import":
		result, err = d.importCompose(ctx, request.Params)
	case "docker.action":
		result, err = d.dockerAction(ctx, request.Params)
	case "docker.terminal":
		result, err = d.dockerTerminal(ctx, request.Params)
	case "proxy.routes":
		result = d.proxy.Routes()
	case "doctor":
		result = d.doctor(ctx)
	case "cleanup":
		result, err = d.cleanup()
	default:
		err = fmt.Errorf("unknown method %q", request.Method)
	}
	if err != nil {
		return control.Failure(request.ID, errorCode(err), err.Error())
	}
	return control.Success(request.ID, result)
}

func (d *Daemon) beginSerializedOperation(ctx context.Context, method string) (context.Context, func(), error) {
	operationCtx, cancel := context.WithTimeout(ctx, methodTimeout(method))
	for !d.operations.TryLock() {
		if interruptsOperation(method) {
			d.cancelActiveOperation()
		}
		select {
		case <-operationCtx.Done():
			cancel()
			return nil, nil, operationCtx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	d.activeOpMu.Lock()
	d.activeOpCancel = cancel
	d.activeOpMu.Unlock()
	finish := func() {
		d.activeOpMu.Lock()
		d.activeOpCancel = nil
		d.activeOpMu.Unlock()
		cancel()
		d.operations.Unlock()
	}
	return operationCtx, finish, nil
}

func methodTimeout(method string) time.Duration {
	if method == "stop" || method == "restart" {
		return 12 * time.Hour
	}
	if method == "start" || method == "restart" || method == "docker.action" {
		return 6 * time.Minute
	}
	return time.Minute
}

func (d *Daemon) cancelActiveOperation() {
	d.activeOpMu.Lock()
	cancel := d.activeOpCancel
	d.activeOpMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func interruptsOperation(method string) bool {
	return method == "stop" || method == "kill" || method == "restart"
}

func serializedOperation(method string) bool {
	switch method {
	case "start", "stop", "restart", "kill",
		"config.import", "settings.update", "settings.patch",
		"project.create", "project.update", "project.duplicate", "project.delete", "project.reorder",
		"service.create", "service.update", "service.delete",
		"docker.import", "docker.action", "docker.terminal", "cleanup":
		return true
	default:
		return false
	}
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	if len(raw) > control.MaxMessageBytes/2 {
		return errors.New("params too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (d *Daemon) performAction(ctx context.Context, action, target string) error {
	config := d.store.Get()
	ids, err := resolveTargets(config, target)
	if err != nil {
		return err
	}
	services := config.AllServices()
	switch action {
	case "start":
		return d.startServices(ctx, services, ids)
	case "stop":
		order, err := deps.ShutdownOrder(services, ids)
		if err != nil {
			return err
		}
		var failures []error
		for _, id := range order {
			if ctx.Err() != nil {
				return errors.Join(append(failures, ctx.Err())...)
			}
			d.healthStates.Reset(id)
			manager := d.systemd
			manager.Timeout = time.Duration(services[id].GracefulStopSeconds+10) * time.Second
			if err := manager.Stop(ctx, id); err != nil {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	case "restart":
		if err := d.performAction(ctx, "stop", target); err != nil {
			return err
		}
		return d.performAction(ctx, "start", target)
	case "kill":
		for _, id := range ids {
			d.healthStates.Reset(id)
			if err := d.systemd.ForceKill(ctx, id); err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported lifecycle action")
	}
	return nil
}

func resolveTargets(config model.Config, target string) ([]string, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == "all" {
		result := []string{}
		projects := append([]model.Project{}, config.Projects...)
		sort.SliceStable(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })
		for _, project := range projects {
			for _, service := range project.Services {
				result = append(result, service.ID)
			}
		}
		if len(result) == 0 {
			return nil, errors.New("no services configured")
		}
		return result, nil
	}
	for _, project := range config.Projects {
		if project.ID == target || project.Name == target {
			result := make([]string, 0, len(project.Services))
			for _, service := range project.Services {
				result = append(result, service.ID)
			}
			if len(result) == 0 {
				return nil, errors.New("project has no services")
			}
			return result, nil
		}
	}
	var matches []string
	for _, project := range config.Projects {
		for _, service := range project.Services {
			if service.ID == target || project.Name+"/"+service.Name == target || service.Name == target {
				matches = append(matches, service.ID)
			}
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("project or service %q not found", target)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("service name %q is ambiguous; use project/service or the stable id", target)
	}
	return matches, nil
}

func (d *Daemon) readLogs(ctx context.Context, params control.LogsParams) (any, error) {
	if d.logWorkers != nil {
		select {
		case d.logWorkers <- struct{}{}:
			defer func() { <-d.logWorkers }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	config := d.store.Get()
	ids, err := resolveTargets(config, params.Target)
	if err != nil {
		return nil, err
	}
	lines := params.Lines
	if lines < 1 {
		lines = 200
	}
	if lines > 50000 {
		lines = 50000
	}
	names := map[string]string{}
	for _, project := range config.Projects {
		for _, service := range project.Services {
			names[service.ID] = project.Name + " / " + service.Name
		}
	}
	entries := make([]logs.Entry, 0, lines)
	truncated := false
	for _, id := range ids {
		serviceEntries, limited, readErr := logs.ReadDetailed(ctx, id, lines, params.Query)
		if readErr != nil {
			return nil, readErr
		}
		for index := range serviceEntries {
			serviceEntries[index].Service = names[id]
		}
		truncated = truncated || limited
		entries = append(entries, serviceEntries...)
		// Bound the merge after every source, not only after all 128 sources.
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp.Before(entries[j].Timestamp) })
		if len(entries) > lines {
			entries = entries[len(entries)-lines:]
		}
		bounded := boundLogEntries(entries, control.MaxMessageBytes/2)
		truncated = truncated || len(bounded) < len(entries)
		entries = bounded
	}
	sort.SliceStable(entries, func(left, right int) bool { return entries[left].Timestamp.Before(entries[right].Timestamp) })
	if len(entries) > lines {
		entries = entries[len(entries)-lines:]
	}
	secrets := redact.Secrets(config)
	for i := range entries {
		entries[i].Message = redact.Text(entries[i].Message, secrets)
	}
	bounded := boundLogEntries(entries, control.MaxMessageBytes/2)
	truncated = truncated || len(bounded) < len(entries)
	if params.Metadata {
		result := logs.Result{Entries: bounded, Truncated: truncated, Limit: lines}
		if truncated {
			result.Notice = "Showing newest available entries; journal capture, message, or 512 KiB response limit reached."
		}
		return result, nil
	}
	return bounded, nil
}

func boundLogEntries(entries []logs.Entry, budget int) []logs.Entry {
	if budget < 1024 {
		return nil
	}
	remaining := budget - 2 // JSON array delimiters.
	reversed := make([]logs.Entry, 0, len(entries))
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		encoded, err := json.Marshal(entry)
		if err != nil || len(encoded)+1 > remaining {
			break
		}
		remaining -= len(encoded) + 1
		reversed = append(reversed, entry)
	}
	result := make([]logs.Entry, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result
}

func (d *Daemon) createProject(raw json.RawMessage) (any, error) {
	var params projectPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	project := params.Project
	if project.ID == "" {
		project.ID, _ = validate.NewID()
	}
	config := d.store.Get()
	project.Order = len(config.Projects)
	for index := range project.Services {
		normalizeService(&project.Services[index])
	}
	if err := d.store.Update(func(config *model.Config) error { config.Projects = append(config.Projects, project); return nil }); err != nil {
		return nil, err
	}
	return map[string]string{"status": "created", "id": project.ID}, nil
}

func (d *Daemon) updateProject(raw json.RawMessage) (any, error) {
	var params projectPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if !validate.ID(params.Project.ID) {
		return nil, errors.New("invalid project id")
	}
	err := d.store.Update(func(config *model.Config) error {
		for index := range config.Projects {
			if config.Projects[index].ID == params.Project.ID {
				if len(params.Project.Services) == 0 {
					params.Project.Services = config.Projects[index].Services
				}
				mergeMaskedProjectSecrets(&params.Project, config.Projects[index])
				config.Projects[index] = params.Project
				return nil
			}
		}
		return errors.New("project not found")
	})
	return map[string]string{"status": "saved", "id": params.Project.ID}, err
}

func (d *Daemon) duplicateProject(raw json.RawMessage) (any, error) {
	var params projectIDPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var duplicate model.Project
	clearedRoutes := false
	err := d.store.Update(func(config *model.Config) error {
		for _, project := range config.Projects {
			if project.ID != params.ProjectID {
				continue
			}
			data, _ := json.Marshal(project)
			_ = json.Unmarshal(data, &duplicate)
			duplicate.ID, _ = validate.NewID()
			name := []rune(duplicate.Name)
			if len(name) > 75 {
				name = name[:75]
			}
			duplicate.Name = string(name) + " copy"
			duplicate.Order = len(config.Projects)
			remap := map[string]string{}
			for index := range duplicate.Services {
				// A cloned definition must not steal the original's unique route.
				// Let the user explicitly assign hostnames for the copied stack.
				if duplicate.Services[index].Route != nil {
					clearedRoutes = true
					duplicate.Services[index].Route = nil
				}
				old := duplicate.Services[index].ID
				duplicate.Services[index].ID, _ = validate.NewID()
				remap[old] = duplicate.Services[index].ID
			}
			for index := range duplicate.Services {
				for depIndex := range duplicate.Services[index].Dependencies {
					if replacement := remap[duplicate.Services[index].Dependencies[depIndex].ServiceID]; replacement != "" {
						duplicate.Services[index].Dependencies[depIndex].ServiceID = replacement
					}
				}
			}
			config.Projects = append(config.Projects, duplicate)
			return nil
		}
		return errors.New("project not found")
	})
	result := map[string]string{"status": "created", "id": duplicate.ID}
	if clearedRoutes {
		result["notice"] = "Project duplicated without local routes; assign unique hostnames to the copy."
	}
	return result, err
}

func (d *Daemon) deleteProject(ctx context.Context, raw json.RawMessage) (any, error) {
	var params projectIDPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	config := d.store.Get()
	project, ok := config.FindProject(params.ProjectID)
	if !ok {
		return nil, errors.New("project not found")
	}
	for _, service := range project.Services {
		if err := d.ensureStopped(ctx, service.ID); err != nil {
			return nil, err
		}
	}
	err := d.store.Update(func(config *model.Config) error {
		index := -1
		removedServices := map[string]bool{}
		for i, project := range config.Projects {
			if project.ID == params.ProjectID {
				index = i
				for _, service := range project.Services {
					removedServices[service.ID] = true
				}
				break
			}
		}
		if index < 0 {
			return errors.New("project not found")
		}
		if serviceName := externalProjectDependency(*config, params.ProjectID, removedServices); serviceName != "" {
			return fmt.Errorf("cannot delete project; service %q depends on it", serviceName)
		}
		config.Projects = append(config.Projects[:index], config.Projects[index+1:]...)
		for i := range config.Projects {
			config.Projects[i].Order = i
		}
		return nil
	})
	return map[string]string{"status": "deleted"}, err
}

func externalProjectDependency(config model.Config, removedProjectID string, removedServices map[string]bool) string {
	for _, project := range config.Projects {
		if project.ID == removedProjectID {
			continue
		}
		for _, service := range project.Services {
			for _, dependency := range service.Dependencies {
				if removedServices[dependency.ServiceID] {
					return service.Name
				}
			}
		}
	}
	return ""
}

func (d *Daemon) reorderProjects(raw json.RawMessage) (any, error) {
	var params projectOrderPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	err := d.store.Update(func(config *model.Config) error {
		if len(params.ProjectIDs) != len(config.Projects) {
			return errors.New("project order must contain every project exactly once")
		}
		byID := map[string]model.Project{}
		for _, project := range config.Projects {
			byID[project.ID] = project
		}
		ordered := make([]model.Project, 0, len(config.Projects))
		seen := map[string]bool{}
		for index, id := range params.ProjectIDs {
			project, ok := byID[id]
			if !ok || seen[id] {
				return errors.New("invalid project order")
			}
			seen[id] = true
			project.Order = index
			ordered = append(ordered, project)
		}
		config.Projects = ordered
		return nil
	})
	return map[string]string{"status": "saved"}, err
}

func (d *Daemon) createService(raw json.RawMessage) (any, error) {
	var params servicePayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	normalizeService(&params.Service)
	err := d.store.Update(func(config *model.Config) error {
		for index := range config.Projects {
			if config.Projects[index].ID == params.ProjectID {
				config.Projects[index].Services = append(config.Projects[index].Services, params.Service)
				return nil
			}
		}
		return errors.New("project not found")
	})
	return map[string]string{"status": "created", "id": params.Service.ID}, err
}

func (d *Daemon) updateService(raw json.RawMessage) (any, error) {
	var params servicePayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if !validate.ID(params.Service.ID) {
		return nil, errors.New("invalid service id")
	}
	err := d.store.Update(func(config *model.Config) error {
		for pi := range config.Projects {
			for si := range config.Projects[pi].Services {
				if config.Projects[pi].Services[si].ID == params.Service.ID {
					mergeMaskedSecrets(&params.Service, config.Projects[pi].Services[si])
					config.Projects[pi].Services[si] = params.Service
					return nil
				}
			}
		}
		return errors.New("service not found")
	})
	return map[string]string{"status": "saved", "id": params.Service.ID}, err
}

func (d *Daemon) deleteService(ctx context.Context, raw json.RawMessage) (any, error) {
	var params serviceIDPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if err := d.ensureStopped(ctx, params.ServiceID); err != nil {
		return nil, err
	}
	err := d.store.Update(func(config *model.Config) error {
		for _, project := range config.Projects {
			for _, service := range project.Services {
				for _, dep := range service.Dependencies {
					if dep.ServiceID == params.ServiceID {
						return fmt.Errorf("cannot delete service; %q depends on it", service.Name)
					}
				}
			}
		}
		for pi := range config.Projects {
			for si := range config.Projects[pi].Services {
				if config.Projects[pi].Services[si].ID == params.ServiceID {
					config.Projects[pi].Services = append(config.Projects[pi].Services[:si], config.Projects[pi].Services[si+1:]...)
					return nil
				}
			}
		}
		return errors.New("service not found")
	})
	return map[string]string{"status": "deleted"}, err
}

func (d *Daemon) ensureStopped(ctx context.Context, serviceID string) error {
	if !validate.ID(serviceID) {
		return errors.New("invalid service id")
	}
	unit, err := d.systemd.Show(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("cannot verify service is stopped: %w", err)
	}
	if unit.MainPID != 0 || unit.ActiveState == "active" || unit.ActiveState == "activating" || unit.ActiveState == "deactivating" || unit.ActiveState == "reloading" {
		return errors.New("service must be stopped before its definition can be deleted")
	}
	if unit.ActiveState != "inactive" && unit.ActiveState != "failed" {
		return fmt.Errorf("cannot verify service is stopped: unexpected systemd state %q", unit.ActiveState)
	}
	return nil
}

func (d *Daemon) exportConfig() (any, error) {
	name := "omastack-backup-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
	path := filepath.Join(d.paths.ExportDir, name)
	data, err := json.MarshalIndent(d.store.Get(), "", "  ")
	if err != nil {
		return nil, err
	}
	if err := store.AtomicWrite(path, append(data, '\n'), 0o600); err != nil {
		return nil, err
	}
	return map[string]string{"status": "exported", "path": path}, nil
}

func (d *Daemon) importConfig(ctx context.Context, raw json.RawMessage) (any, error) {
	var params configImportPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if !params.Replace {
		return nil, errors.New("config import requires explicit replacement confirmation")
	}
	if !filepath.IsAbs(params.Path) || filepath.Clean(params.Path) != params.Path {
		return nil, errors.New("import path must be absolute and normalized")
	}
	for id := range d.store.Get().AllServices() {
		if err := d.ensureStopped(ctx, id); err != nil {
			return nil, err
		}
	}
	data, err := securefile.ReadRegular(params.Path, 8<<20, true)
	if err != nil {
		return nil, err
	}
	config, err := store.DecodeConfig(data)
	if err != nil {
		return nil, err
	}
	if err := d.store.Update(func(current *model.Config) error { *current = config; return nil }); err != nil {
		return nil, err
	}
	return map[string]string{"status": "imported"}, nil
}

func normalizeService(service *model.Service) {
	if service.ID == "" {
		service.ID, _ = validate.NewID()
	}
	if service.StopSignal == "" {
		service.StopSignal = "SIGTERM"
	}
	if service.GracefulStopSeconds == 0 {
		service.GracefulStopSeconds = 10
	}
	if service.Restart.Mode == "" {
		service.Restart.Mode = "never"
	}
	if service.Restart.DelaySeconds == 0 {
		service.Restart.DelaySeconds = 1
	}
	if service.Environment == nil {
		service.Environment = map[string]model.EnvValue{}
	}
}

func mergeMaskedProjectSecrets(next *model.Project, previous model.Project) {
	old := map[string]model.Service{}
	for _, service := range previous.Services {
		old[service.ID] = service
	}
	for index := range next.Services {
		if prior, ok := old[next.Services[index].ID]; ok {
			mergeMaskedSecrets(&next.Services[index], prior)
		}
	}
}

func mergeMaskedSecrets(next *model.Service, previous model.Service) {
	for name, value := range next.Environment {
		if value.Secret && value.Value == redact.Mask {
			if old, ok := previous.Environment[name]; ok && old.Secret {
				next.Environment[name] = old
			}
		}
	}
}

func (d *Daemon) importCompose(ctx context.Context, raw json.RawMessage) (any, error) {
	var params dockerImportPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	return docker.ImportCompose(ctx, params.ComposeFile)
}

func (d *Daemon) dockerAction(ctx context.Context, raw json.RawMessage) (any, error) {
	var params dockerActionPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	_, service, ok := d.store.Get().FindService(params.ServiceID)
	if !ok || service.Docker == nil {
		return nil, errors.New("docker service not found")
	}
	d.healthStates.Reset(service.ID)
	err := docker.Action(ctx, service.Docker.ComposeFile, service.Docker.ProjectName, service.Docker.Service, params.Action)
	return map[string]string{"status": "ok"}, err
}

func (d *Daemon) dockerTerminal(ctx context.Context, raw json.RawMessage) (any, error) {
	var params serviceIDPayload
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	_, service, ok := d.store.Get().FindService(params.ServiceID)
	if !ok || service.Docker == nil {
		return nil, errors.New("docker service not found")
	}
	if err := docker.OpenTerminal(ctx, service.Docker.ComposeFile, service.Docker.ProjectName, service.Docker.Service); err != nil {
		return nil, err
	}
	return map[string]string{"status": "opened"}, nil
}

func (d *Daemon) doctor(ctx context.Context) any {
	availability := docker.Detect(ctx)
	return map[string]any{
		"version":    1,
		"paths":      map[string]string{"config": d.paths.ConfigFile, "runtime": d.paths.RuntimeDir, "state": d.paths.StateDir, "cache": d.paths.CacheDir, "socket": d.paths.SocketFile},
		"backendPid": os.Getpid(), "docker": availability, "diagnostics": d.Snapshot().Diagnostics,
	}
}

func (d *Daemon) cleanup() (any, error) {
	config := d.store.Get()
	valid := map[string]bool{}
	for id := range config.AllServices() {
		valid[id] = true
	}
	removed := []string{}
	entries, err := os.ReadDir(d.paths.RuntimeServicesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validate.ID(id) || valid[id] {
			continue
		}
		path := filepath.Join(d.paths.RuntimeServicesDir, entry.Name())
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			if os.Remove(path) == nil {
				removed = append(removed, path)
			}
		}
	}
	return map[string]any{"removed": removed, "projectDefinitionsPreserved": true}, nil
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "not found"):
		return "not-found"
	case strings.Contains(text, "ambiguous"):
		return "ambiguous-target"
	case strings.Contains(text, "invalid"), strings.Contains(text, "must"), strings.Contains(text, "unknown field"):
		return "invalid-argument"
	case strings.Contains(text, "dependency cycle"):
		return "dependency-cycle"
	default:
		return "operation-failed"
	}
}
