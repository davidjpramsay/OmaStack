package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAndEnsureUsePrivateXDGDirectories(t *testing.T) {
	base := t.TempDir()
	configHome := filepath.Join(base, "config")
	stateHome := filepath.Join(base, "state")
	cacheHome := filepath.Join(base, "cache")
	runtimeHome := filepath.Join(base, "runtime")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("XDG_RUNTIME_DIR", runtimeHome)

	resolved, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ConfigFile != filepath.Join(configHome, "omastack", "config.json") || resolved.SocketFile != filepath.Join(runtimeHome, "omastack", "control.sock") {
		t.Fatalf("unexpected paths: %#v", resolved)
	}
	if err := resolved.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{resolved.ConfigDir, resolved.ExportDir, resolved.RuntimeDir, resolved.StateDir, resolved.CacheDir, resolved.RuntimeServicesDir} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %v", directory, info.Mode())
		}
	}
}

func TestResolveRejectsRelativeXDGPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if _, err := Resolve(); err == nil {
		t.Fatal("relative XDG_CONFIG_HOME accepted")
	}
}

func TestEnsureDirRejectsSymlinkAndRegularFile(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(link); err == nil {
		t.Fatal("symlink directory accepted")
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(file); err == nil {
		t.Fatal("regular file accepted as directory")
	}
}
