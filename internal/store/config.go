package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"omastack/internal/model"
	"omastack/internal/securefile"
	"omastack/internal/validate"
)

type ConfigStore struct {
	path   string
	mu     sync.RWMutex
	config model.Config
}

func OpenConfig(path string) (*ConfigStore, error) {
	store := &ConfigStore{path: path, config: model.DefaultConfig()}
	if err := store.Reload(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *ConfigStore) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := securefile.ReadRegular(s.path, 8<<20, true)
	if errors.Is(err, os.ErrNotExist) {
		s.config = model.DefaultConfig()
		return s.writeLocked(s.config)
	}
	if err != nil {
		return err
	}
	config, err := DecodeConfig(data)
	if err != nil {
		return err
	}
	s.config = config
	return nil
}

func (s *ConfigStore) Get() model.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s.config)
	var copy model.Config
	_ = json.Unmarshal(data, &copy)
	return copy
}

func (s *ConfigStore) Update(mutator func(*model.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.config
	data, _ := json.Marshal(s.config)
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	if err := mutator(&next); err != nil {
		return err
	}
	if err := validate.Config(next); err != nil {
		return err
	}
	if err := s.writeLocked(next); err != nil {
		return err
	}
	s.config = next
	return nil
}

func (s *ConfigStore) writeLocked(config model.Config) error {
	if err := validate.Config(config); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return AtomicWrite(s.path, data, 0o600)
}

func DecodeConfig(data []byte) (model.Config, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return model.Config{}, fmt.Errorf("parse config header: %w", err)
	}
	if header.Version == 0 {
		var legacy struct {
			Projects []model.Project `json:"projects"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return model.Config{}, err
		}
		migrated := model.DefaultConfig()
		migrated.Projects = legacy.Projects
		if err := validate.Config(migrated); err != nil {
			return model.Config{}, fmt.Errorf("migrate config: %w", err)
		}
		return migrated, nil
	}
	if header.Version != model.CurrentConfigVersion {
		return model.Config{}, fmt.Errorf("unsupported config version %d", header.Version)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config model.Config
	if err := decoder.Decode(&config); err != nil {
		return model.Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := validate.Config(config); err != nil {
		return model.Config{}, fmt.Errorf("validate config: %w", err)
	}
	return config, nil
}
