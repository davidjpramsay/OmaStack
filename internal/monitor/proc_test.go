package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcStatHandlesSpacesInCommand(t *testing.T) {
	line := "42 (worker with spaces) S 1 0 0 0 0 0 0 0 0 0 10 5 0 0 0 0 0 0 0 0 25"
	sample, err := ParseProcStat(42, line, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if sample.PPID != 1 || sample.Ticks != 15 || sample.RSSBytes != 25*4096 {
		t.Fatalf("sample=%#v", sample)
	}
}

func TestListeningPortsFromSyntheticProc(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "123", "fd"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[98765]", filepath.Join(root, "123", "fd", "4")); err != nil {
		t.Fatal(err)
	}
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n   0: 0100007F:0BB8 00000000:0000 0A 0:0 0:0 00000000 1000 0 98765\n"
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	ports, err := ListeningPorts(root, []int{123})
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0] != 3000 {
		t.Fatalf("ports=%v", ports)
	}
}
