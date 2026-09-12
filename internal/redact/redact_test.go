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

func TestTextDoesNotReprocessMasks(t *testing.T) {
	secrets := []string{"•", "•", "•", "•", "•", "•", "•", "•", "•", "•", Mask, ""}
	if got := Text("before • after", secrets); got != "before "+Mask+" after" {
		t.Fatalf("mask amplification: %q", got)
	}
	if got := Text("token-long token", []string{"token", "token-long", "•"}); got != Mask+" "+Mask {
		t.Fatalf("overlap: %q", got)
	}
	if got := Text("ordinary output", secrets); got != "ordinary output" {
		t.Fatalf("ordinary text changed: %q", got)
	}
}

func TestTextBoundsExpandedOutputWithoutPartialDisclosure(t *testing.T) {
	got := Text(strings.Repeat("x", 100000)+"private-tail", []string{"x", "private-tail"})
	if got != "[OmaStack: message omitted because redacted output exceeds 1 MiB]" {
		t.Fatalf("expected complete omission, got %d bytes", len(got))
	}
}
