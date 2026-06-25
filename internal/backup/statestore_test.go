package backup_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/local-finance-manager/backend/internal/backup"
)

func TestFileStateStore_LoadMissingReturnsZero(t *testing.T) {
	store := backup.NewFileStateStore(filepath.Join(t.TempDir(), "backup-state.json"))
	st, err := store.Load()
	if err != nil {
		t.Fatalf("arquivo ausente não deveria ser erro: %v", err)
	}
	if st.LatestFileID != "" || len(st.Versions) != 0 {
		t.Errorf("estado zero esperado, got %+v", st)
	}
}

func TestFileStateStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup-state.json")
	store := backup.NewFileStateStore(path)

	want := backup.BackupState{
		DriveFolderID:      "folder-1",
		LatestFileID:       "latest-1",
		LastBackupAt:       time.Date(2026, 6, 12, 14, 30, 0, 0, time.UTC),
		LastChecksumSHA256: "9f86d081",
		LastBackupSize:     245760,
		Versions: []backup.Version{
			{FileID: "v1", Name: "financas-2026-06-12T14-30-00Z.sqlite", CreatedAt: time.Date(2026, 6, 12, 14, 30, 0, 0, time.UTC), Size: 245760},
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.DriveFolderID != want.DriveFolderID || got.LatestFileID != want.LatestFileID ||
		got.LastChecksumSHA256 != want.LastChecksumSHA256 || got.LastBackupSize != want.LastBackupSize {
		t.Errorf("round-trip divergente: got %+v", got)
	}
	if !got.LastBackupAt.Equal(want.LastBackupAt) {
		t.Errorf("lastBackupAt: got %v, want %v", got.LastBackupAt, want.LastBackupAt)
	}
	if len(got.Versions) != 1 || got.Versions[0].FileID != "v1" {
		t.Errorf("versions: got %+v", got.Versions)
	}
}

func TestFileStateStore_Overwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup-state.json")
	store := backup.NewFileStateStore(path)

	store.Save(backup.BackupState{LatestFileID: "first"})
	if err := store.Save(backup.BackupState{LatestFileID: "second"}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ := store.Load()
	if got.LatestFileID != "second" {
		t.Errorf("got %q, want second", got.LatestFileID)
	}
}

func TestFileStateStore_ErrorPaths(t *testing.T) {
	// Save com diretório inexistente → erro ao escrever o tmp
	bad := backup.NewFileStateStore(filepath.Join(t.TempDir(), "sem", "dir", "state.json"))
	if err := bad.Save(backup.BackupState{}); err == nil {
		t.Error("Save deveria falhar com diretório inexistente")
	}
	// Load com json malformado → erro
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte("{nao-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.NewFileStateStore(p).Load(); err == nil {
		t.Error("Load deveria falhar com json malformado")
	}
}
