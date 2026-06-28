package backup_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/backup"
	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeFile struct {
	df      backup.DriveFile
	content []byte
}

type fakeDrive struct {
	mu       sync.Mutex
	folderID string
	files    map[string]fakeFile // id → file
	seq      int
	offline  bool
	badMD5   bool
	failList bool
}

func newFakeDrive() *fakeDrive {
	return &fakeDrive{folderID: "folder-1", files: map[string]fakeFile{}}
}

func offlineErr() error {
	// classificado como offline por isOffline (contém "dial tcp"/"connection refused")
	return &netOpError{}
}

type netOpError struct{}

func (e *netOpError) Error() string   { return "dial tcp 142.0.0.1:443: connect: connection refused" }
func (e *netOpError) Timeout() bool   { return false }
func (e *netOpError) Temporary() bool { return true }

func (d *fakeDrive) EnsureFolder(_ context.Context, _ string) (string, error) {
	if d.offline {
		return "", offlineErr()
	}
	return d.folderID, nil
}

func (d *fakeDrive) put(name string, r io.Reader) (backup.DriveFile, error) {
	content, _ := io.ReadAll(r)
	sum := md5.Sum(content)
	md5hex := hex.EncodeToString(sum[:])
	if d.badMD5 {
		md5hex = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	d.seq++
	id := name + "-" + itoa(d.seq)
	df := backup.DriveFile{
		ID: id, Name: name, Size: int64(len(content)), MD5Checksum: md5hex,
		ModifiedTime: time.Now().UTC(), CreatedTime: time.Now().UTC(),
	}
	d.files[id] = fakeFile{df: df, content: content}
	return df, nil
}

func (d *fakeDrive) UploadNew(_ context.Context, _, name string, r io.Reader) (backup.DriveFile, error) {
	if d.offline {
		return backup.DriveFile{}, offlineErr()
	}
	return d.put(name, r)
}

func (d *fakeDrive) UpdateContents(_ context.Context, fileID string, r io.Reader) (backup.DriveFile, error) {
	if d.offline {
		return backup.DriveFile{}, offlineErr()
	}
	content, _ := io.ReadAll(r)
	sum := md5.Sum(content)
	md5hex := hex.EncodeToString(sum[:])
	if d.badMD5 {
		md5hex = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	existing, ok := d.files[fileID]
	if !ok {
		return backup.DriveFile{}, offlineErr()
	}
	existing.content = content
	existing.df.Size = int64(len(content))
	existing.df.MD5Checksum = md5hex
	existing.df.ModifiedTime = time.Now().UTC()
	d.files[fileID] = existing
	return existing.df, nil
}

func (d *fakeDrive) FindByName(_ context.Context, _, name string) (*backup.DriveFile, error) {
	if d.offline {
		return nil, offlineErr()
	}
	for _, f := range d.files {
		if f.df.Name == name {
			df := f.df
			return &df, nil
		}
	}
	return nil, nil
}

func (d *fakeDrive) List(_ context.Context, _ string) ([]backup.DriveFile, error) {
	if d.offline || d.failList {
		return nil, offlineErr()
	}
	out := []backup.DriveFile{}
	for _, f := range d.files {
		out = append(out, f.df)
	}
	return out, nil
}

func (d *fakeDrive) Download(_ context.Context, fileID string) (io.ReadCloser, error) {
	if d.offline {
		return nil, offlineErr()
	}
	f, ok := d.files[fileID]
	if !ok {
		return nil, offlineErr()
	}
	return io.NopCloser(bytes.NewReader(f.content)), nil
}

func (d *fakeDrive) Delete(_ context.Context, fileID string) error {
	if d.offline {
		return offlineErr()
	}
	delete(d.files, fileID)
	return nil
}

// fakeSnap escreve um conteúdo controlado no destino (controla o checksum).
type fakeSnap struct {
	content []byte
	err     error
}

func (s *fakeSnap) Snapshot(_ context.Context, dest string) error {
	if s.err != nil {
		return s.err
	}
	return os.WriteFile(dest, s.content, 0o600)
}

type fakeStore struct {
	st  backup.BackupState
	set bool
}

func (s *fakeStore) Load() (backup.BackupState, error) {
	if !s.set {
		return backup.BackupState{}, nil
	}
	return s.st, nil
}
func (s *fakeStore) Save(st backup.BackupState) error {
	s.st = st
	s.set = true
	return nil
}

type fakeRestarter struct{ called int }

func (r *fakeRestarter) Restart() { r.called++ }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ─── helpers ────────────────────────────────────────────────────────────────

func newSvc(t *testing.T, drive backup.DriveClient, snap backup.Snapshotter, store backup.StateStore, rest backup.Restarter) *backup.Service {
	t.Helper()
	dir := t.TempDir()
	return backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite",
			Retention: 30, DataDir: dir,
		},
		Drive: drive, Snap: snap, Store: store, Restarter: rest,
		DBPath: filepath.Join(dir, "financas.sqlite"),
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func validSQLiteBytes(t *testing.T) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "valid.sqlite")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ─── Disabled ───────────────────────────────────────────────────────────────

func TestService_Disabled(t *testing.T) {
	svc := backup.NewService(backup.Deps{
		Enabled: false,
		Cfg:     config.BackupConfig{DataDir: t.TempDir()},
		Store:   &fakeStore{},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx := context.Background()

	if _, err := svc.Backup(ctx); err != backup.ErrBackupDisabled {
		t.Errorf("Backup: got %v, want ErrBackupDisabled", err)
	}
	if _, err := svc.Restore(ctx, "", true); err != backup.ErrBackupDisabled {
		t.Errorf("Restore: got %v, want ErrBackupDisabled", err)
	}
	if _, err := svc.ListVersions(ctx, shared.DefaultPagination()); err != backup.ErrBackupDisabled {
		t.Errorf("ListVersions: got %v, want ErrBackupDisabled", err)
	}
	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.SyncEnabled {
		t.Error("Status.SyncEnabled deveria ser false quando desabilitado")
	}
}

// ─── Backup ─────────────────────────────────────────────────────────────────

func TestService_Backup_HappyPath(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{content: []byte("db-content-v1")}
	store := &fakeStore{}
	svc := newSvc(t, drive, snap, store, &fakeRestarter{})

	res, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !res.Uploaded || res.Unchanged {
		t.Errorf("esperava upload, got %+v", res)
	}
	// latest + dated = 2 arquivos no drive
	if len(drive.files) != 2 {
		t.Errorf("esperava 2 arquivos (latest+datado), got %d", len(drive.files))
	}
	// sidecar atualizado
	if store.st.LastChecksumSHA256 == "" || store.st.LatestFileID == "" || len(store.st.Versions) != 1 {
		t.Errorf("sidecar não atualizado: %+v", store.st)
	}
}

func TestService_Backup_NoOp(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{content: []byte("same-content")}
	store := &fakeStore{}
	svc := newSvc(t, drive, snap, store, &fakeRestarter{})

	if _, err := svc.Backup(context.Background()); err != nil {
		t.Fatalf("primeiro backup: %v", err)
	}
	filesAfterFirst := len(drive.files)

	res, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("segundo backup: %v", err)
	}
	if res.Uploaded || !res.Unchanged {
		t.Errorf("esperava no-op, got %+v", res)
	}
	if len(drive.files) != filesAfterFirst {
		t.Errorf("no-op não deveria criar arquivos novos: %d → %d", filesAfterFirst, len(drive.files))
	}
}

func TestService_Backup_Offline(t *testing.T) {
	drive := newFakeDrive()
	drive.offline = true
	store := &fakeStore{}
	svc := newSvc(t, drive, &fakeSnap{content: []byte("x")}, store, &fakeRestarter{})

	_, err := svc.Backup(context.Background())
	if err != backup.ErrDriveOffline {
		t.Errorf("got %v, want ErrDriveOffline", err)
	}
	if store.st.LastError == nil {
		t.Error("LastError deveria ter sido registrado no sidecar")
	}
}

func TestService_Backup_ChecksumMismatch(t *testing.T) {
	drive := newFakeDrive()
	drive.badMD5 = true
	svc := newSvc(t, drive, &fakeSnap{content: []byte("x")}, &fakeStore{}, &fakeRestarter{})

	_, err := svc.Backup(context.Background())
	if err != backup.ErrChecksumMismatch {
		t.Errorf("got %v, want ErrChecksumMismatch", err)
	}
}

func TestService_Backup_Retention(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{}
	store := &fakeStore{}
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite",
			Retention: 2, DataDir: dir,
		},
		Drive: drive, Snap: snap, Store: store, Restarter: &fakeRestarter{},
		DBPath: filepath.Join(dir, "financas.sqlite"),
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// 3 backups com conteúdos diferentes → retenção 2 deve podar 1 datado
	for i, c := range []string{"v1", "v2", "v3"} {
		snap.content = []byte(c)
		res, err := svc.Backup(context.Background())
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		_ = res
	}
	if len(store.st.Versions) != 2 {
		t.Errorf("retenção: esperava 2 versões no sidecar, got %d", len(store.st.Versions))
	}
}

// ─── Status ─────────────────────────────────────────────────────────────────

func TestService_Status_DirtyAfterChange(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{content: []byte("v1")}
	store := &fakeStore{}
	svc := newSvc(t, drive, snap, store, &fakeRestarter{})

	svc.Backup(context.Background()) // salva v1
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.IsDirty {
		t.Error("logo após backup não deveria estar dirty")
	}

	snap.content = []byte("v2-changed")
	st, _ = svc.Status(context.Background())
	if !st.IsDirty {
		t.Error("após mudança deveria estar dirty")
	}
	if st.State != "dirty" {
		t.Errorf("state: got %q, want dirty", st.State)
	}
}

// ─── ListVersions ───────────────────────────────────────────────────────────

func TestService_ListVersions(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{}
	store := &fakeStore{}
	svc := newSvc(t, drive, snap, store, &fakeRestarter{})

	for _, c := range []string{"v1", "v2"} {
		snap.content = []byte(c)
		svc.Backup(context.Background())
	}
	res, err := svc.ListVersions(context.Background(), shared.Pagination{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 2 datados (a "latest" é excluída da listagem)
	if res.Pagination.Total != 2 || len(res.Data) != 2 {
		t.Errorf("got total=%d len=%d, want 2/2", res.Pagination.Total, len(res.Data))
	}
	for _, v := range res.Data {
		if v.Name == "financas-latest.sqlite" {
			t.Error("a 'latest' não deveria aparecer nas versões")
		}
	}
}

// ─── Restore ────────────────────────────────────────────────────────────────

func TestService_Restore_NotConfirmed(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{content: []byte("x")}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Restore(context.Background(), "", false); err != backup.ErrRestoreNotConfirmed {
		t.Errorf("got %v, want ErrRestoreNotConfirmed", err)
	}
}

func TestService_Restore_StagesPendingAndRestarts(t *testing.T) {
	drive := newFakeDrive()
	// coloca uma "latest" válida (sqlite real) no drive p/ passar no integrity_check
	valid := validSQLiteBytes(t)
	drive.put("financas-latest.sqlite", bytes.NewReader(valid))

	snap := &fakeSnap{content: []byte("current-db")} // safety snapshot (conteúdo qualquer)
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

	res, err := svc.Restore(context.Background(), "", true)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !res.RestartRequired {
		t.Error("esperava RestartRequired")
	}
	// pending criado e íntegro
	pending := backup.PendingRestorePath(dbPath)
	if _, err := os.Stat(pending); err != nil {
		t.Errorf("pending deveria existir: %v", err)
	}
	// handler é quem chama Restart; o serviço por si não chamou
	if rest.called != 0 {
		t.Error("o serviço não deveria chamar Restart (o handler chama após responder)")
	}
	svc.Restart()
	if rest.called != 1 {
		t.Error("Restart() deveria delegar ao restarter")
	}
}

func TestService_Restore_VersionNotFound(t *testing.T) {
	svc := newSvc(t, newFakeDrive(), &fakeSnap{content: []byte("x")}, &fakeStore{}, &fakeRestarter{})
	if _, err := svc.Restore(context.Background(), "inexistente", true); err != backup.ErrVersionNotFound {
		t.Errorf("got %v, want ErrVersionNotFound", err)
	}
}

// ─── SyncOnBoot (D16) ───────────────────────────────────────────────────────

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func bootCfg(dir string) config.BackupConfig {
	return config.BackupConfig{FolderName: "Financas", LatestFilename: "financas-latest.sqlite", DataDir: dir}
}

// Local-first: com banco local existente, o boot NUNCA sobrescreve (mesmo que o
// remoto pareça mais novo / sidecar zerado). Restaurar sobre um banco existente é
// ação EXPLÍCITA do usuário — sobrescrever no boot já causou perda de dados.
func TestSyncOnBoot_DoesNotOverwriteExistingLocal(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytes.NewReader(validSQLiteBytes(t)))
	store := &fakeStore{} // sidecar zero → no comportamento ANTIGO, restauraria por cima
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("local-keep"), 0o600)

	backup.SyncOnBoot(context.Background(), drive, store, bootCfg(dir), dbPath, discardLog())

	got, _ := os.ReadFile(dbPath)
	if string(got) != "local-keep" {
		t.Error("banco local existente NUNCA pode ser sobrescrito no boot")
	}
}

// Sem banco local (máquina nova / recuperação de desastre), o boot restaura do remoto.
func TestSyncOnBoot_RestoresWhenNoLocal(t *testing.T) {
	drive := newFakeDrive()
	valid := validSQLiteBytes(t)
	drive.put("financas-latest.sqlite", bytes.NewReader(valid))
	store := &fakeStore{}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite") // não existe ainda

	backup.SyncOnBoot(context.Background(), drive, store, bootCfg(dir), dbPath, discardLog())

	got, _ := os.ReadFile(dbPath)
	if !bytes.Equal(got, valid) {
		t.Error("sem banco local, o boot deveria restaurar a versão remota")
	}
	if store.st.LatestFileID == "" || store.st.LastChecksumSHA256 == "" {
		t.Errorf("sidecar deveria ter sido atualizado: %+v", store.st)
	}
}

func TestSyncOnBoot_NoopWhenLocalNewer(t *testing.T) {
	drive := newFakeDrive()
	drive.put("financas-latest.sqlite", bytes.NewReader(validSQLiteBytes(t)))
	store := &fakeStore{st: backup.BackupState{LastBackupAt: time.Now().Add(time.Hour)}, set: true}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("local-keep"), 0o600)

	backup.SyncOnBoot(context.Background(), drive, store, bootCfg(dir), dbPath, discardLog())

	got, _ := os.ReadFile(dbPath)
	if string(got) != "local-keep" {
		t.Error("local mais novo → não deveria trocar pelo remoto")
	}
}

func TestSyncOnBoot_NoRemote(t *testing.T) {
	drive := newFakeDrive() // sem arquivos
	store := &fakeStore{}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("local"), 0o600)

	backup.SyncOnBoot(context.Background(), drive, store, bootCfg(dir), dbPath, discardLog())

	got, _ := os.ReadFile(dbPath)
	if string(got) != "local" {
		t.Error("sem backup remoto → no-op")
	}
}

func TestSyncOnBoot_NilDriveIsNoop(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "financas.sqlite")
	os.WriteFile(dbPath, []byte("local"), 0o600)
	// não deve entrar em pânico com drive nil (feature desabilitada)
	backup.SyncOnBoot(context.Background(), nil, &fakeStore{}, bootCfg(dir), dbPath, discardLog())
	got, _ := os.ReadFile(dbPath)
	if string(got) != "local" {
		t.Error("drive nil → no-op")
	}
}
