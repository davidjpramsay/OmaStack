package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(link, []byte("unsafe"), 0o600); err == nil {
		t.Fatal("symlink write succeeded")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "safe" {
		t.Fatalf("target changed to %q", data)
	}
}
