package install

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"omastack/internal/bounded"
	"omastack/internal/paths"
	"omastack/internal/securefile"
	"omastack/internal/store"
	"omastack/internal/systemd"
)

//go:embed assets/omastackd.service
var daemonUnit []byte

//go:embed assets/omastack-service@.service
var serviceUnit []byte

type Result struct {
	Binary      string `json:"binary"`
	DaemonUnit  string `json:"daemonUnit"`
	ServiceUnit string `json:"serviceUnit"`
}

func Setup(ctx context.Context, resolved paths.Paths) (Result, error) {
	if err := resolved.Ensure(); err != nil {
		return Result{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{}, err
	}
	binDir := filepath.Join(home, ".local", "bin")
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := secureDirectory(binDir, 0o755); err != nil {
		return Result{}, err
	}
	if err := secureDirectory(unitDir, 0o700); err != nil {
		return Result{}, err
	}
	target := filepath.Join(binDir, "omastack")
	current, err := os.Executable()
	if err != nil {
		return Result{}, err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return Result{}, err
	}
	if current != target {
		data, err := securefile.ReadRegular(current, 128<<20, false)
		if err != nil {
			return Result{}, err
		}
		if err := store.AtomicWrite(target, data, 0o755); err != nil {
			return Result{}, err
		}
	}
	daemonPath := filepath.Join(unitDir, "omastackd.service")
	servicePath := filepath.Join(unitDir, "omastack-service@.service")
	if err := store.AtomicWrite(daemonPath, daemonUnit, 0o644); err != nil {
		return Result{}, err
	}
	if err := store.AtomicWrite(servicePath, serviceUnit, 0o644); err != nil {
		return Result{}, err
	}
	if _, err := systemctl(ctx, "daemon-reload"); err != nil {
		return Result{}, err
	}
	_, activeErr := systemctl(ctx, "is-active", "--quiet", "omastackd.service")
	if activeErr == nil {
		// Older daemons do not have the session autostart marker yet. Preserve
		// the current app selection when upgrading them too.
		if err := store.AtomicWrite(filepath.Join(resolved.RuntimeDir, "autostart-attempted"), []byte("1\n"), 0o600); err != nil {
			return Result{}, err
		}
	}
	if _, err := systemctl(ctx, "enable", "omastackd.service"); err != nil {
		return Result{}, err
	}
	action := "start"
	if activeErr == nil {
		action = "restart"
	}
	if _, err := systemctl(ctx, action, "omastackd.service"); err != nil {
		return Result{}, err
	}
	if _, err := systemctl(ctx, "is-active", "--quiet", "omastackd.service"); err != nil {
		return Result{}, fmt.Errorf("new daemon did not become active: %w", err)
	}
	return Result{Binary: target, DaemonUnit: daemonPath, ServiceUnit: servicePath}, nil
}

func Uninstall(ctx context.Context, resolved paths.Paths, serviceIDs []string, removeBinary bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	daemonPath := filepath.Join(unitDir, "omastackd.service")
	if _, err := os.Lstat(daemonPath); err == nil {
		if _, err := systemctl(ctx, "disable", "--now", "omastackd.service"); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Stop the control plane before service shutdown so no OmaStack client can
	// race the uninstall by starting a unit after it has been stopped.
	manager := systemd.Manager{Timeout: 310 * time.Second}
	for _, id := range serviceIDs {
		if err := manager.Stop(ctx, id); err != nil {
			return fmt.Errorf("stop managed service %s: %w", id, err)
		}
	}
	for _, name := range []string{"omastackd.service", "omastack-service@.service"} {
		path := filepath.Join(unitDir, name)
		if err := removeRegular(path); err != nil {
			return err
		}
	}
	if _, err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	for _, path := range []string{resolved.SocketFile, resolved.SnapshotFile} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	// Remove the runtime directory only when it is empty. Unknown files are
	// preserved instead of being deleted recursively by a non-purge uninstall.
	_ = os.Remove(resolved.RuntimeDir)
	if removeBinary {
		if err := removeRegular(filepath.Join(home, ".local", "bin", "omastack")); err != nil {
			return err
		}
	}
	return nil
}

func Purge(resolved paths.Paths) error {
	for _, path := range []string{resolved.ConfigDir, resolved.RuntimeDir, resolved.StateDir, resolved.CacheDir} {
		if filepath.Base(path) != "omastack" || path == "/" || path == "." {
			return fmt.Errorf("refusing unsafe purge path %s", path)
		}
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing symlink purge path %s", path)
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func secureDirectory(path string, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("refusing unsafe directory %s", path)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Getuid() {
			return fmt.Errorf("refusing directory not owned by current user %s", path)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("refusing group/world-writable directory %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, mode)
}

func removeRegular(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular file %s", path)
	}
	return os.Remove(path)
}

func systemctl(ctx context.Context, args ...string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "systemctl", append([]string{"--user"}, args...)...)
	stdout := bounded.NewBuffer(1 << 20)
	stderr := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
