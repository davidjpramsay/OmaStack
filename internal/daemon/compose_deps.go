package daemon

import (
	"context"
	"fmt"
	"sort"
	"time"

	"omastack/internal/docker"
	"omastack/internal/model"
	"omastack/internal/supervise"
)

// Compose dependencies get separate managed supervisors. Never let one
// attached `up` command acquire ownership of a shared container. Inspect only
// the requested closure, so a broken unrelated Compose file cannot block Start.
func managedComposeGraph(ctx context.Context, services map[string]model.Service, targets []string) (map[string]model.Service, error) {
	graph := make(map[string]model.Service, len(services))
	for id, service := range services {
		service.Dependencies = append([]model.Dependency{}, service.Dependencies...)
		graph[id] = service
	}
	models := map[string]map[string]docker.ImportedService{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true
		service, ok := graph[id]
		if !ok {
			return fmt.Errorf("unknown service %q", id)
		}
		if spec := service.Docker; spec != nil {
			entries, ok := models[spec.ComposeFile]
			if !ok {
				policy, err := supervise.ForService(service)
				if err != nil {
					return err
				}
				imported, err := docker.ImportCompose(ctx, spec.ComposeFile, policy)
				if err != nil {
					return err
				}
				entries = map[string]docker.ImportedService{}
				for _, item := range imported {
					entries[item.Name] = item
				}
				models[spec.ComposeFile] = entries
			}
			entry, ok := entries[spec.Service]
			if !ok {
				return fmt.Errorf("compose service %q not found", spec.Service)
			}
			for _, dependency := range entry.Dependencies {
				match := ""
				for candidateID, candidate := range graph {
					other := candidate.Docker
					if other != nil && other.ComposeFile == spec.ComposeFile && other.ProjectName == spec.ProjectName && other.Service == dependency.Service {
						if match != "" {
							return fmt.Errorf("compose dependency %q is managed more than once", dependency.Service)
						}
						match = candidateID
					}
				}
				if match == "" {
					if !dependency.Required {
						continue
					}
					return fmt.Errorf("add Compose dependency %q to OmaStack with the same Compose file and project name before starting %q", dependency.Service, spec.Service)
				}
				condition := map[string]string{"service_started": "docker_started", "service_healthy": "healthy", "service_completed_successfully": "docker_completed"}[dependency.Condition]
				if condition == "" {
					return fmt.Errorf("unsupported Compose dependency condition %q", dependency.Condition)
				}
				service.Dependencies = append(service.Dependencies, model.Dependency{ServiceID: match, Condition: condition})
			}
		}
		// Stable ordering also makes Start All reproducible for shared deps.
		sort.SliceStable(service.Dependencies, func(i, j int) bool { return service.Dependencies[i].ServiceID < service.Dependencies[j].ServiceID })
		graph[id] = service
		for _, dependency := range service.Dependencies {
			if err := visit(dependency.ServiceID); err != nil {
				return err
			}
		}
		return nil
	}
	for _, id := range targets {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func waitDockerCondition(ctx context.Context, service model.Service, condition string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if service.Docker == nil {
		return fmt.Errorf("docker dependency has no Compose configuration")
	}
	spec := service.Docker
	policy, err := supervise.ForService(service)
	if err != nil {
		return err
	}
	for {
		state, err := docker.InspectHealth(ctx, spec.ComposeFile, spec.ProjectName, spec.Service, policy)
		if err != nil {
			return err
		}
		if state.ID != "" && !state.StartedAt.IsZero() {
			if condition == "docker_started" && state.State == "running" {
				return nil
			}
			if state.State == "exited" || state.State == "dead" {
				if condition == "docker_completed" && state.State == "exited" && state.ExitCode == 0 {
					return nil
				}
				return fmt.Errorf("docker dependency exited before readiness (exit %d)", state.ExitCode)
			}
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
