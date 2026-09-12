package daemon

import (
	"encoding/json"
	"omastack/internal/model"
)

// Pointer fields distinguish omitted values from explicit false/zero. Legacy
// settings.update remains a full replacement; the UI uses atomic field patches.
type settingsPatch struct {
	PollIntervalSeconds *int `json:"pollIntervalSeconds"`
	HistorySamples      *int `json:"historySamples"`
	LogBufferLines      *int `json:"logBufferLines"`
	Notifications       *struct {
		Crashed   *bool `json:"crashed"`
		Unhealthy *bool `json:"unhealthy"`
		Recovered *bool `json:"recovered"`
	} `json:"notifications"`
	Proxy *struct {
		Enabled    *bool   `json:"enabled"`
		ListenHost *string `json:"listenHost"`
		HTTPPort   *int    `json:"httpPort"`
		HTTPSPort  *int    `json:"httpsPort"`
	} `json:"proxy"`
}

func assign[T any](destination *T, value *T) {
	if value != nil {
		*destination = *value
	}
}

func (d *Daemon) patchSettings(raw json.RawMessage) (any, error) {
	var patch settingsPatch
	if err := decodeParams(raw, &patch); err != nil {
		return nil, err
	}
	err := d.store.Update(func(c *model.Config) error {
		s := &c.Settings
		assign(&s.PollIntervalSeconds, patch.PollIntervalSeconds)
		assign(&s.HistorySamples, patch.HistorySamples)
		assign(&s.LogBufferLines, patch.LogBufferLines)
		if n := patch.Notifications; n != nil {
			assign(&s.Notifications.Crashed, n.Crashed)
			assign(&s.Notifications.Unhealthy, n.Unhealthy)
			assign(&s.Notifications.Recovered, n.Recovered)
		}
		if p := patch.Proxy; p != nil {
			assign(&s.Proxy.Enabled, p.Enabled)
			assign(&s.Proxy.ListenHost, p.ListenHost)
			assign(&s.Proxy.HTTPPort, p.HTTPPort)
			assign(&s.Proxy.HTTPSPort, p.HTTPSPort)
		}
		return nil
	})
	return d.store.Get().Settings, err
}
