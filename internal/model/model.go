package model

import "time"

const CurrentConfigVersion = 1

type Config struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Projects []Project `json:"projects"`
}

type Settings struct {
	PollIntervalSeconds int                  `json:"pollIntervalSeconds"`
	HistorySamples      int                  `json:"historySamples"`
	LogBufferLines      int                  `json:"logBufferLines"`
	Notifications       NotificationSettings `json:"notifications"`
	Proxy               ProxySettings        `json:"proxy"`
}

type NotificationSettings struct {
	Unhealthy bool `json:"unhealthy"`
	Crashed   bool `json:"crashed"`
	Recovered bool `json:"recovered"`
}

type ProxySettings struct {
	Enabled    bool   `json:"enabled"`
	ListenHost string `json:"listenHost"`
	HTTPPort   int    `json:"httpPort"`
	HTTPSPort  int    `json:"httpsPort"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Color       string    `json:"color,omitempty"`
	Order       int       `json:"order"`
	Services    []Service `json:"services"`
}

type Service struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Description         string              `json:"description,omitempty"`
	Command             CommandSpec         `json:"command"`
	WorkingDirectory    string              `json:"workingDirectory"`
	Environment         map[string]EnvValue `json:"environment,omitempty"`
	EnvironmentFile     string              `json:"environmentFile,omitempty"`
	Shell               *ShellSpec          `json:"shell,omitempty"`
	Autostart           bool                `json:"autostart"`
	Restart             RestartPolicy       `json:"restart"`
	StopSignal          string              `json:"stopSignal"`
	GracefulStopSeconds int                 `json:"gracefulStopSeconds"`
	Dependencies        []Dependency        `json:"dependencies,omitempty"`
	Docker              *DockerSpec         `json:"docker,omitempty"`
	URL                 string              `json:"url,omitempty"`
	Notes               string              `json:"notes,omitempty"`
	Health              *HealthCheck        `json:"health,omitempty"`
	Route               *Route              `json:"route,omitempty"`
}

type CommandSpec struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments,omitempty"`
}

type ShellSpec struct {
	Enabled bool   `json:"enabled"`
	Shell   string `json:"shell,omitempty"`
	Command string `json:"command,omitempty"`
}

type EnvValue struct {
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

type Dependency struct {
	ServiceID string `json:"serviceId"`
	Condition string `json:"condition"`
}

type RestartPolicy struct {
	Mode              string `json:"mode"`
	DelaySeconds      int    `json:"delaySeconds"`
	MaxAttempts       int    `json:"maxAttempts"`
	ResetAfterSeconds int    `json:"resetAfterSeconds"`
}

func (p RestartPolicy) ShouldRestart(exitCode int, attempt int) bool {
	if p.MaxAttempts > 0 && attempt >= p.MaxAttempts {
		return false
	}
	switch p.Mode {
	case "always":
		return true
	case "on-failure":
		return exitCode != 0
	default:
		return false
	}
}

type DockerSpec struct {
	ComposeFile string `json:"composeFile"`
	ProjectName string `json:"projectName,omitempty"`
	Service     string `json:"service"`
}

type HealthCheck struct {
	Type              string       `json:"type"`
	IntervalSeconds   int          `json:"intervalSeconds"`
	TimeoutSeconds    int          `json:"timeoutSeconds"`
	Retries           int          `json:"retries"`
	StartGraceSeconds int          `json:"startGraceSeconds"`
	HTTP              *HTTPCheck   `json:"http,omitempty"`
	TCP               *TCPCheck    `json:"tcp,omitempty"`
	Command           *CommandSpec `json:"command,omitempty"`
}

type HTTPCheck struct {
	URL              string `json:"url"`
	ExpectedStatus   int    `json:"expectedStatus"`
	ResponseContains string `json:"responseContains,omitempty"`
	ResponseRegex    string `json:"responseRegex,omitempty"`
	SkipTLSVerify    bool   `json:"skipTLSVerify,omitempty"`
}

type TCPCheck struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type Route struct {
	Hostname   string `json:"hostname"`
	TargetPort int    `json:"targetPort,omitempty"`
	HTTPS      bool   `json:"https,omitempty"`
}

type ServiceStatus string

const (
	StatusStarting  ServiceStatus = "starting"
	StatusRunning   ServiceStatus = "running"
	StatusStopping  ServiceStatus = "stopping"
	StatusStopped   ServiceStatus = "stopped"
	StatusUnhealthy ServiceStatus = "unhealthy"
	StatusCrashed   ServiceStatus = "crashed"
)

type MetricsPoint struct {
	At       time.Time `json:"at"`
	CPU      float64   `json:"cpu"`
	MemoryMB float64   `json:"memoryMb"`
}

type ServiceRuntime struct {
	ServiceID       string         `json:"serviceId"`
	ProjectID       string         `json:"projectId"`
	Status          ServiceStatus  `json:"status"`
	PID             int            `json:"pid,omitempty"`
	ExitCode        *int           `json:"exitCode,omitempty"`
	Signal          string         `json:"signal,omitempty"`
	UptimeSeconds   int64          `json:"uptimeSeconds,omitempty"`
	CPU             float64        `json:"cpu"`
	MemoryMB        float64        `json:"memoryMb"`
	Ports           []int          `json:"ports,omitempty"`
	Health          string         `json:"health,omitempty"`
	RestartCount    int            `json:"restartCount"`
	ContainerID     string         `json:"containerId,omitempty"`
	ContainerName   string         `json:"containerName,omitempty"`
	ContainerState  string         `json:"containerState,omitempty"`
	ContainerHealth string         `json:"containerHealth,omitempty"`
	LastError       string         `json:"lastError,omitempty"`
	LastStarted     *time.Time     `json:"lastStarted,omitempty"`
	LastStopped     *time.Time     `json:"lastStopped,omitempty"`
	History         []MetricsPoint `json:"history,omitempty"`
}

type Snapshot struct {
	Version     int                       `json:"version"`
	GeneratedAt time.Time                 `json:"generatedAt"`
	Connected   bool                      `json:"connected"`
	BackendPID  int                       `json:"backendPid"`
	Settings    Settings                  `json:"settings"`
	Projects    []Project                 `json:"projects"`
	Runtime     map[string]ServiceRuntime `json:"runtime"`
	Routes      []ActiveRoute             `json:"routes,omitempty"`
	Diagnostics []Diagnostic              `json:"diagnostics,omitempty"`
}

type ActiveRoute struct {
	Hostname string `json:"hostname"`
	Target   string `json:"target"`
	HTTPS    bool   `json:"https"`
	Active   bool   `json:"active"`
	Error    string `json:"error,omitempty"`
}

type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func DefaultConfig() Config {
	return Config{
		Version: CurrentConfigVersion,
		Settings: Settings{
			PollIntervalSeconds: 2,
			HistorySamples:      60,
			LogBufferLines:      2000,
			Notifications:       NotificationSettings{Unhealthy: true, Crashed: true, Recovered: true},
			Proxy:               ProxySettings{ListenHost: "127.0.0.1", HTTPPort: 8088, HTTPSPort: 8448},
		},
		Projects: []Project{},
	}
}
