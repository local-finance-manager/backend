package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExpandHome cobre a causa-raiz da perda de dados: um DB_PATH com "~" precisa
// virar caminho absoluto sob o home; sem expansão, "~/..." é relativo ao working
// dir e o banco cai numa camada efêmera do container.
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"til sozinho", "~", home},
		{"til com subpath", "~/.local/share/financas/financas.sqlite",
			filepath.Join(home, ".local/share/financas/financas.sqlite")},
		{"absoluto inalterado", "/root/.local/share/financas/financas.sqlite",
			"/root/.local/share/financas/financas.sqlite"},
		{"relativo sem til inalterado", "data/financas.sqlite", "data/financas.sqlite"},
		{"til no meio não expande", "/opt/~/db.sqlite", "/opt/~/db.sqlite"},
		{"vazio", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandHome(tc.in); got != tc.want {
				t.Errorf("expandHome(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestLoadExpandsDBPath garante que DB_PATH com "~" é expandido no Load (não fica
// um caminho relativo que gravaria o banco no lugar errado).
func TestLoadExpandsDBPath(t *testing.T) {
	t.Setenv("DB_PATH", "~/.local/share/financas/financas.sqlite")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if filepath.IsAbs(cfg.Database.Path) == false {
		t.Fatalf("Database.Path deveria ser absoluto, got %q", cfg.Database.Path)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local/share/financas/financas.sqlite")
	if cfg.Database.Path != want {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, want)
	}
}
