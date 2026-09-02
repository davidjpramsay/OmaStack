package validate

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	qmlTextBlock       = regexp.MustCompile(`\bText\s*\{`)
	qmlPlainTextBlock  = regexp.MustCompile(`\bText\s*\{\s*(?:id\s*:\s*[A-Za-z_][A-Za-z0-9_]*\s*)?textFormat\s*:\s*Text\.PlainText\b`)
	qmlUnsafeTextMode  = regexp.MustCompile(`Text\.(AutoText|RichText|StyledText|MarkdownText)`)
	qmlPlainLogMessage = regexp.MustCompile(`TextEdit\s*\{[^}]*textFormat\s*:\s*TextEdit\.PlainText`)
)

func TestProductionQMLTextUsesPlainText(t *testing.T) {
	root := filepath.Join("..", "..", "qml")
	total := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".qml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		blocks := len(qmlTextBlock.FindAll(data, -1))
		plain := len(qmlPlainTextBlock.FindAll(data, -1))
		if blocks != plain {
			t.Errorf("%s has %d Text blocks but %d start with Text.PlainText", path, blocks, plain)
		}
		if qmlUnsafeTextMode.Match(data) {
			t.Errorf("%s explicitly enables automatic or rich text", path)
		}
		total += blocks
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("no production QML Text blocks found")
	}

	logsView, err := os.ReadFile(filepath.Join(root, "components", "LogsView.qml"))
	if err != nil {
		t.Fatal(err)
	}
	if !qmlPlainLogMessage.Match(logsView) {
		t.Fatal("log message TextEdit does not explicitly use TextEdit.PlainText")
	}
}

func TestQMLLogLoaderUsesConfiguredBufferSize(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "qml", "Service.qml"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{"snapshot.settings.logBufferLines", `"--lines", String(configuredLines)`} {
		if !strings.Contains(source, required) {
			t.Errorf("Service.qml is missing %q", required)
		}
	}
	if strings.Contains(source, `"--lines", "1000"`) {
		t.Fatal("Service.qml still hardcodes the log line count")
	}
}

func TestCompactServiceRowOffersBrowserOpen(t *testing.T) {
	row, err := os.ReadFile(filepath.Join("..", "..", "qml", "components", "CompactServiceRow.qml"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(row)
	for _, required := range []string{
		"readonly property string browserUrl",
		"serviceData.url",
		"runtimeData.ports",
		`"http://127.0.0.1:"`,
		"root.openRequested(root.browserUrl)",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("CompactServiceRow.qml is missing %q", required)
		}
	}

	panel, err := os.ReadFile(filepath.Join("..", "..", "qml", "CompactPanel.qml"))
	if err != nil {
		t.Fatal(err)
	}
	panelSource := string(panel)
	for _, required := range []string{
		"function openBrowserUrl(value)",
		`openUrlProcess.command = ["xdg-open", target]`,
		"onOpenRequested: function(url) { root.openBrowserUrl(url) }",
	} {
		if !strings.Contains(panelSource, required) {
			t.Errorf("CompactPanel.qml is missing %q", required)
		}
	}
}
