package logs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadParsesFiltersAndBoundsJournalRequest(t *testing.T) {
	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "arguments")
	script := `#!/bin/sh
printf '%s\n' "$*" > "$OMASTACK_JOURNAL_TEST_ARGS"
printf '%s\n' \
  '{"__REALTIME_TIMESTAMP":"1000000","MESSAGE":"stdout\tready"}' \
  '{"__REALTIME_TIMESTAMP":"2000000","MESSAGE":"stderr\tfailed request"}' \
  '{not-json}'
`
	path := filepath.Join(directory, "journalctl")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OMASTACK_JOURNAL_TEST_ARGS", argumentsPath)

	entries, err := Read(context.Background(), "22222222-2222-4222-8222-222222222222", 999999, "FAILED")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Stream != "stderr" || entries[0].Message != "failed request" || entries[0].Timestamp.Unix() != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "--lines=50000") {
		t.Fatalf("journal arguments = %q", arguments)
	}
}

func TestStringValueHandlesJournalRepresentations(t *testing.T) {
	if got := stringValue("text"); got != "text" {
		t.Fatal(got)
	}
	if got := stringValue(json.Number("123")); got != "123" {
		t.Fatal(got)
	}
	if got := stringValue([]any{65, 66}); got != "[65 66]" {
		t.Fatal(got)
	}
}
