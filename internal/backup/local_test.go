package backup_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/backup"
	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/shared"
)

// newLocalSvc cria um Service só com o tier local ligado (Drive desabilitado).
func newLocalSvc(t *testing.T, snap backup.Snapshotter, store backup.StateStore) (*backup.Service, string) {
	t.Helper()
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Enabled: false,
		Cfg: config.BackupConfig{
			DataDir: dir, LocalSnapshotEnabled: true, LocalSnapshotRetention: 5,
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite",
		},
		Snap: snap, Store: store, Restarter: &fakeRestarter{},
		DBPath: filepath.Join(dir, "financas.sqlite"), Log: discardLog(),
	})
	return svc, dir
}

func snapshotsDir(dir string) string { return filepath.Join(dir, "snapshots") }

func writeSnapshotFile(t *testing.T, dir, iso string, content []byte) string {
	t.Helper()
	sd := snapshotsDir(dir)
	if err := os.MkdirAll(sd, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "financas-" + iso + ".sqlite"
	if err := os.WriteFile(filepath.Join(sd, name), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func countSnapshots(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(snapshotsDir(dir))
	if err != nil {
		return 0
	}
	return len(entries)
}

// ─── LocalSnapshotsToPrune (pura) ───────────────────────────────────────────

func TestLocalSnapshotsToPrune(t *testing.T) {
	n := func(iso string) string { return "financas-" + iso + ".sqlite" }
	names := []string{
		n("2026-06-01T00-00-00Z"), n("2026-06-05T00-00-00Z"), n("2026-06-03T00-00-00Z"),
		n("2026-06-02T00-00-00Z"), n("2026-06-04T00-00-00Z"), n("2026-06-06T00-00-00Z"),
	}
	cases := []struct {
		retention int
		wantLen   int
		mustKeep  string // o mais novo nunca é podado quando retention >= 1
	}{
		{5, 1, n("2026-06-06T00-00-00Z")},
		{6, 0, ""},
		{10, 0, ""},
		{0, 6, ""},
		{1, 5, ""},
	}
	for _, tc := range cases {
		prune := backup.LocalSnapshotsToPrune(names, tc.retention)
		if len(prune) != tc.wantLen {
			t.Errorf("retention=%d: podou %d, queria %d", tc.retention, len(prune), tc.wantLen)
		}
		if tc.mustKeep != "" {
			for _, p := range prune {
				if p == tc.mustKeep {
					t.Errorf("retention=%d: nunca deveria podar o mais novo %q", tc.retention, tc.mustKeep)
				}
			}
		}
	}
	if backup.LocalSnapshotsToPrune(nil, 5) != nil {
		t.Error("lista vazia deveria devolver nil")
	}
}

// ─── SnapshotLocal ──────────────────────────────────────────────────────────

func TestSnapshotLocal_Disabled(t *testing.T) {
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Cfg:  config.BackupConfig{DataDir: dir, LocalSnapshotEnabled: false},
		Snap: &fakeSnap{content: []byte("x")}, Store: &fakeStore{}, Log: discardLog(),
	})
	res, err := svc.SnapshotLocal(context.Background())
	if err != nil {
		t.Fatalf("snapshot local desabilitado: %v", err)
	}
	if !res.Skipped {
		t.Error("deveria ter Skipped=true quando o tier está desligado")
	}
}

func TestSnapshotLocal_CreatesAndNoOp(t *testing.T) {
	snap := &fakeSnap{content: []byte("conteudo-v1")}
	store := &fakeStore{}
	svc, dir := newLocalSvc(t, snap, store)
	ctx := context.Background()

	// 1ª vez: cria
	r1, err := svc.SnapshotLocal(ctx)
	if err != nil || !r1.Created {
		t.Fatalf("1º snapshot deveria criar: %+v err=%v", r1, err)
	}
	if countSnapshots(t, dir) != 1 {
		t.Fatalf("esperava 1 snapshot, got %d", countSnapshots(t, dir))
	}

	// 2ª vez sem mudança: no-op (RF-BKP-16)
	r2, err := svc.SnapshotLocal(ctx)
	if err != nil {
		t.Fatalf("2º snapshot: %v", err)
	}
	if !r2.Unchanged || r2.Created {
		t.Errorf("2º snapshot deveria ser no-op: %+v", r2)
	}
	if countSnapshots(t, dir) != 1 {
		t.Errorf("no-op não deveria criar arquivo: got %d", countSnapshots(t, dir))
	}

	// muda o conteúdo: cria de novo
	snap.content = []byte("conteudo-v2")
	r3, err := svc.SnapshotLocal(ctx)
	if err != nil || !r3.Created {
		t.Fatalf("3º snapshot (mudou) deveria criar: %+v err=%v", r3, err)
	}
}

func TestSnapshotLocal_Retention(t *testing.T) {
	snap := &fakeSnap{content: []byte("novo")}
	store := &fakeStore{}
	svc, dir := newLocalSvc(t, snap, store)

	// pré-cria 5 snapshots datados antigos
	oldest := writeSnapshotFile(t, dir, "2026-06-01T00-00-00Z", []byte("a"))
	for _, iso := range []string{"2026-06-02T00-00-00Z", "2026-06-03T00-00-00Z", "2026-06-04T00-00-00Z", "2026-06-05T00-00-00Z"} {
		writeSnapshotFile(t, dir, iso, []byte("a"))
	}

	// novo snapshot (now real é mais novo que 2026-06-05) → poda mantém 5
	if _, err := svc.SnapshotLocal(context.Background()); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := countSnapshots(t, dir); got != 5 {
		t.Errorf("retenção: esperava 5 snapshots, got %d", got)
	}
	if _, err := os.Stat(filepath.Join(snapshotsDir(dir), oldest)); !os.IsNotExist(err) {
		t.Error("o snapshot mais antigo deveria ter sido podado")
	}
}

// ─── RestoreLocal ───────────────────────────────────────────────────────────

func TestRestoreLocal(t *testing.T) {
	snap := &fakeSnap{content: validSQLiteBytes(t)} // safety snapshot
	store := &fakeStore{}
	svc, dir := newLocalSvc(t, snap, store)
	ctx := context.Background()
	name := writeSnapshotFile(t, dir, "2026-06-20T10-00-00Z", validSQLiteBytes(t))

	// não confirmado
	if _, err := svc.RestoreLocal(ctx, name, false); err != backup.ErrRestoreNotConfirmed {
		t.Errorf("não confirmado: got %v", err)
	}
	// path traversal
	if _, err := svc.RestoreLocal(ctx, "../"+name, true); err != backup.ErrInvalidSnapshotName {
		t.Errorf("path traversal deveria ser rejeitado: got %v", err)
	}
	// nome fora do padrão datado
	if _, err := svc.RestoreLocal(ctx, "qualquer.sqlite", true); err != backup.ErrInvalidSnapshotName {
		t.Errorf("nome inválido: got %v", err)
	}
	// inexistente (mas padrão válido)
	if _, err := svc.RestoreLocal(ctx, "financas-2099-01-01T00-00-00Z.sqlite", true); err != backup.ErrSnapshotNotFound {
		t.Errorf("inexistente: got %v", err)
	}
	// feliz
	res, err := svc.RestoreLocal(ctx, name, true)
	if err != nil {
		t.Fatalf("restore local: %v", err)
	}
	if !res.RestartRequired || res.RestoredFrom != name {
		t.Errorf("resultado inesperado: %+v", res)
	}
	if _, err := os.Stat(backup.PendingRestorePath(filepath.Join(dir, "financas.sqlite"))); err != nil {
		t.Errorf("pending restore deveria existir: %v", err)
	}
}

// ─── Run (orquestrador) ─────────────────────────────────────────────────────

func TestRun_LocalOnly(t *testing.T) {
	snap := &fakeSnap{content: []byte("dados")}
	store := &fakeStore{}
	svc, dir := newLocalSvc(t, snap, store) // Drive off, local on
	res, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("run local-only: %v", err)
	}
	if !res.Uploaded { // sintetizado do LocalResult.Created
		t.Errorf("esperava Uploaded(=Created) no run local-only: %+v", res)
	}
	if countSnapshots(t, dir) != 1 {
		t.Errorf("run deveria ter criado 1 snapshot local, got %d", countSnapshots(t, dir))
	}
	// 2ª vez: no-op
	res2, _ := svc.Run(context.Background())
	if !res2.Unchanged {
		t.Errorf("2º run sem mudança deveria ser Unchanged: %+v", res2)
	}
}

func TestRun_BothDisabled(t *testing.T) {
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Enabled: false,
		Cfg:     config.BackupConfig{DataDir: dir, LocalSnapshotEnabled: false},
		Snap:    &fakeSnap{content: []byte("x")}, Store: &fakeStore{}, Log: discardLog(),
	})
	if _, err := svc.Run(context.Background()); err != backup.ErrBackupDisabled {
		t.Errorf("ambos os tiers off deveria dar ErrBackupDisabled, got %v", err)
	}
}

func TestRun_DriveAndLocal(t *testing.T) {
	drive := newFakeDrive()
	snap := &fakeSnap{content: validSQLiteBytes(t)}
	store := &fakeStore{}
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 5,
			DataDir: dir, LocalSnapshotEnabled: true, LocalSnapshotRetention: 5,
		},
		Drive: drive, Snap: snap, Store: store, Restarter: &fakeRestarter{},
		DBPath: filepath.Join(dir, "financas.sqlite"), Log: discardLog(),
	})
	if _, err := svc.Run(context.Background()); err != nil {
		t.Fatalf("run dois tiers: %v", err)
	}
	if countSnapshots(t, dir) != 1 {
		t.Errorf("run deveria criar 1 snapshot local, got %d", countSnapshots(t, dir))
	}
}

// ─── Status com tier local ──────────────────────────────────────────────────

func TestStatus_LocalFields(t *testing.T) {
	snap := &fakeSnap{content: []byte("dados")}
	store := &fakeStore{}
	svc, dir := newLocalSvc(t, snap, store)
	ctx := context.Background()
	_, _ = svc.SnapshotLocal(ctx)

	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.LocalSnapshotsEnabled {
		t.Error("LocalSnapshotsEnabled deveria ser true")
	}
	if st.LocalSnapshotCount != 1 {
		t.Errorf("LocalSnapshotCount: got %d, want 1", st.LocalSnapshotCount)
	}
	if st.LocalLastSnapshotAt == nil {
		t.Error("LocalLastSnapshotAt deveria estar preenchido")
	}
	_ = dir
}

func TestListLocalSnapshots(t *testing.T) {
	svc, dir := newLocalSvc(t, &fakeSnap{content: []byte("x")}, &fakeStore{})
	writeSnapshotFile(t, dir, "2026-06-01T00-00-00Z", []byte("a"))
	writeSnapshotFile(t, dir, "2026-06-03T00-00-00Z", []byte("a"))
	writeSnapshotFile(t, dir, "2026-06-02T00-00-00Z", []byte("a"))
	// ruídos ignorados (cobre os ramos de parseSnapshotTime/listLocalSnapshots)
	sd := snapshotsDir(dir)
	os.WriteFile(filepath.Join(sd, "naosqlite.txt"), []byte("x"), 0o600)                        // não .sqlite
	os.WriteFile(filepath.Join(sd, "curto.sqlite"), []byte("x"), 0o600)                         // curto demais
	os.WriteFile(filepath.Join(sd, "financas-2026-13-99T99-99-99Z.sqlite"), []byte("x"), 0o600) // data inválida
	os.Mkdir(filepath.Join(sd, "umdir"), 0o700)                                                 // diretório

	res, err := svc.ListLocalSnapshots(context.Background(), shared.Pagination{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pagination.Total != 3 {
		t.Errorf("total: got %d, want 3 (ruídos devem ser ignorados)", res.Pagination.Total)
	}
	if len(res.Data) != 3 || res.Data[0].CreatedAt.Day() != 3 {
		t.Errorf("ordem (mais novos primeiro) inesperada: %+v", res.Data)
	}
	// paginação em memória
	res2, _ := svc.ListLocalSnapshots(context.Background(), shared.Pagination{Page: 2, Limit: 2})
	if res2.Pagination.Total != 3 || len(res2.Data) != 1 {
		t.Errorf("paginação page2/limit2: total=%d len=%d", res2.Pagination.Total, len(res2.Data))
	}
}

func TestSnapshotLocal_Errors(t *testing.T) {
	// snapshot falha
	s1, _ := newLocalSvc(t, &fakeSnap{err: errors.New("disco cheio")}, &fakeStore{})
	if _, err := s1.SnapshotLocal(context.Background()); err == nil {
		t.Error("SnapshotLocal deveria falhar quando o snapshot falha")
	}
	// store.Load falha
	s2, _ := newLocalSvc(t, &fakeSnap{content: []byte("x")}, errStore{loadErr: errors.New("boom")})
	if _, err := s2.SnapshotLocal(context.Background()); err == nil {
		t.Error("SnapshotLocal deveria falhar quando store.Load falha")
	}
}

func TestRun_DriveError(t *testing.T) {
	// Drive offline + local DESLIGADO → erro propaga (sem rede de segurança local).
	drive := newFakeDrive()
	drive.offline = true
	dir := t.TempDir()
	svc := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 5,
			DataDir: dir, LocalSnapshotEnabled: false,
		},
		Drive: drive, Snap: &fakeSnap{content: []byte("x")}, Store: &fakeStore{},
		Restarter: &fakeRestarter{}, DBPath: filepath.Join(dir, "financas.sqlite"), Log: discardLog(),
	})
	if _, err := svc.Run(context.Background()); err == nil {
		t.Error("Run deveria propagar o erro do Drive quando não há tier local")
	}

	// Drive offline + local LIGADO → erro do Drive é engolido (dado salvo localmente).
	dir2 := t.TempDir()
	drive2 := newFakeDrive()
	drive2.offline = true
	svc2 := backup.NewService(backup.Deps{
		Enabled: true,
		Cfg: config.BackupConfig{
			FolderName: "Financas", LatestFilename: "financas-latest.sqlite", Retention: 5,
			DataDir: dir2, LocalSnapshotEnabled: true, LocalSnapshotRetention: 5,
		},
		Drive: drive2, Snap: &fakeSnap{content: []byte("y")}, Store: &fakeStore{},
		Restarter: &fakeRestarter{}, DBPath: filepath.Join(dir2, "financas.sqlite"), Log: discardLog(),
	})
	if _, err := svc2.Run(context.Background()); err != nil {
		t.Errorf("Run não deveria propagar o erro do Drive quando o local salvou: %v", err)
	}
	if countSnapshots(t, dir2) != 1 {
		t.Errorf("o tier local deveria ter salvado mesmo com o Drive offline, got %d", countSnapshots(t, dir2))
	}
}

func TestRestoreLocal_IntegrityFails(t *testing.T) {
	svc, dir := newLocalSvc(t, &fakeSnap{content: validSQLiteBytes(t)}, &fakeStore{})
	// snapshot com conteúdo inválido (não-sqlite) → integrity_check do staging falha
	name := writeSnapshotFile(t, dir, "2026-06-20T10-00-00Z", []byte("não é sqlite"))
	if _, err := svc.RestoreLocal(context.Background(), name, true); err == nil {
		t.Error("RestoreLocal deveria falhar no integrity_check de um snapshot inválido")
	}
}

func TestSnapshotLocal_SaveError(t *testing.T) {
	svc, _ := newLocalSvc(t, &fakeSnap{content: []byte("x")}, errStore{saveErr: errors.New("save falhou")})
	if _, err := svc.SnapshotLocal(context.Background()); err == nil {
		t.Error("SnapshotLocal deveria falhar quando store.Save falha")
	}
}

func TestRun_LocalError(t *testing.T) {
	// só tier local, com snapshot falhando → Run propaga o erro local.
	svc, _ := newLocalSvc(t, &fakeSnap{err: errors.New("falhou")}, &fakeStore{})
	if _, err := svc.Run(context.Background()); err == nil {
		t.Error("Run deveria propagar o erro do tier local quando é o único ativo")
	}
}

func TestLocalSnapshotRoutes(t *testing.T) {
	svc, dir := newLocalSvc(t, &fakeSnap{content: validSQLiteBytes(t)}, &fakeStore{})
	name := writeSnapshotFile(t, dir, "2026-06-20T10-00-00Z", validSQLiteBytes(t))
	r := chi.NewRouter()
	r.Route("/api/backup", backup.Routes(backup.NewHandler(svc)))

	if code := backupReq(t, r, http.MethodGet, "/api/backup/local-snapshots", ""); code != http.StatusOK {
		t.Errorf("GET local-snapshots: got %d, want 200", code)
	}
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore-local", "{"); code != http.StatusBadRequest {
		t.Errorf("restore-local bad json: got %d, want 400", code)
	}
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore-local",
		`{"snapshot":"`+name+`","confirm":false}`); code != http.StatusBadRequest {
		t.Errorf("restore-local não confirmado: got %d, want 400", code)
	}
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore-local",
		`{"snapshot":"financas-2099-01-01T00-00-00Z.sqlite","confirm":true}`); code != http.StatusNotFound {
		t.Errorf("restore-local inexistente: got %d, want 404", code)
	}
	if code := backupReq(t, r, http.MethodPost, "/api/backup/restore-local",
		`{"snapshot":"`+name+`","confirm":true}`); code != http.StatusOK {
		t.Errorf("restore-local feliz: got %d, want 200", code)
	}
}

// ─── Determinismo do VACUUM INTO (RNF-BKP2-01) ──────────────────────────────

func shaFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestSnapshotter_Determinism prova a pré-condição do no-op (RNF-BKP2-01): o mesmo conteúdo
// lógico gera o mesmo SHA-256 do snapshot; uma mudança real gera um SHA diferente.
func TestSnapshotter_Determinism(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	for i := 0; i < 50; i++ {
		db.Exec("INSERT INTO t (v) VALUES ('linha')")
	}
	snap := backup.NewSQLiteSnapshotter(db)
	ctx := context.Background()

	a := filepath.Join(dir, "a.sqlite")
	b := filepath.Join(dir, "b.sqlite")
	if err := snap.Snapshot(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := snap.Snapshot(ctx, b); err != nil {
		t.Fatal(err)
	}
	if shaFile(t, a) != shaFile(t, b) {
		t.Fatal("VACUUM INTO não determinístico: mesmo conteúdo gerou hashes diferentes (no-op quebraria)")
	}

	// insert+delete volta ao mesmo estado lógico → mesmo hash
	db.Exec("INSERT INTO t (v) VALUES ('temp')")
	db.Exec("DELETE FROM t WHERE v='temp'")
	c := filepath.Join(dir, "c.sqlite")
	if err := snap.Snapshot(ctx, c); err != nil {
		t.Fatal(err)
	}
	if shaFile(t, a) != shaFile(t, c) {
		t.Error("insert+delete (mesmo estado lógico) deveria gerar o mesmo hash")
	}

	// mudança real → hash diferente
	db.Exec("INSERT INTO t (v) VALUES ('nova')")
	d := filepath.Join(dir, "d.sqlite")
	if err := snap.Snapshot(ctx, d); err != nil {
		t.Fatal(err)
	}
	if shaFile(t, a) == shaFile(t, d) {
		t.Error("mudança real deveria gerar um hash diferente")
	}
}
