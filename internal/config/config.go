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
	Backup   BackupConfig
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

// BackupConfig holds Google Drive backup settings. Secrets must never be logged.
type BackupConfig struct {
	Enabled          bool
	ClientID         string
	ClientSecret     string
	RefreshToken     string // fallback; preferimos o token.json (TokenPath)
	FolderName       string
	LatestFilename   string
	Retention        int
	AutosaveInterval int    // minutos; 0 = desabilitado
	DataDir          string // dir do banco — sidecar, snapshots, safety backups
	TokenPath        string // ~/.config/financas/token.json (volume montado)

	// Tier de snapshot local (independe do Drive) — RF-BKP-18.
	LocalSnapshotEnabled   bool
	LocalSnapshotRetention int // nº de snapshots locais datados a manter; 0 = desliga o tier
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

	loadBackup(&cfg.Backup, cfg.Database.Path)

	return cfg, nil
}

// loadBackup reads DRIVE_*/BACKUP_* env vars into the BackupConfig.
// Never returns an error: missing/invalid config just leaves the feature disabled.
func loadBackup(b *BackupConfig, dbPath string) {
	b.Enabled = os.Getenv("DRIVE_SYNC_ENABLED") == "true"
	b.ClientID = os.Getenv("DRIVE_CLIENT_ID")
	b.ClientSecret = os.Getenv("DRIVE_CLIENT_SECRET")
	b.RefreshToken = os.Getenv("DRIVE_REFRESH_TOKEN")
	b.FolderName = getenvDefault("DRIVE_FOLDER_NAME", "Financas")
	b.LatestFilename = getenvDefault("DRIVE_LATEST_FILENAME", "financas-latest.sqlite")
	b.Retention = atoiDefault(os.Getenv("DRIVE_BACKUP_RETENTION"), 5)           // RF-BKP-15 (era 30)
	b.AutosaveInterval = atoiDefault(os.Getenv("BACKUP_AUTOSAVE_INTERVAL"), 15) // RF-BKP-17 (era 0)
	b.DataDir = filepath.Dir(dbPath)

	// Tier de snapshot local — ligado por padrão, retenção 5 (RF-BKP-18 / RNF-BKP2-04).
	b.LocalSnapshotEnabled = getenvDefault("LOCAL_SNAPSHOT_ENABLED", "true") == "true"
	b.LocalSnapshotRetention = atoiDefault(os.Getenv("LOCAL_SNAPSHOT_RETENTION"), 5)

	if path := os.Getenv("DRIVE_TOKEN_PATH"); path != "" {
		b.TokenPath = path
	} else {
		home, _ := os.UserHomeDir()
		b.TokenPath = filepath.Join(home, ".config", "financas", "token.json")
	}
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoiDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
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
