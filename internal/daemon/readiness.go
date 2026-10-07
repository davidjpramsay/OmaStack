package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"omastack/internal/deps"
	"omastack/internal/docker"
	"omastack/internal/health"
	"omastack/internal/model"
	"omastack/internal/supervise"
	"omastack/internal/systemd"
)

func runIdentity(service model.Service, unit systemd.UnitState, record supervise.RuntimeRecord) string {
	if unit.ActiveState != "active" || unit.MainPID <= 0 || record.SupervisorPID != unit.MainPID || record.PID <= 0 || record.StartedAt == nil || record.StoppedAt != nil {
		return ""
	}
	definition, _ := json.Marshal(service)
	return fmt.Sprintf("%s/%d/%d/%d/%s", unit.InvocationID, unit.MainPID, record.PID, record.StartedAt.UnixNano(), definition)
}

func (d *Daemon) currentRun(ctx context.Context, id string) (model.Service, string, *time.Time, error) {
	service, key, started, _, err := d.currentRunState(ctx, id)
	return service, key, started, err
}

func dockerRunIdentity(key string, container docker.ContainerState) string {
	if key == "" || container.ID == "" || container.StartedAt.IsZero() || container.State != "running" {
		return ""
	}
	return fmt.Sprintf("%s/container/%s/%d", key, container.ID, container.StartedAt.UnixNano())
}

func (d *Daemon) currentRunState(ctx context.Context, id string) (model.Service, string, *time.Time, docker.ContainerState, error) {
	var container docker.ContainerState
	_, service, ok := d.store.Get().FindService(id)
	if !ok {
		return model.Service{}, "", nil, container, errors.New("service no longer exists")
	}
	unit, err := d.systemd.Show(ctx, id)
	if err != nil {
		return *service, "", nil, container, err
	}
	if unit.ActiveState == "failed" {
		return *service, "", nil, container, errors.New("service failed before becoming healthy")
	}
	record, err := supervise.ReadRuntime(d.paths.RuntimeServicesDir, id)
	if err != nil {
		return *service, "", nil, container, nil
	}
	key, started := runIdentity(*service, unit, record), record.StartedAt
	if key != "" && service.Docker != nil {
		spec := service.Docker
		policy, policyErr := supervise.ForService(*service)
		if policyErr != nil {
			return *service, "", nil, container, policyErr
		}
		container, err = docker.InspectHealth(ctx, spec.ComposeFile, spec.ProjectName, spec.Service, policy)
		if err != nil {
			return *service, "", nil, container, err
		}
		key = dockerRunIdentity(key, container)
		started = &container.StartedAt
	}
	return *service, key, started, container, nil
}

func (d *Daemon) startServices(ctx context.Context, services map[string]model.Service, ids []string) error {
	services, err := managedComposeGraph(ctx, services, ids)
	if err != nil {
		return err
	}
	order, err := deps.StartupOrder(services, ids)
	if err != nil {
		return err
	}
	for _, id := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, dependency := range services[id].Dependencies {
			if dependency.Condition == "healthy" {
				if err := d.waitHealthy(ctx, dependency.ServiceID, 5*time.Minute); err != nil {
					return fmt.Errorf("dependency %s: %w", dependency.ServiceID, err)
				}
			} else if dependency.Condition == "docker_started" || dependency.Condition == "docker_completed" {
				if err := waitDockerCondition(ctx, services[dependency.ServiceID], dependency.Condition); err != nil {
					return fmt.Errorf("dependency %s: %w", dependency.ServiceID, err)
				}
			}
		}
		unit, err := d.systemd.Show(ctx, id)
		if err != nil {
			return err
		}
		if unit.ActiveState == "active" || unit.ActiveState == "activating" {
			continue
		}
		d.healthStates.Reset(id)
		_ = d.systemd.ResetFailed(ctx, id)
		if err := d.systemd.Start(ctx, id); err != nil {
			return err
		}
		if services[id].Docker != nil {
			d.dockerChanged(id)
		}
	}
	return nil
}

// Startup readiness is self-sufficient and always probes the current run. It
// does not trust the asynchronous display cache, even when it says healthy.
func (d *Daemon) waitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var state health.State
	lastRun := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		service, key, started, container, err := d.currentRunState(ctx, id)
		if err != nil {
			return err
		}
		if key != lastRun {
			state = health.State{}
			lastRun = key
		}
		delay := 500 * time.Millisecond
		if key != "" && service.Health == nil && service.Docker != nil {
			_, after, _, err := d.currentRun(ctx, id)
			if err != nil {
				return err
			}
			if key == after && container.State == "running" {
				switch container.Health {
				case "healthy":
					return nil
				case "unhealthy":
					return errors.New("docker dependency is unhealthy")
				case "":
					return errors.New("docker dependency has no native health check; configure an OmaStack health check")
				}
			}
		} else if key != "" && service.Health != nil {
			check := *service.Health
			if time.Since(*started) >= time.Duration(check.StartGraceSeconds)*time.Second {
				_, generation := d.healthStates.Observe(id, key)
				policy, policyErr := supervise.ForService(service)
				if policyErr != nil {
					return policyErr
				}
				checkErr := d.healthChecker.Check(ctx, check, policy)
				_, after, _, err := d.currentRun(ctx, id)
				if err != nil {
					return err
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if key == after {
					state = health.Transition(state, checkErr == nil, check.Retries, errorText(checkErr))
					if d.healthStates.Commit(id, generation, state) {
						if state.Status == "healthy" {
							return nil
						}
						if state.Status == "unhealthy" {
							return errors.New(state.LastError)
						}
					}
					delay = time.Duration(check.IntervalSeconds) * time.Second
				}
			}
		} else if service.Health == nil && service.Docker == nil {
			return errors.New("dependency has no health check")
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
