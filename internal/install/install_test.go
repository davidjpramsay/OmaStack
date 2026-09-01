package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omastack/internal/paths"
)

func TestSecureDirectoryRejectsWritableExistingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := secureDirectory(path, 0o700); err != nil {
		t.Fatalf("private owned directory rejected: %v", err)
	}
	if err := os.Chmod(path, 0o722); err != nil {
		t.Fatal(err)
	}
	if err := secureDirectory(path, 0o700); err == nil || !strings.Contains(err.Error(), "group/world-writable") {
		t.Fatalf("writable directory error = %v", err)
	}
}

func TestSecureDirectoryCreatesPrivateDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "install")
	if err := secureDirectory(path, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestRemoveRegularRejectsNonRegularTargets(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing")
	if err := removeRegular(missing); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeRegular(file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("regular file still exists: %v", err)
	}
	if err := removeRegular(directory); err == nil {
		t.Fatal("directory accepted")
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(missing, link); err != nil {
		t.Fatal(err)
	}
	if err := removeRegular(link); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestPurgeRemovesOnlyValidatedOmaStackDirectories(t *testing.T) {
	base := t.TempDir()
	makePath := func(parent string) string {
		path := filepath.Join(base, parent, "omastack")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	resolved := paths.Paths{
		ConfigDir: makePath("config"), RuntimeDir: makePath("runtime"),
		StateDir: makePath("state"), CacheDir: makePath("cache"),
	}
	if err := Purge(resolved); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{resolved.ConfigDir, resolved.RuntimeDir, resolved.StateDir, resolved.CacheDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("purged path remains %s: %v", path, err)
		}
	}
	bad := resolved
	bad.ConfigDir = filepath.Join(base, "not-omastack")
	if err := Purge(bad); err == nil {
		t.Fatal("unsafe purge basename accepted")
	}
}

func TestSetupAndUninstallLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	helperDir := filepath.Join(home, "helpers")
	if err := os.Mkdir(helperDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(home, "systemctl.log")
	systemctlPath := filepath.Join(helperDir, "systemctl")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$OMASTACK_SYSTEMCTL_LOG\"\n"
	if err := os.WriteFile(systemctlPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", helperDir)
	t.Setenv("OMASTACK_SYSTEMCTL_LOG", logFile)

	base := filepath.Join(home, "xdg")
	resolved := paths.Paths{
		ConfigDir:          filepath.Join(base, "config", "omastack"),
		ConfigFile:         filepath.Join(base, "config", "omastack", "config.json"),
		RuntimeDir:         filepath.Join(base, "runtime", "omastack"),
		SocketFile:         filepath.Join(base, "runtime", "omastack", "control.sock"),
		SnapshotFile:       filepath.Join(base, "runtime", "omastack", "state.json"),
		StateDir:           filepath.Join(base, "state", "omastack"),
		CacheDir:           filepath.Join(base, "cache", "omastack"),
		RuntimeServicesDir: filepath.Join(base, "state", "omastack", "services"),
		ExportDir:          filepath.Join(base, "config", "omastack", "exports"),
	}
	result, err := Setup(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{result.Binary, result.DaemonUnit, result.ServiceUnit} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("installed file %s: info=%v err=%v", path, info, err)
		}
	}
	if info, err := os.Stat(result.Binary); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("binary mode: info=%v err=%v", info, err)
	}
	for _, path := range []string{resolved.SocketFile, resolved.SnapshotFile} {
		if err := os.WriteFile(path, []byte("temporary"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const serviceID = "12345678-1234-4234-8234-123456789abc"
	if err := Uninstall(context.Background(), resolved, []string{serviceID}, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{result.Binary, result.DaemonUnit, result.ServiceUnit, resolved.SocketFile, resolved.SnapshotFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("uninstalled path remains %s: %v", path, err)
		}
	}
	if _, err := os.Stat(resolved.ConfigDir); err != nil {
		t.Fatalf("configuration directory was not preserved: %v", err)
	}
	commands, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"--user daemon-reload",
		"--user enable --now omastackd.service",
		"--user disable --now omastackd.service",
		"--user stop omastack-service@" + serviceID + ".service",
	} {
		if !strings.Contains(string(commands), expected) {
			t.Errorf("missing systemctl invocation %q in:\n%s", expected, commands)
		}
	}
}
