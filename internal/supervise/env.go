package supervise

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"omastack/internal/model"
	"omastack/internal/securefile"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var inheritedEnvironment = map[string]bool{
	"CLICOLOR": true, "CLICOLOR_FORCE": true, "COLORTERM": true,
	"DBUS_SESSION_BUS_ADDRESS": true, "DISPLAY": true,
	"DOCKER_CONTEXT": true, "DOCKER_HOST": true,
	"HOME": true, "LANG": true, "LANGUAGE": true, "LOGNAME": true,
	"NO_COLOR": true, "PATH": true, "SHELL": true,
	"TEMP": true, "TERM": true, "TERM_PROGRAM": true, "TMP": true, "TMPDIR": true, "TZ": true,
	"USER": true, "WAYLAND_DISPLAY": true, "XAUTHORITY": true,
	"XDG_CACHE_HOME": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
	"XDG_RUNTIME_DIR": true, "XDG_STATE_HOME": true,
}

func BuildEnvironment(base []string, envFile string, inline map[string]model.EnvValue) ([]string, error) {
	environment, _, err := BuildEnvironmentWithSecrets(base, envFile, inline)
	return environment, err
}

func BuildEnvironmentWithSecrets(base []string, envFile string, inline map[string]model.EnvValue) ([]string, []string, error) {
	values := make(map[string]string, len(base)+len(inline))
	secrets := []string{}
	for _, entry := range base {
		name, value, ok := strings.Cut(entry, "=")
		if ok && envName.MatchString(name) && shouldInheritEnvironment(name) {
			values[name] = value
		}
	}
	if envFile != "" {
		fileValues, err := readEnvironmentFile(envFile)
		if err != nil {
			return nil, nil, err
		}
		for name, value := range fileValues {
			values[name] = value
			if value != "" {
				secrets = append(secrets, value)
			}
		}
	}
	for name, value := range inline {
		values[name] = value.Value
		if value.Secret && value.Value != "" {
			secrets = append(secrets, value.Value)
		}
	}
	result := make([]string, 0, len(values))
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	sort.Slice(secrets, func(left, right int) bool { return len(secrets[left]) > len(secrets[right]) })
	return result, secrets, nil
}

func shouldInheritEnvironment(name string) bool {
	return inheritedEnvironment[name] || strings.HasPrefix(name, "LC_")
}

func readEnvironmentFile(path string) (map[string]string, error) {
	data, err := securefile.ReadRegular(path, 1<<20, true)
	if err != nil {
		return nil, err
	}
	return ParseEnvironment(bytes.NewReader(data))
}

func ParseEnvironment(reader io.Reader) (map[string]string, error) {
	result := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(reader, 1<<20))
	scanner.Buffer(make([]byte, 4096), 64*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !envName.MatchString(name) {
			return nil, fmt.Errorf("invalid environment entry on line %d", lineNo)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("NUL in environment entry on line %d", lineNo)
		}
		result[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
