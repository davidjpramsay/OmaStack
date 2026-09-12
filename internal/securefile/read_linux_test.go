//go:build linux

package securefile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadRegularRefusesSymlinkAndPublicMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(link, 100, true); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(target, 100, true); err == nil {
		t.Fatal("public mode accepted")
	}
}

func TestReadRegularRejectsFIFOWithoutWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := ReadRegular(path, 100, true); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		// Release a regressed blocking open without leaving the test hung.
		fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = syscall.Close(fd)
		}
		t.Fatal("FIFO read blocked before type validation")
	}
	if _, err := ReadRegular(filepath.Dir(path), 100, false); err == nil {
		t.Fatal("directory accepted")
	}
	regular := filepath.Join(filepath.Dir(path), "regular")
	if err := os.WriteFile(regular, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadRegular(regular, 100, true)
	if err != nil || string(data) != "safe" {
		t.Fatalf("regular file: %q %v", data, err)
	}
}

func TestReadRegularEnforcesLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(path, 4, true); err == nil {
		t.Fatal("oversize file accepted")
	}
}
