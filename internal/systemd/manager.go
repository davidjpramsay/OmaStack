package systemd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"omastack/internal/bounded"
	"omastack/internal/validate"
)

type Manager struct{ Timeout time.Duration }

type UnitState struct {
	ActiveState    string
	SubState       string
	MainPID        int
	Result         string
	ExecMainCode   int
	ExecMainStatus int
	NRestarts      int
	InvocationID   string
}

func UnitName(serviceID string) (string, error) {
	if !validate.ID(serviceID) {
		return "", errors.New("invalid service id")
	}
	return "omastack-service@" + serviceID + ".service", nil
}

func (m Manager) Start(ctx context.Context, serviceID string) error {
	return m.action(ctx, "start", serviceID)
}
func (m Manager) Stop(ctx context.Context, serviceID string) error {
	return m.action(ctx, "stop", serviceID)
}
func (m Manager) Restart(ctx context.Context, serviceID string) error {
	return m.action(ctx, "restart", serviceID)
}

func (m Manager) ForceKill(ctx context.Context, serviceID string) error {
	unit, err := UnitName(serviceID)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, "kill", "--kill-whom=all", "--signal=SIGKILL", unit)
	return err
}

func (m Manager) ResetFailed(ctx context.Context, serviceID string) error {
	return m.action(ctx, "reset-failed", serviceID)
}

func (m Manager) Show(ctx context.Context, serviceID string) (UnitState, error) {
	unit, err := UnitName(serviceID)
	if err != nil {
		return UnitState{}, err
	}
	output, err := m.run(ctx, "show", "--no-pager", "--property=ActiveState,SubState,MainPID,Result,ExecMainCode,ExecMainStatus,NRestarts,InvocationID", unit)
	if err != nil {
		return UnitState{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	if values["ActiveState"] == "" {
		return UnitState{}, errors.New("systemd returned no unit state")
	}
	return UnitState{
		ActiveState: values["ActiveState"], SubState: values["SubState"], Result: values["Result"],
		InvocationID: values["InvocationID"],
		MainPID:      integer(values["MainPID"]), ExecMainCode: integer(values["ExecMainCode"]), ExecMainStatus: integer(values["ExecMainStatus"]), NRestarts: integer(values["NRestarts"]),
	}, nil
}

func (m Manager) IsEnabled(ctx context.Context, unit string) bool {
	if unit != "omastackd.service" && unit != "omastack-service@.service" {
		return false
	}
	_, err := m.run(ctx, "is-enabled", unit)
	return err == nil
}

func (m Manager) action(ctx context.Context, action, serviceID string) error {
	unit, err := UnitName(serviceID)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, action, unit)
	return err
}

func (m Manager) run(ctx context.Context, args ...string) (string, error) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "systemctl", append([]string{"--user"}, args...)...)
	stdout := bounded.NewBuffer(1 << 20)
	stderr := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func integer(value string) int { result, _ := strconv.Atoi(value); return result }
