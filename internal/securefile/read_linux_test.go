//go:build linux

package securefile

import (
	"os"
	"path/filepath"
	"testing"
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

func TestReadRegularEnforcesLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(path, 4, true); err == nil {
		t.Fatal("oversize file accepted")
	}
}
