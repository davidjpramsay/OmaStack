package validate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"omastack/internal/model"
)

var (
	idPattern      = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	colorPattern   = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	dockerName     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	hostLabel      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

func ID(value string) bool { return idPattern.MatchString(value) }

func Config(c model.Config) error {
	if c.Version != model.CurrentConfigVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.Settings.PollIntervalSeconds < 1 || c.Settings.PollIntervalSeconds > 60 {
		return errors.New("poll interval must be between 1 and 60 seconds")
	}
	if c.Settings.HistorySamples < 10 || c.Settings.HistorySamples > 600 {
		return errors.New("historySamples must be between 10 and 600")
	}
	if c.Settings.LogBufferLines < 100 || c.Settings.LogBufferLines > 50000 {
		return errors.New("logBufferLines must be between 100 and 50000")
	}
	if c.Settings.Proxy.ListenHost != "127.0.0.1" && c.Settings.Proxy.ListenHost != "::1" {
		return errors.New("proxy must listen on loopback")
	}
	if !Port(c.Settings.Proxy.HTTPPort) || !Port(c.Settings.Proxy.HTTPSPort) {
		return errors.New("proxy ports must be between 1 and 65535")
	}
	seenProjects := map[string]bool{}
	seenServices := map[string]bool{}
	if len(c.Projects) > 64 {
		return errors.New("configuration contains more than 64 projects")
	}
	totalServices := 0
	for pi := range c.Projects {
		p := &c.Projects[pi]
		if len(p.Services) > 64 {
			return fmt.Errorf("project %q contains more than 64 services", p.Name)
		}
		totalServices += len(p.Services)
		if totalServices > 128 {
			return errors.New("configuration contains more than 128 services")
		}
		if err := Project(p); err != nil {
			return fmt.Errorf("project %d: %w", pi, err)
		}
		if seenProjects[p.ID] {
			return fmt.Errorf("duplicate project id %q", p.ID)
		}
		seenProjects[p.ID] = true
		for si := range p.Services {
			s := &p.Services[si]
			if err := Service(s); err != nil {
				return fmt.Errorf("project %q service %d: %w", p.Name, si, err)
			}
			if seenServices[s.ID] {
				return fmt.Errorf("duplicate service id %q", s.ID)
			}
			seenServices[s.ID] = true
		}
	}
	for _, p := range c.Projects {
		for _, s := range p.Services {
			for _, dep := range s.Dependencies {
				if !seenServices[dep.ServiceID] {
					return fmt.Errorf("service %q depends on unknown service %q", s.Name, dep.ServiceID)
				}
				if dep.ServiceID == s.ID {
					return fmt.Errorf("service %q depends on itself", s.Name)
				}
			}
		}
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	if len(encoded) > 512<<10 {
		return errors.New("configuration exceeds 512 KiB")
	}
	return nil
}

func Project(p *model.Project) error {
	if !ID(p.ID) {
		return errors.New("invalid project id")
	}
	if err := Text("project name", p.Name, 1, 80); err != nil {
		return err
	}
	if err := Text("project description", p.Description, 0, 500); err != nil {
		return err
	}
	if p.Color != "" && !colorPattern.MatchString(p.Color) {
		return errors.New("project color must be #RRGGBB")
	}
	if len(p.Icon) > 8 {
		return errors.New("project icon is too long")
	}
	return nil
}

func Service(s *model.Service) error {
	if !ID(s.ID) {
		return errors.New("invalid service id")
	}
	if err := Text("service name", s.Name, 1, 80); err != nil {
		return err
	}
	if err := Text("service description", s.Description, 0, 500); err != nil {
		return err
	}
	if err := Text("service notes", s.Notes, 0, 16384); err != nil {
		return err
	}
	if s.EnvironmentFile != "" {
		if err := Path("environment file", s.EnvironmentFile, true); err != nil {
			return err
		}
	}
	if s.Docker == nil {
		if err := Path("working directory", s.WorkingDirectory, true); err != nil {
			return err
		}
		if s.Shell != nil && s.Shell.Enabled {
			if err := Path("shell executable", s.Shell.Shell, true); err != nil {
				return err
			}
			if err := Text("shell command", s.Shell.Command, 1, 16384); err != nil {
				return err
			}
		} else if err := Command(s.Command); err != nil {
			return err
		}
	} else if s.Shell != nil && s.Shell.Enabled {
		return errors.New("docker and shell modes cannot be combined")
	}
	if len(s.Environment) > 256 {
		return errors.New("too many environment variables")
	}
	for name, value := range s.Environment {
		if !envNamePattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
		if len(value.Value) > 65536 || strings.ContainsRune(value.Value, '\x00') {
			return fmt.Errorf("invalid value for environment variable %q", name)
		}
	}
	if s.GracefulStopSeconds < 1 || s.GracefulStopSeconds > 300 {
		return errors.New("graceful stop timeout must be between 1 and 300 seconds")
	}
	if !validSignal(s.StopSignal) {
		return errors.New("unsupported stop signal")
	}
	if !contains([]string{"never", "on-failure", "always"}, s.Restart.Mode) {
		return errors.New("invalid restart policy")
	}
	if s.Restart.DelaySeconds < 0 || s.Restart.DelaySeconds > 3600 {
		return errors.New("restart delay is out of range")
	}
	if s.Restart.MaxAttempts < 0 || s.Restart.MaxAttempts > 1000 {
		return errors.New("restart max attempts is out of range")
	}
	if s.Restart.ResetAfterSeconds < 0 || s.Restart.ResetAfterSeconds > 86400 {
		return errors.New("restart reset period is out of range")
	}
	for _, dep := range s.Dependencies {
		if !ID(dep.ServiceID) {
			return errors.New("invalid dependency id")
		}
		if !contains([]string{"started", "healthy"}, dep.Condition) {
			return errors.New("dependency condition must be started or healthy")
		}
	}
	if len(s.Dependencies) > 128 {
		return errors.New("service has too many dependencies")
	}
	if s.Docker != nil {
		if err := Path("compose file", s.Docker.ComposeFile, true); err != nil {
			return err
		}
		if !dockerName.MatchString(s.Docker.Service) {
			return errors.New("invalid Compose service name")
		}
		if s.Docker.ProjectName != "" && !dockerName.MatchString(s.Docker.ProjectName) {
			return errors.New("invalid Compose project name")
		}
	}
	if s.URL != "" {
		if len(s.URL) > 2048 {
			return errors.New("URL is too long")
		}
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("URL must be HTTP or HTTPS")
		}
	}
	if s.Health != nil {
		if err := Health(s.Health); err != nil {
			return err
		}
	}
	if s.Route != nil {
		if !LocalHostname(s.Route.Hostname) {
			return errors.New("route hostname must be a valid .test or .localhost name")
		}
		if s.Route.TargetPort != 0 && !Port(s.Route.TargetPort) {
			return errors.New("route target port is invalid")
		}
		if s.Route.HTTPS {
			return errors.New("local HTTPS requires the privileged onboarding milestone and is not enabled")
		}
	}
	return nil
}

func Command(c model.CommandSpec) error {
	if err := Path("executable", c.Executable, false); err != nil {
		return err
	}
	if len(c.Arguments) > 128 {
		return errors.New("too many command arguments")
	}
	for _, arg := range c.Arguments {
		if len(arg) > 16384 || strings.ContainsRune(arg, '\x00') {
			return errors.New("invalid command argument")
		}
	}
	return nil
}

func Health(h *model.HealthCheck) error {
	if !contains([]string{"http", "https", "tcp", "command"}, h.Type) {
		return errors.New("invalid health-check type")
	}
	if h.IntervalSeconds < 1 || h.IntervalSeconds > 3600 {
		return errors.New("health-check interval is out of range")
	}
	if h.TimeoutSeconds < 1 || h.TimeoutSeconds > 60 || h.TimeoutSeconds > h.IntervalSeconds {
		return errors.New("health-check timeout is invalid")
	}
	if h.Retries < 1 || h.Retries > 20 {
		return errors.New("health-check retries is out of range")
	}
	if h.StartGraceSeconds < 0 || h.StartGraceSeconds > 3600 {
		return errors.New("health-check grace period is out of range")
	}
	switch h.Type {
	case "http", "https":
		if h.HTTP == nil {
			return errors.New("HTTP health-check configuration is required")
		}
		u, err := url.Parse(h.HTTP.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("invalid health-check URL")
		}
		if h.HTTP.ExpectedStatus < 100 || h.HTTP.ExpectedStatus > 599 {
			return errors.New("invalid expected HTTP status")
		}
		if h.HTTP.SkipTLSVerify {
			return errors.New("TLS certificate verification cannot be disabled")
		}
		if len(h.HTTP.ResponseContains) > 4096 {
			return errors.New("health response match is too long")
		}
		if len(h.HTTP.ResponseRegex) > 256 {
			return errors.New("health response regex is too long")
		}
		if h.HTTP.ResponseRegex != "" {
			if _, err := regexp.Compile(h.HTTP.ResponseRegex); err != nil {
				return fmt.Errorf("invalid health response regex: %w", err)
			}
		}
	case "tcp":
		if h.TCP == nil || !Port(h.TCP.Port) {
			return errors.New("invalid TCP health-check")
		}
		if net.ParseIP(h.TCP.Host) == nil && !hostname(h.TCP.Host) {
			return errors.New("invalid TCP health-check host")
		}
	case "command":
		if h.Command == nil {
			return errors.New("command health-check configuration is required")
		}
		if err := Command(*h.Command); err != nil {
			return fmt.Errorf("health check: %w", err)
		}
	}
	return nil
}

func Text(label, value string, min, max int) error {
	n := len([]rune(strings.TrimSpace(value)))
	if n < min || n > max || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must contain between %d and %d characters", label, min, max)
	}
	return nil
}

func Path(label, value string, requireAbsolute bool) error {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s is empty or invalid", label)
	}
	if requireAbsolute && !filepath.IsAbs(value) {
		return fmt.Errorf("%s must be absolute", label)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("%s must be normalized", label)
	}
	return nil
}

func Port(value int) bool { return value >= 1 && value <= 65535 }

func LocalHostname(value string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if len(h) < 3 || len(h) > 253 || !(strings.HasSuffix(h, ".test") || strings.HasSuffix(h, ".localhost")) {
		return false
	}
	return hostname(h)
}

func hostname(value string) bool {
	for _, label := range strings.Split(value, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validSignal(value string) bool {
	return contains([]string{"SIGTERM", "SIGINT", "SIGHUP", "SIGQUIT"}, value)
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
