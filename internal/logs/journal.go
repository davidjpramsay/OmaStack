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

func Read(ctx context.Context, serviceID string, lines int, query string) ([]Entry, error) {
	unit, err := systemd.UnitName(serviceID)
	if err != nil {
		return nil, err
	}
	if lines < 1 {
		lines = 200
	}
	if lines > 50000 {
		lines = 50000
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "journalctl", "--user-unit="+unit, "--no-pager", "--output=json", "--lines="+strconv.Itoa(lines))
	output := bounded.NewBuffer(16 << 20)
	stderr := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = output, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("journalctl: %s", strings.TrimSpace(stderr.String()))
	}
	result := make([]Entry, 0, lines)
	scanner := bufio.NewScanner(bytes.NewReader(output.Bytes()))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	needle := strings.ToLower(query)
	for scanner.Scan() {
		var raw map[string]any
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			continue
		}
		message := stringValue(raw["MESSAGE"])
		if len(message) > maxMessageBytes {
			message = message[:maxMessageBytes-3] + "…"
		}
		if needle != "" && !strings.Contains(strings.ToLower(message), needle) {
			continue
		}
		stream := "system"
		if prefix, rest, ok := strings.Cut(message, "\t"); ok && (prefix == "stdout" || prefix == "stderr") {
			stream, message = prefix, rest
		}
		micros, _ := strconv.ParseInt(stringValue(raw["__REALTIME_TIMESTAMP"]), 10, 64)
		result = append(result, Entry{Timestamp: time.UnixMicro(micros).UTC(), ServiceID: serviceID, Stream: stream, Message: message})
	}
	return result, scanner.Err()
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
