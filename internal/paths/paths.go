package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	ConfigDir          string
	ConfigFile         string
	RuntimeDir         string
	SocketFile         string
	SnapshotFile       string
	StateDir           string
	CacheDir           string
	RuntimeServicesDir string
	ExportDir          string
}

func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	configHome, err := absoluteEnv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err != nil {
		return Paths{}, err
	}
	stateHome, err := absoluteEnv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	if err != nil {
		return Paths{}, err
	}
	cacheHome, err := absoluteEnv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err != nil {
		return Paths{}, err
	}
	runtimeDefault := fmt.Sprintf("/run/user/%d", os.Getuid())
	runtimeHome, err := absoluteEnv("XDG_RUNTIME_DIR", runtimeDefault)
	if err != nil {
		return Paths{}, err
	}

	result := Paths{
		ConfigDir:  filepath.Join(configHome, "omastack"),
		RuntimeDir: filepath.Join(runtimeHome, "omastack"),
		StateDir:   filepath.Join(stateHome, "omastack"),
		CacheDir:   filepath.Join(cacheHome, "omastack"),
	}
	result.ConfigFile = filepath.Join(result.ConfigDir, "config.json")
	result.ExportDir = filepath.Join(result.ConfigDir, "exports")
	result.SocketFile = filepath.Join(result.RuntimeDir, "control.sock")
	result.SnapshotFile = filepath.Join(result.RuntimeDir, "state.json")
	result.RuntimeServicesDir = filepath.Join(result.StateDir, "services")
	return result, nil
}

func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir, p.ExportDir, p.RuntimeDir, p.StateDir, p.CacheDir, p.RuntimeServicesDir} {
		if err := ensureDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func absoluteEnv(name, fallback string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		value = fallback
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be absolute", name)
	}
	return filepath.Clean(value), nil
}

func ensureDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink directory %s", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", path)
		}
		return os.Chmod(path, 0o700)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}
