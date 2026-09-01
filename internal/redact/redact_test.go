package redact

import (
	"strings"
	"testing"

	"omastack/internal/model"
)

func TestSecretRedaction(t *testing.T) {
	config := model.DefaultConfig()
	config.Projects = []model.Project{{Services: []model.Service{{Environment: map[string]model.EnvValue{"TOKEN": {Value: "super-secret-token", Secret: true}, "PIN": {Value: "x", Secret: true}, "PUBLIC": {Value: "visible"}}}}}}
	secrets := Secrets(config)
	text := Text("failed with super-secret-token and x", secrets)
	if strings.Contains(text, "super-secret-token") || strings.Contains(text, " x") || !strings.Contains(text, Mask) {
		t.Fatalf("redacted text=%q", text)
	}
	redacted := Config(config)
	if redacted.Projects[0].Services[0].Environment["TOKEN"].Value != Mask {
		t.Fatal("config secret not masked")
	}
	if redacted.Projects[0].Services[0].Environment["PUBLIC"].Value != "visible" {
		t.Fatal("public value masked")
	}
}
