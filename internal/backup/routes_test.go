package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/backup"
	"github.com/local-finance-manager/backend/internal/config"
)

func backupReq(t *testing.T, router http.Handler, method, path, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestBackupRoutes_Enabled(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytes.NewReader(validSQLiteBytes(t)))
	// snapshot precisa ser um sqlite válido: o POST /backup faz upload dele como "latest",
	// e o restore confirmado baixa o "latest" e roda integrity_check.
	snap := &fakeSnap{content: validSQLiteBytes(t)}
	store := &fakeStore{}
	rest := &fakeRestarter{}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)

	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite",
			Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: snap, Store: store, Restarter: rest,
		DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	r := chi.NewRouter()
	r.Route("/api/backup", backup.Routes(backup.NewHandler(svc)))

	if code := backupReq(t, r, http.MethodPost, "/api/backup", ""); code != http.StatusOK {
		t.Errorf("backup: got %d, want 200", code)
	}
	if code := backupReq(t, r, http.MethodGet, "/api/backup/status", ""); code != http.StatusOK {
		t.Errorf("status: got %d, want 200", code)
	}
	if code := backupReq(t, r, http.MethodGet, "/api/backup/versions", ""); code != http.StatusOK {
		t.Errorf("versions: got %d, want 200", code)
	}
	// Restore sem confirmação → erro de domínio (>= 400)
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore", `{"confirm":false}`); code < 400 {
		t.Errorf("restore not confirmed: got %d, want >=400", code)
	}
	// Restore bad json → 400
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore", `{`); code != http.StatusBadRequest {
		t.Errorf("restore bad json: got %d, want 400", code)
	}
	// Restore com version_id inexistente → erro de domínio (>=400, não 503)
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore", `{"version_id":"nao-existe","confirm":true}`); code < 400 || code == http.StatusServiceUnavailable {
		t.Errorf("restore version inexistente: got %d", code)
	}
	// Restore confirmado → 200 e o handler chama Restart
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore", `{"confirm":true}`); code != http.StatusOK {
		t.Errorf("restore confirmed: got %d, want 200", code)
	}
	if rest.called != 1 {
		t.Errorf("Restart deveria ter sido chamado pelo handler, called=%d", rest.called)
	}
}

func TestBackupRoutes_Disabled(t *testing.T) {
	svc := backup.NewService(backup.Deps{Enabled: false, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	r := chi.NewRouter()
	r.Route("/api/backup", backup.Routes(backup.NewHandler(svc)))

	// Backup desabilitado → ErrBackupDisabled (Conflict 409)
	if code := backupReq(t, r, http.MethodPost, "/api/backup", ""); code != http.StatusConflict {
		t.Errorf("backup disabled: got %d, want 409", code)
	}
	if code := backupReq(t, r, http.MethodGet, "/api/backup/versions", ""); code != http.StatusConflict {
		t.Errorf("versions disabled: got %d, want 409", code)
	}
}

func TestBackupBestEffort(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytes.NewReader(validSQLiteBytes(t)))
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: &fakeSnap{content: validSQLiteBytes(t)}, Store: &fakeStore{},
		Restarter: &fakeRestarter{}, DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	svc.BackupBestEffort(context.Background()) // caminho de sucesso (best-effort, sem retorno)

	// Desabilitado → retorna cedo (sem panic/efeito).
	off := backup.NewService(backup.Deps{Enabled: false, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	off.BackupBestEffort(context.Background())
}

// TestSnapshotter cobre snapshot.go: VACUUM INTO gera um arquivo válido.
func TestSnapshotter(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "src.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY); INSERT INTO t (id) VALUES (1)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	snap := backup.NewSQLiteSnapshotter(db)
	dest := filepath.Join(dir, "snap.sqlite")
	if err := snap.Snapshot(context.Background(), dest); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Errorf("snapshot deveria gerar arquivo não-vazio: info=%v err=%v", info, err)
	}
}
