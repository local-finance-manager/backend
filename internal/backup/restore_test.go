package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/local-finance-manager/backend/internal/backup"
)

func TestApplyPendingRestore_NoPending(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	if err := os.WriteFile(dbPath, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	applied, err := backup.ApplyPendingRestore(dbPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Error("não havia pending — não deveria aplicar")
	}
	data, _ := os.ReadFile(dbPath)
	if string(data) != "current" {
		t.Errorf("banco não deveria mudar, got %q", string(data))
	}
}

func TestApplyPendingRestore_SwapsAndCleansWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("old-db"), 0o600)
	os.WriteFile(dbPath+"-wal", []byte("stale-wal"), 0o600)
	os.WriteFile(dbPath+"-shm", []byte("stale-shm"), 0o600)
	os.WriteFile(backup.PendingRestorePath(dbPath), []byte("restored-db"), 0o600)

	applied, err := backup.ApplyPendingRestore(dbPath)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied {
		t.Fatal("deveria ter aplicado o pending")
	}
	data, _ := os.ReadFile(dbPath)
	if string(data) != "restored-db" {
		t.Errorf("banco deveria ser o restaurado, got %q", string(data))
	}
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Error("-wal órfão deveria ter sido removido")
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Error("-shm órfão deveria ter sido removido")
	}
	if _, err := os.Stat(backup.PendingRestorePath(dbPath)); !os.IsNotExist(err) {
		t.Error("pending deveria ter sido consumido (renomeado)")
	}
}
