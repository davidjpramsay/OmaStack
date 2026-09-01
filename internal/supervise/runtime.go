package supervise

import (
	"encoding/json"
	"path/filepath"
	"time"

	"omastack/internal/securefile"
	"omastack/internal/store"
)

type RuntimeRecord struct {
	ServiceID     string     `json:"serviceId"`
	SupervisorPID int        `json:"supervisorPid"`
	PID           int        `json:"pid"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	StoppedAt     *time.Time `json:"stoppedAt,omitempty"`
	ExitCode      *int       `json:"exitCode,omitempty"`
	Signal        string     `json:"signal,omitempty"`
	RestartCount  int        `json:"restartCount"`
	LastError     string     `json:"lastError,omitempty"`
}

func runtimePath(dir, serviceID string) string { return filepath.Join(dir, serviceID+".json") }

func WriteRuntime(dir string, record RuntimeRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return store.AtomicWrite(runtimePath(dir, record.ServiceID), append(data, '\n'), 0o600)
}

func ReadRuntime(dir, serviceID string) (RuntimeRecord, error) {
	data, err := osReadFile(runtimePath(dir, serviceID))
	if err != nil {
		return RuntimeRecord{}, err
	}
	var record RuntimeRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RuntimeRecord{}, err
	}
	return record, nil
}

var osReadFile = func(path string) ([]byte, error) {
	return securefile.ReadRegular(path, 64<<10, true)
}
