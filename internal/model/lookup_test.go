package model

import "testing"

func TestConfigLookupsAndDefaults(t *testing.T) {
	config := DefaultConfig()
	project := Project{ID: "11111111-1111-4111-8111-111111111111", Name: "App"}
	project.Services = []Service{{ID: "22222222-2222-4222-8222-222222222222", Name: "API"}}
	config.Projects = []Project{project}
	if config.Settings.LogBufferLines != 2000 || config.Settings.HistorySamples != 60 {
		t.Fatalf("defaults = %#v", config.Settings)
	}
	if got, ok := config.FindProject("App"); !ok || got.ID != project.ID {
		t.Fatalf("project lookup = %#v, %v", got, ok)
	}
	if owner, service, ok := config.FindService("App/API"); !ok || owner.ID != project.ID || service.ID != project.Services[0].ID {
		t.Fatalf("service lookup = %#v %#v %v", owner, service, ok)
	}
	if _, ok := config.FindProject("missing"); ok {
		t.Fatal("missing project found")
	}
	if _, _, ok := config.FindService("missing"); ok {
		t.Fatal("missing service found")
	}
	services := config.AllServices()
	if len(services) != 1 || services[project.Services[0].ID].Name != "API" {
		t.Fatalf("services = %#v", services)
	}
}
