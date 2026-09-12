package daemon

import (
	"context"
	"sync"
	"time"

	"omastack/internal/docker"
	"omastack/internal/model"
	"omastack/internal/systemd"
)

type observation struct {
	unit         systemd.UnitState
	unitErr      error
	container    docker.ContainerState
	dockerErr    error
	dockerPolled bool
}

// One slow systemd/Docker command must not serialize the whole 128-service
// snapshot behind it. Both concurrency and the complete polling cycle are
// bounded; missing observations are explicitly stale, never invented stops.
func (d *Daemon) observeServices(ctx context.Context, config model.Config) map[string]observation {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d.mu.RLock()
	panelOpen := d.panelOpen
	d.mu.RUnlock()
	interval := 30 * time.Second
	if panelOpen {
		interval = 5 * time.Second
	}
	type job struct {
		service    model.Service
		pollDocker bool
	}
	jobs := make(chan job, 128)
	for _, project := range config.Projects {
		for _, service := range project.Services {
			jobs <- job{service: service, pollDocker: service.Docker != nil && time.Since(d.lastDockerPoll[service.ID]) >= interval}
		}
	}
	close(jobs)
	results := map[string]observation{}
	var mu sync.Mutex
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			manager := d.systemd
			manager.Timeout = 2 * time.Second
			for job := range jobs {
				if ctx.Err() != nil {
					return
				}
				unit, err := manager.Show(ctx, job.service.ID)
				result := observation{unit: unit, unitErr: err}
				if job.pollDocker && ctx.Err() == nil {
					spec := job.service.Docker
					result.container, result.dockerErr = docker.Inspect(ctx, spec.ComposeFile, spec.ProjectName, spec.Service)
					result.dockerPolled = true
				}
				mu.Lock()
				results[job.service.ID] = result
				mu.Unlock()
			}
		}()
	}
	workers.Wait()
	return results
}
