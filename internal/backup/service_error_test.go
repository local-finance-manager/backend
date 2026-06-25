package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/backup"
	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/shared"
)

// domErrDrive faz EnsureFolder retornar um erro de domínio (ex.: 403 do Drive).
type domErrDrive struct{ *fakeDrive }

func (domErrDrive) EnsureFolder(context.Context, string) (string, error) {
	return "", domainerr.NewForbidden("acesso negado ao Drive")
}

// genErrDrive faz EnsureFolder retornar um erro genérico (não-domínio, não-offline).
type genErrDrive struct{ *fakeDrive }

func (genErrDrive) EnsureFolder(context.Context, string) (string, error) {
	return "", errors.New("falha inesperada")
}

func TestService_Backup_DomainError(t *testing.T) {
	svc := newSvc(t, domErrDrive{newFakeDrive()}, &fakeSnap{content: validSQLiteBytes(t)}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("esperava erro de domínio propagado pelo fail")
	}
}

func TestService_Backup_GenericError(t *testing.T) {
	svc := newSvc(t, genErrDrive{newFakeDrive()}, &fakeSnap{content: validSQLiteBytes(t)}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("esperava erro genérico embrulhado pelo fail")
	}
}

// TestService_Backup_SnapshotError cobre o caminho de erro do snapshot no Backup.
func TestService_Backup_SnapshotError(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{err: errors.New("disco cheio")}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("esperava erro quando o snapshot falha")
	}
}

// TestService_Status_Disabled cobre o ramo desabilitado do Status.
func TestService_Status_Disabled(t *testing.T) {
	svc := backup.NewService(backup.Deps{Enabled: false, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.SyncEnabled {
		t.Error("SyncEnabled deveria ser false quando desabilitado")
	}
}

// TestService_Status_ErrorState cobre o ramo StateError + LastBackupAt preenchido do Status.
func TestService_Status_ErrorState(t *testing.T) {
	msg := "falha anterior"
	store := &fakeStore{set: true, st: backup.BackupState{
		DriveFolderID: "f", LastChecksumSHA256: "abc", LastBackupSize: 100,
		LastBackupAt: time.Now(), LastError: &msg,
	}}
	svc := newSvc(t, newFakeDrive(), &fakeSnap{content: []byte("x")}, store, &fakeRestarter{})
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if string(st.State) != "error" {
		t.Errorf("state: got %q, want error", st.State)
	}
	if st.LastBackupAt == nil {
		t.Error("LastBackupAt deveria estar preenchido")
	}
}

// TestBackupHandler_Offline503 cobre o ramo offline do writeErr (503).
func TestBackupHandler_Offline503(t *testing.T) {
	drive := newFakeDrive()
	drive.offline = true
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: &fakeSnap{content: []byte("x")}, Store: &fakeStore{},
		Restarter: &fakeRestarter{}, DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	r := chi.NewRouter()
	r.Route("/api/backup", backup.Routes(backup.NewHandler(svc)))
	if code := backupReq(t, r, http.MethodPost, "/api/backup", ""); code != http.StatusServiceUnavailable {
		t.Errorf("backup offline: got %d, want 503", code)
	}
}

func TestToken_ErrorPaths(t *testing.T) {
	// LoadToken com json inválido
	bad := filepath.Join(t.TempDir(), "tok.json")
	os.WriteFile(bad, []byte("{nao-json"), 0o600)
	if _, err := backup.LoadToken(bad); err == nil {
		t.Error("LoadToken deveria falhar com json inválido")
	}
	// SaveToken cujo "diretório" pai é na verdade um arquivo → MkdirAll falha
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	if err := backup.SaveToken(filepath.Join(blocker, "tok.json"), &oauth2.Token{}); err == nil {
		t.Error("SaveToken deveria falhar quando o pai é um arquivo")
	}
}

func TestSnapshotter_Error(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("CREATE TABLE t (id INTEGER)")
	// destino inválido (diretório inexistente) → VACUUM INTO falha
	snap := backup.NewSQLiteSnapshotter(db)
	if err := snap.Snapshot(context.Background(), filepath.Join(dir, "sem", "dir", "snap.sqlite")); err == nil {
		t.Error("Snapshot deveria falhar com destino inválido")
	}
}

// TestService_Status_ComputeDirtyError exercita o ramo de erro do computeDirty (snapshot
// falha) — o Status engole o erro e não marca dirty.
func TestService_Status_ComputeDirtyError(t *testing.T) {
	store := &fakeStore{set: true, st: backup.BackupState{LastChecksumSHA256: "abc"}}
	svc := newSvc(t, newFakeDrive(), &fakeSnap{err: errors.New("snap falhou")}, store, &fakeRestarter{})
	if _, err := svc.Status(context.Background()); err != nil {
		t.Fatalf("status não deveria propagar erro do computeDirty: %v", err)
	}
}

// TestSyncOnBoot_OfflineIsNoop cobre o ramo offline (best-effort) do SyncOnBoot.
func TestSyncOnBoot_OfflineIsNoop(t *testing.T) {
	drive := newFakeDrive()
	drive.offline = true
	dir := t.TempDir()
	backup.SyncOnBoot(context.Background(), drive, &fakeStore{},
		config.BackupConfig{FolderName: "F", LatestFilename: "latest.sqlite", DataDir: dir},
		filepath.Join(dir, "db.sqlite"), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestService_BackupBestEffort_Failure(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{err: errors.New("falhou")}, &fakeStore{}, &fakeRestarter{})
	svc.BackupBestEffort(context.Background()) // não deve panicar; loga e retorna
}

func TestService_ListVersions_DriveError(t *testing.T) {
	drive := newFakeDrive()
	drive.failList = true
	store := &fakeStore{set: true, st: backup.BackupState{DriveFolderID: "f"}}
	svc := newSvc(t, drive, &fakeSnap{content: []byte("x")}, store, &fakeRestarter{})
	if _, err := svc.ListVersions(context.Background(), sharedPagination()); err == nil {
		t.Error("ListVersions deveria falhar quando o List do Drive falha")
	}
}

func sharedPagination() shared.Pagination {
	return shared.Pagination{Page: 1, Limit: 10, OrderBy: "created_at", Order: "DESC"}
}

func TestService_Restore_ByVersionID(t *testing.T) {
	drive := newFakeDrive()
	valid := validSQLiteBytes(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: &fakeSnap{content: valid}, Store: &fakeStore{},
		Restarter: &fakeRestarter{}, DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if _, err := svc.Backup(context.Background()); err != nil { // cria latest + versão datada
		t.Fatalf("backup: %v", err)
	}
	vers, err := svc.ListVersions(context.Background(), sharedPagination())
	if err != nil || len(vers.Data) == 0 {
		t.Fatalf("list versions: err=%v len=%d", err, len(vers.Data))
	}
	res, err := svc.Restore(context.Background(), vers.Data[0].FileID, true)
	if err != nil {
		t.Fatalf("restore by id: %v", err)
	}
	if !res.RestartRequired {
		t.Error("esperava RestartRequired")
	}
}

// dlFailDrive faz só o Download falhar (resolveTarget via FindByName ainda funciona).
type dlFailDrive struct{ *fakeDrive }

func (dlFailDrive) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("download falhou")
}

func newRestoreSvc(t *testing.T, drive backup.DriveClient, snap backup.Snapshotter) *backup.Service {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)
	return backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: snap, Store: &fakeStore{}, Restarter: &fakeRestarter{},
		DBPath: dbPath, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestService_Restore_SafetySnapshotError(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader(validSQLiteBytes(t)))
	svc := newRestoreSvc(t, drive, &fakeSnap{err: errors.New("snap falhou")})
	if _, err := svc.Restore(context.Background(), "", true); err == nil {
		t.Error("esperava erro no safety snapshot")
	}
}

func TestService_Restore_IntegrityFails(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader([]byte("não é sqlite")))
	svc := newRestoreSvc(t, drive, &fakeSnap{content: validSQLiteBytes(t)})
	if _, err := svc.Restore(context.Background(), "", true); err == nil {
		t.Error("esperava falha no integrity_check")
	}
}

func TestService_Restore_DownloadError(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader(validSQLiteBytes(t)))
	svc := newRestoreSvc(t, dlFailDrive{drive}, &fakeSnap{content: validSQLiteBytes(t)})
	if _, err := svc.Restore(context.Background(), "", true); err == nil {
		t.Error("esperava erro no download")
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// findErrDrive faz FindByName falhar (EnsureFolder/embedded continuam ok).
type findErrDrive struct{ *fakeDrive }

func (findErrDrive) FindByName(context.Context, string, string) (*backup.DriveFile, error) {
	return nil, errors.New("find falhou")
}

func syncBootDB(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("old-local"), 0o600)
	return dir, dbPath
}

func TestSyncOnBoot_EnsureFolderError(t *testing.T) {
	dir, dbPath := syncBootDB(t)
	backup.SyncOnBoot(context.Background(), domErrDrive{newFakeDrive()}, &fakeStore{},
		bootCfg(dir), dbPath, discardLog())
}

func TestSyncOnBoot_FindError(t *testing.T) {
	dir, dbPath := syncBootDB(t)
	backup.SyncOnBoot(context.Background(), findErrDrive{newFakeDrive()}, &fakeStore{},
		bootCfg(dir), dbPath, discardLog())
}

func TestSyncOnBoot_DownloadError(t *testing.T) {
	dir, dbPath := syncBootDB(t)
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader(validSQLiteBytes(t))) // remoto mais novo
	backup.SyncOnBoot(context.Background(), dlFailDrive{drive}, &fakeStore{},
		bootCfg(dir), dbPath, discardLog())
	// como o download falha, o local é preservado
	if b, _ := os.ReadFile(dbPath); string(b) != "old-local" {
		t.Error("local não deveria ter sido alterado quando o download falha")
	}
}

func TestSyncOnBoot_IntegrityFails(t *testing.T) {
	dir, dbPath := syncBootDB(t)
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader([]byte("não é sqlite"))) // remoto inválido
	backup.SyncOnBoot(context.Background(), drive, &fakeStore{}, bootCfg(dir), dbPath, discardLog())
	if b, _ := os.ReadFile(dbPath); string(b) != "old-local" {
		t.Error("local deveria ser mantido quando a integridade do remoto falha")
	}
}

// failDrive falha numa operação específica (op), delegando o resto ao fakeDrive.
type failDrive struct {
	*fakeDrive
	op string
}

func (d failDrive) UploadNew(ctx context.Context, folderID, name string, r io.Reader) (backup.DriveFile, error) {
	if d.op == "upload" {
		return backup.DriveFile{}, errors.New("upload falhou")
	}
	return d.fakeDrive.UploadNew(ctx, folderID, name, r)
}

func (d failDrive) UpdateContents(ctx context.Context, id string, r io.Reader) (backup.DriveFile, error) {
	if d.op == "update" {
		return backup.DriveFile{}, errors.New("update falhou")
	}
	return d.fakeDrive.UpdateContents(ctx, id, r)
}

func (d failDrive) Delete(ctx context.Context, id string) error {
	if d.op == "delete" {
		return errors.New("delete falhou")
	}
	return d.fakeDrive.Delete(ctx, id)
}

func TestService_Backup_UploadDatedError(t *testing.T) {
	svc := newSvc(t, failDrive{newFakeDrive(), "upload"}, &fakeSnap{content: validSQLiteBytes(t)}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("esperava erro no upload da cópia datada")
	}
}

func TestService_Backup_UploadLatestError(t *testing.T) {
	// state com LatestFileID → uploadLatest usa UpdateContents (que falha)
	store := &fakeStore{set: true, st: backup.BackupState{DriveFolderID: "f", LatestFileID: "latest-id"}}
	svc := newSvc(t, failDrive{newFakeDrive(), "update"}, &fakeSnap{content: validSQLiteBytes(t)}, store, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("esperava erro no upload da latest")
	}
}

func TestService_Backup_PruneDeleteError(t *testing.T) {
	// retenção 1 + várias versões antigas → prune tenta Delete (que falha, mas é best-effort)
	old := []backup.Version{
		{FileID: "v1", Name: "a", Size: 1}, {FileID: "v2", Name: "b", Size: 1},
	}
	store := &fakeStore{set: true, st: backup.BackupState{DriveFolderID: "f", Versions: old}}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("current"), 0o600)
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 1, DataDir: dir,
		},
		Drive: failDrive{newFakeDrive(), "delete"}, Snap: &fakeSnap{content: validSQLiteBytes(t)},
		Store: store, Restarter: &fakeRestarter{}, DBPath: dbPath, Log: discardLog(),
	})
	if _, err := svc.Backup(context.Background()); err != nil {
		t.Fatalf("backup com prune-delete falho não deveria propagar erro: %v", err)
	}
}

func TestService_Restore_FindLatestError(t *testing.T) {
	svc := newRestoreSvc(t, findErrDrive{newFakeDrive()}, &fakeSnap{content: validSQLiteBytes(t)})
	if _, err := svc.Restore(context.Background(), "", true); err == nil {
		t.Error("esperava erro ao resolver a latest")
	}
}

func TestResolveToken_Branches(t *testing.T) {
	dir := t.TempDir()
	// token sem refresh/access + env definido → usa o env
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(empty, []byte(`{}`), 0o600)
	tok, err := backup.ResolveToken(empty, "refresh-do-env")
	if err != nil || tok == nil || tok.RefreshToken != "refresh-do-env" {
		t.Errorf("ResolveToken env: tok=%+v err=%v", tok, err)
	}
	// sem arquivo e sem env → nil, nil
	tok, err = backup.ResolveToken(filepath.Join(dir, "missing.json"), "")
	if err != nil || tok != nil {
		t.Errorf("ResolveToken vazio: tok=%+v err=%v", tok, err)
	}
}

func TestService_Restore_ListError(t *testing.T) {
	drive := newFakeDrive()
	drive.failList = true
	svc := newRestoreSvc(t, drive, &fakeSnap{content: validSQLiteBytes(t)})
	if _, err := svc.Restore(context.Background(), "alguma-versao", true); err == nil {
		t.Error("esperava erro ao listar versões para restore")
	}
}

// errStore faz Load/Save falharem (cobre os ramos de erro de estado em várias funções).
type errStore struct{ loadErr, saveErr error }

func (s errStore) Load() (backup.BackupState, error) { return backup.BackupState{}, s.loadErr }
func (s errStore) Save(backup.BackupState) error     { return s.saveErr }

func TestService_StoreLoadErrors(t *testing.T) {
	boom := errors.New("load falhou")
	mk := func() *backup.Service {
		return newSvc(t, newFakeDrive(), &fakeSnap{content: validSQLiteBytes(t)}, errStore{loadErr: boom}, &fakeRestarter{})
	}
	if _, err := mk().Backup(context.Background()); err == nil {
		t.Error("Backup deveria falhar quando store.Load falha")
	}
	if _, err := mk().Status(context.Background()); err == nil {
		t.Error("Status deveria falhar quando store.Load falha")
	}
	if _, err := mk().ListVersions(context.Background(), sharedPagination()); err == nil {
		t.Error("ListVersions deveria falhar quando store.Load falha (via folderID)")
	}
	if _, err := mk().Restore(context.Background(), "", true); err == nil {
		t.Error("Restore deveria falhar quando store.Load falha (via folderID)")
	}
}

// TestBackupHandler_StatusError cobre o ramo de erro do handler Status.
func TestBackupHandler_StatusError(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{content: []byte("x")}, errStore{loadErr: errors.New("boom")}, &fakeRestarter{})
	r := chi.NewRouter()
	r.Route("/api/backup", backup.Routes(backup.NewHandler(svc)))
	if code := backupReq(t, r, http.MethodGet, "/api/backup/status", ""); code < 400 {
		t.Errorf("status com erro de store: got %d, want >=400", code)
	}
}

func TestService_Backup_SaveError(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{content: validSQLiteBytes(t)}, errStore{saveErr: errors.New("save falhou")}, &fakeRestarter{})
	if _, err := svc.Backup(context.Background()); err == nil {
		t.Error("Backup deveria falhar quando o store.Save final falha")
	}
}

func TestSyncOnBoot_SaveError(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytesReader(validSQLiteBytes(t)))
	dir, dbPath := syncBootDB(t)
	// Save falha após o swap → apenas loga (best-effort), sem panic
	backup.SyncOnBoot(context.Background(), drive, errStore{saveErr: errors.New("x")},
		bootCfg(dir), dbPath, discardLog())
}
