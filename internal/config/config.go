package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}

func Load() (*Config, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}

	cfg := defaults()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("config: create dir: %w", err)
		}
		if err := writeDefaults(path, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}

	return cfg, nil
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{Port: 19742},
		Database: DatabaseConfig{
			Path: filepath.Join(home, ".local", "share", "financas", "financas.sqlite"),
		},
	}
}

func filePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: get config dir: %w", err)
	}
	return filepath.Join(dir, "financas", "config.toml"), nil
}

func writeDefaults(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("config: write defaults: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
