package redact

import (
	"encoding/json"
	"sort"
	"strings"

	"omastack/internal/bounded"
	"omastack/internal/model"
)

const Mask = "••••••••"

func Secrets(config model.Config) []string {
	seen := map[string]bool{}
	for _, project := range config.Projects {
		for _, service := range project.Services {
			for _, value := range service.Environment {
				if value.Secret && value.Value != "" {
					seen[value.Value] = true
				}
			}
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

func Text(value string, secrets []string) string {
	// Match only original input, never the masks emitted by earlier matches.
	seen := make(map[string]bool)
	patterns := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" && !seen[secret] {
			seen[secret] = true
			patterns = append(patterns, secret)
		}
	}
	if len(patterns) == 0 {
		return value
	}
	sort.Slice(patterns, func(i, j int) bool { return len(patterns[i]) > len(patterns[j]) })
	pairs := make([]string, 0, 2*len(patterns))
	for _, pattern := range patterns {
		pairs = append(pairs, pattern, Mask)
	}
	output := bounded.NewBuffer(1 << 20)
	_, _ = strings.NewReplacer(pairs...).WriteString(output, value)
	if output.Truncated {
		return "[OmaStack: message omitted because redacted output exceeds 1 MiB]"
	}
	return output.String()
}

func Config(config model.Config) model.Config {
	data, _ := json.Marshal(config)
	var result model.Config
	_ = json.Unmarshal(data, &result)
	for pi := range result.Projects {
		for si := range result.Projects[pi].Services {
			for name, value := range result.Projects[pi].Services[si].Environment {
				if value.Secret {
					value.Value = Mask
					result.Projects[pi].Services[si].Environment[name] = value
				}
			}
		}
	}
	return result
}
