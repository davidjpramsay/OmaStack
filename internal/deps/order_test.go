package deps

import (
	"errors"
	"reflect"
	"testing"

	"omastack/internal/model"
)

func TestDependencyOrderAndReverseShutdown(t *testing.T) {
	services := map[string]model.Service{
		"db":  {ID: "db"},
		"api": {ID: "api", Dependencies: []model.Dependency{{ServiceID: "db", Condition: "healthy"}}},
		"web": {ID: "web", Dependencies: []model.Dependency{{ServiceID: "api", Condition: "started"}}},
	}
	start, err := StartupOrder(services, []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(start, []string{"db", "api", "web"}) {
		t.Fatalf("start order = %v", start)
	}
	stop, err := ShutdownOrder(services, []string{"web", "db", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stop, []string{"web", "api", "db"}) {
		t.Fatalf("stop order = %v", stop)
	}
}

func TestTargetedShutdownPreservesPrerequisites(t *testing.T) {
	services := map[string]model.Service{
		"db":     {ID: "db"},
		"api":    {ID: "api", Dependencies: []model.Dependency{{ServiceID: "db"}}},
		"worker": {ID: "worker", Dependencies: []model.Dependency{{ServiceID: "db"}}},
	}
	for _, targets := range [][]string{{"api"}, {"api", "worker"}} {
		got, err := ShutdownOrder(services, targets)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range got {
			if id == "db" {
				t.Fatal("shutdown expanded outside the selection")
			}
		}
		if len(got) != len(targets) {
			t.Fatalf("order=%v", got)
		}
	}
}

func TestDependencyCycle(t *testing.T) {
	services := map[string]model.Service{
		"a": {ID: "a", Dependencies: []model.Dependency{{ServiceID: "b"}}},
		"b": {ID: "b", Dependencies: []model.Dependency{{ServiceID: "a"}}},
	}
	_, err := StartupOrder(services, []string{"a"})
	var cycle *CycleError
	if !errors.As(err, &cycle) || len(cycle.Path) != 3 {
		t.Fatalf("cycle = %#v, err = %v", cycle, err)
	}
}
