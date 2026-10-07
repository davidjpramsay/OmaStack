package daemon

import (
	"context"
	"sync"
	"time"

	"omastack/internal/docker"
	"omastack/internal/model"
	"omastack/internal/supervise"
	"omastack/internal/systemd"
)

type observation struct {
	unit           systemd.UnitState
	unitErr        error
	container      docker.ContainerState
	dockerErr      error
	dockerPolled   bool
	dockerRevision uint64
}

// Invalidate polling without racing with the reconciliation-owned caches.
// An action finishing during a poll leaves a newer revision for the next poll.
func (d *Daemon) dockerChanged(id string) {
	d.mu.Lock()
	d.dockerRevision[id]++
	d.mu.Unlock()
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
		revision   uint64
	}
	jobs := make(chan job, 128)
	for _, project := range config.Projects {
		for _, service := range project.Services {
			d.mu.RLock()
			revision := d.dockerRevision[service.ID]
			d.mu.RUnlock()
			jobs <- job{service: service, revision: revision, pollDocker: service.Docker != nil && (revision != d.dockerObservedRevision[service.ID] || time.Since(d.lastDockerPoll[service.ID]) >= interval)}
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
				result := observation{unit: unit, unitErr: err, dockerRevision: job.revision}
				if job.pollDocker && ctx.Err() == nil {
					spec := job.service.Docker
					policy, policyErr := supervise.ForService(job.service)
					result.dockerErr = policyErr
					if policyErr == nil {
						result.container, result.dockerErr = docker.Inspect(ctx, spec.ComposeFile, spec.ProjectName, spec.Service, policy)
					}
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
