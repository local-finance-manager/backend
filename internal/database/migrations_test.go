package database

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestRunMigrations_AppliesCleanly garante que toda a cadeia .up.sql aplica em um
// banco novo sem erro (inclui a 0014 de caixinhas) e que o schema/seed esperado existe.
func TestRunMigrations_AppliesCleanly(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable fk: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// Rodar de novo é idempotente (schema_migrations impede reaplicar).
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations (2nd): %v", err)
	}

	// Tabela caixinhas existe.
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='caixinhas'",
	).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tabela caixinhas ausente: n=%d err=%v", n, err)
	}

	// Coluna caixinha_id em transactions.
	if _, err := db.Exec("SELECT caixinha_id FROM transactions LIMIT 0"); err != nil {
		t.Fatalf("coluna transactions.caixinha_id ausente: %v", err)
	}

	// Coluna caixinha_direction em subcategories + seed das subcategorias de sistema.
	var dir string
	if err := db.QueryRow(
		"SELECT caixinha_direction FROM subcategories WHERE id='sub-caixinha-aporte'",
	).Scan(&dir); err != nil || dir != "aporte" {
		t.Fatalf("seed sub-caixinha-aporte inválido: dir=%q err=%v", dir, err)
	}
	if err := db.QueryRow(
		"SELECT caixinha_direction FROM subcategories WHERE id='sub-caixinha-resgate'",
	).Scan(&dir); err != nil || dir != "resgate" {
		t.Fatalf("seed sub-caixinha-resgate inválido: dir=%q err=%v", dir, err)
	}
}
