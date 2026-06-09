package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the application.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int
}

// DatabaseConfig holds SQLite database settings.
type DatabaseConfig struct {
	// Path is the absolute path to the SQLite file on disk.
	Path string
}

// Addr returns the TCP address the server should listen on.
func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}

// Load reads configuration from environment variables, falling back to
// defaults. A .env file in the working directory is loaded if present.
func Load() (*Config, error) {
	// Ignore error: .env is optional; real env vars take precedence anyway.
	_ = godotenv.Load()

	cfg := defaults()

	if raw := os.Getenv("SERVER_PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config: SERVER_PORT must be an integer: %w", err)
		}
		cfg.Server.Port = p
	}

	if path := os.Getenv("DB_PATH"); path != "" {
		cfg.Database.Path = path
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0o700); err != nil {
		return nil, fmt.Errorf("config: create database directory: %w", err)
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
