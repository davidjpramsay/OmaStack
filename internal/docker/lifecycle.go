package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"omastack/internal/model"
	"omastack/internal/supervise"
)

func composeArguments(spec *model.DockerSpec) []string {
	args := []string{"compose", "-f", spec.ComposeFile}
	if spec.ProjectName != "" {
		args = append(args, "--project-name", spec.ProjectName)
	}
	return args
}

// StopService also reaches Docker-daemon-owned containers when no supervisor is
// alive. A zero-grace stop marks the container manually stopped, unlike merely
// killing a Compose client (or relying on a container restart policy).
func StopService(ctx context.Context, service model.Service, force bool) error {
	if service.Docker == nil {
		return nil
	}
	policy, err := supervise.ForService(service)
	if err != nil {
		return err
	}
	seconds := service.GracefulStopSeconds
	if force {
		seconds = 0
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(seconds+10)*time.Second)
	defer cancel()
	args := append(composeArguments(service.Docker), "stop", "--timeout", fmt.Sprint(seconds), service.Docker.Service)
	output, err := runWithPolicy(callCtx, policy, 2<<20, "docker", args...)
	if err != nil {
		return fmt.Errorf("stop Docker service: %s", concise(output, err))
	}
	return EnsureStopped(ctx, service)
}

// EnsureStopped checks every replica and actual container state. An inspection
// error or unfamiliar state never authorizes deletion/import/uninstall.
func EnsureStopped(ctx context.Context, service model.Service) error {
	if service.Docker == nil {
		return nil
	}
	policy, err := supervise.ForService(service)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	args := append(composeArguments(service.Docker), "ps", "--all", "--format", "json", service.Docker.Service)
	output, err := runWithPolicy(callCtx, policy, 8<<20, "docker", args...)
	if err != nil {
		return fmt.Errorf("cannot verify Docker service is stopped: %s", concise(output, err))
	}
	records, err := decodePS(output)
	if err != nil {
		return err
	}
	for _, record := range records {
		state := record.State
		if record.ID != "" {
			output, err := runWithPolicy(callCtx, policy, 64<<10, "docker", "inspect", "--format", "{{json .State}}", record.ID)
			if err != nil {
				return fmt.Errorf("cannot verify Docker container is stopped: %s", concise(output, err))
			}
			var actual struct {
				Status string `json:"Status"`
			}
			if err := json.Unmarshal(output, &actual); err != nil {
				return fmt.Errorf("parse Docker container state: %w", err)
			}
			state = actual.Status
		}
		switch state {
		case "created", "exited", "dead", "stopped":
		default:
			return fmt.Errorf("docker container is not confirmed stopped (state %q); stop it before changing its definition", state)
		}
	}
	return nil
}
