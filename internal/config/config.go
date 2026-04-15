package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultDir            = ".gtask"
	defaultDBName         = "gtask.db"
	defaultConfigName     = "config.json"
	defaultGoogleTasklist = "My Tasks"
)

type Config struct {
	DataDir             string `json:"-"`
	DBPath              string `json:"db_path"`
	DefaultGoogleListID string `json:"default_google_list_id,omitempty"`
	DefaultGoogleList   string `json:"default_google_list_title,omitempty"`
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	dataDir := filepath.Join(home, defaultDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create data dir: %w", err)
	}

	cfg := Config{
		DataDir:           dataDir,
		DBPath:            filepath.Join(dataDir, defaultDBName),
		DefaultGoogleList: defaultGoogleTasklist,
	}

	cfgPath := filepath.Join(dataDir, defaultConfigName)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := save(cfgPath, cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.DataDir = dataDir
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(dataDir, defaultDBName)
	}
	if cfg.DefaultGoogleList == "" {
		cfg.DefaultGoogleList = defaultGoogleTasklist
	}
	return cfg, nil
}

func Save(cfg Config) error {
	cfgPath := filepath.Join(cfg.DataDir, defaultConfigName)
	return save(cfgPath, cfg)
}

func save(path string, cfg Config) error {
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
