package logs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"omastack/internal/bounded"
	"omastack/internal/systemd"
)

const maxMessageBytes = 16 << 10

type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	ServiceID string    `json:"serviceId"`
	Service   string    `json:"service,omitempty"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type Result struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
	Limit     int     `json:"limit"`
	Notice    string  `json:"notice,omitempty"`
}

func Read(ctx context.Context, serviceID string, lines int, query string) ([]Entry, error) {
	entries, _, err := ReadDetailed(ctx, serviceID, lines, query)
	return entries, err
}

func ReadDetailed(ctx context.Context, serviceID string, lines int, query string) ([]Entry, bool, error) {
	unit, err := systemd.UnitName(serviceID)
	if err != nil {
		return nil, false, err
	}
	if lines < 1 {
		lines = 200
	}
	if lines > 50000 {
		lines = 50000
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Retain the newest records if the capture byte limit is reached.
	cmd := exec.CommandContext(callCtx, "journalctl", "--user-unit="+unit, "--no-pager", "--output=json", "--reverse", "--lines="+strconv.Itoa(lines))
	output := bounded.NewBuffer(16 << 20)
	stderr := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = output, stderr
	if err := cmd.Run(); err != nil {
		return nil, false, fmt.Errorf("journalctl: %s", strings.TrimSpace(stderr.String()))
	}
	result := make([]Entry, 0, lines)
	scanner := bufio.NewScanner(bytes.NewReader(output.Bytes()))
	scanner.Buffer(make([]byte, 4096), (16<<20)+1)
	truncated := output.Truncated
	needle := strings.ToLower(query)
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		message := stringValue(raw["MESSAGE"])
		stream := "system"
		if prefix, rest, ok := strings.Cut(message, "\t"); ok && (prefix == "stdout" || prefix == "stderr") {
			stream, message = prefix, rest
		}
		if needle != "" && !strings.Contains(strings.ToLower(message), needle) && !strings.Contains(stream, needle) {
			continue
		}
		if len(message) > maxMessageBytes {
			// Do not expose an unredactable prefix of a legacy/raw secret.
			message = "[OmaStack: oversized journal message omitted (>16 KiB)]"
			truncated = true
		}
		micros, _ := strconv.ParseInt(stringValue(raw["__REALTIME_TIMESTAMP"]), 10, 64)
		result = append(result, Entry{Timestamp: time.UnixMicro(micros).UTC(), ServiceID: serviceID, Stream: stream, Message: message})
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, truncated, scanner.Err()
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
