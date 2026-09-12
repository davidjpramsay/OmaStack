package daemon

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"omastack/internal/model"
)

func TestShutdownRetainsOfflineProjectSnapshot(t *testing.T) {
	d := testDaemon(t)
	if err := d.store.Update(func(c *model.Config) error {
		c.Projects = []model.Project{{ID: "11111111-1111-4111-8111-111111111111", Name: "Saved project", Services: []model.Service{}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	eventually(t, func() bool { return d.Snapshot().Connected })
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop")
	}
	data, err := os.ReadFile(d.paths.SnapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot model.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Connected || snapshot.BackendPID != 0 || len(snapshot.Projects) != 1 || snapshot.Projects[0].Name != "Saved project" {
		t.Fatalf("offline snapshot lost projects or reports a live backend: %+v", snapshot)
	}
	if _, err := os.Stat(d.paths.SocketFile); !os.IsNotExist(err) {
		t.Fatalf("control socket remains after shutdown: %v", err)
	}
}
