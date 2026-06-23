package backup

import (
	"context"
	"database/sql"
	"fmt"
)

var _ Snapshotter = (*SQLiteSnapshotter)(nil)

// SQLiteSnapshotter gera um snapshot íntegro via VACUUM INTO, seguro com o banco
// em modo WAL e em uso (RF-BKP-02). Segura o *sql.DB compartilhado.
type SQLiteSnapshotter struct{ db *sql.DB }

// NewSQLiteSnapshotter cria um SQLiteSnapshotter.
func NewSQLiteSnapshotter(db *sql.DB) *SQLiteSnapshotter {
	return &SQLiteSnapshotter{db: db}
}

// Snapshot escreve um snapshot consistente do banco em destPath. O destino NÃO pode
// existir (exigência do VACUUM INTO) — o chamador garante um nome único e a limpeza.
func (s *SQLiteSnapshotter) Snapshot(ctx context.Context, destPath string) error {
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("backup snapshot: vacuum into: %w", err)
	}
	return nil
}
