package config

import (
	"errors"
	"os"

	"github.com/oladapodev/claudecode-go/internal/domain"
	"github.com/oladapodev/claudecode-go/internal/paths"
	"gopkg.in/yaml.v3"
)

type Store struct {
	Paths paths.Paths
}

func NewStore(p paths.Paths) Store {
	return Store{Paths: p}
}

func (s Store) Load() (domain.Config, error) {
	if err := s.Paths.Ensure(); err != nil {
		return domain.Config{}, err
	}

	raw, err := os.ReadFile(s.Paths.ConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := domain.DefaultConfig()
			if err := s.Save(cfg); err != nil {
				return domain.Config{}, err
			}
			return cfg, nil
		}
		return domain.Config{}, err
	}

	cfg := domain.DefaultConfig()
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return domain.Config{}, err
	}

	if cfg.Profiles == nil {
		cfg.Profiles = domain.DefaultConfig().Profiles
	}

	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}

	return cfg, nil
}

func (s Store) Save(cfg domain.Config) error {
	if err := s.Paths.Ensure(); err != nil {
		return err
	}

	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(s.Paths.ConfigFile, raw, 0o600)
}
