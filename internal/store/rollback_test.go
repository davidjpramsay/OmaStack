package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"omastack/internal/model"
)

func TestFailedNestedUpdateIsAtomic(t *testing.T) {
	for _, mode := range []string{"validation", "mutator", "persistence"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			s, err := OpenConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Update(func(c *model.Config) error {
				p := validProject()
				p.Services[0].Environment = map[string]model.EnvValue{"TOKEN": {Value: "kept", Secret: true}}
				p.Services[0].Command.Arguments = []string{"one", "two"}
				p.Services[0].Route = &model.Route{Hostname: "api.localhost", TargetPort: 3000}
				c.Projects = []model.Project{p}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := s.Get()
			disk, _ := os.ReadFile(path)
			if mode == "persistence" {
				s.path = filepath.Join(t.TempDir(), "missing", "config.json")
			}
			err = s.Update(func(c *model.Config) error {
				svc := &c.Projects[0].Services[0]
				svc.Name = "changed"
				svc.Command.Arguments[0] = "changed"
				delete(svc.Environment, "TOKEN")
				svc.Route.TargetPort = 4000
				c.Projects[0].Services = append(c.Projects[0].Services[:0], *svc)
				if mode == "validation" {
					svc.Name = ""
					c.Settings.PollIntervalSeconds = 0
				}
				if mode == "mutator" {
					return errors.New("rejected")
				}
				return nil
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			if !reflect.DeepEqual(s.Get(), before) {
				t.Fatal("rejected update changed live config")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, disk) {
				t.Fatal("rejected update changed disk")
			}
			copy := s.Get()
			copy.Projects[0].Services[0].Route.TargetPort = 9000
			if !reflect.DeepEqual(s.Get(), before) {
				t.Fatal("Get aliases live config")
			}
		})
	}
}
