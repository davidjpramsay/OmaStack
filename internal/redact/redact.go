package redact

import (
	"encoding/json"
	"sort"
	"strings"

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
	result := value
	for _, secret := range secrets {
		if secret != "" {
			result = strings.ReplaceAll(result, secret, Mask)
		}
	}
	return result
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
