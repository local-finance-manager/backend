package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/config"
	"github.com/local-finance-manager/backend/internal/shared"
)

const bootSyncSkew = 5 * time.Second

// Deps agrega as dependências do serviço (injetadas no main.go).
type Deps struct {
	Enabled   bool
	Cfg       config.BackupConfig
	Drive     DriveClient // nil quando desabilitado
	Snap      Snapshotter
	Store     StateStore
	Restarter Restarter
	DBPath    string
	Log       *slog.Logger
}

// Service orquestra backup/restore. Apenas um backup/restauração por vez (RNF-BKP-03).
type Service struct {
	enabled   bool
	cfg       config.BackupConfig
	drive     DriveClient
	snap      Snapshotter
	store     StateStore
	restarter Restarter
	dbPath    string
	log       *slog.Logger
	now       func() time.Time
	mu        sync.Mutex
}

// NewService cria o serviço de backup.
func NewService(d Deps) *Service {
	return &Service{
		enabled:   d.Enabled,
		cfg:       d.Cfg,
		drive:     d.Drive,
		snap:      d.Snap,
		store:     d.Store,
		restarter: d.Restarter,
		dbPath:    d.DBPath,
		log:       d.Log,
		now:       time.Now,
	}
}

// ─── Backup (RF-BKP-01..05) ─────────────────────────────────────────────────

func (s *Service) Backup(ctx context.Context) (BackupResult, error) {
	if !s.enabled {
		return BackupResult{State: StateIdle}, ErrBackupDisabled
	}
	if !s.mu.TryLock() {
		// coalescing: já há um backup em andamento (D6)
		return BackupResult{State: StateSaving}, nil
	}
	defer s.mu.Unlock()

	tmp := filepath.Join(s.cfg.DataDir, fmt.Sprintf(".snapshot-%d.sqlite", s.now().UnixNano()))
	if err := s.snap.Snapshot(ctx, tmp); err != nil {
		return BackupResult{State: StateError}, err
	}
	defer os.Remove(tmp)

	sha, md5sum, size, err := hashFileBoth(tmp)
	if err != nil {
		return BackupResult{State: StateError}, err
	}

	state, err := s.store.Load()
	if err != nil {
		return BackupResult{State: StateError}, err
	}

	if !ShouldUpload(sha, state.LastChecksumSHA256) {
		s.log.Info("backup: no-op (nada mudou)", "checksum", sha[:12])
		return BackupResult{Uploaded: false, Unchanged: true, BackupAt: state.LastBackupAt, State: StateIdle}, nil
	}

	folderID := state.DriveFolderID
	if folderID == "" {
		folderID, err = s.drive.EnsureFolder(ctx, s.cfg.FolderName)
		if err != nil {
			return s.fail(state, "ensure folder", err)
		}
	}

	now := s.now().UTC()
	prefix := strings.TrimSuffix(s.cfg.LatestFilename, "-latest.sqlite")
	datedName := DatedFilename(prefix, now)

	// 1) cópia datada (arquivo novo a cada backup)
	dated, err := s.uploadFromFile(ctx, tmp, func(r fileReader) (DriveFile, error) {
		return s.drive.UploadNew(ctx, folderID, datedName, r)
	})
	if err != nil {
		return s.fail(state, "upload dated", err)
	}
	if dated.MD5Checksum != md5sum {
		return s.failMismatch(state)
	}

	// 2) "latest" (sobrescreve)
	latest, err := s.uploadLatest(ctx, tmp, folderID, state.LatestFileID)
	if err != nil {
		return s.fail(state, "upload latest", err)
	}
	if latest.MD5Checksum != md5sum {
		return s.failMismatch(state)
	}

	// 3) retenção
	versions := append(state.Versions, Version{FileID: dated.ID, Name: datedName, CreatedAt: now, Size: size})
	prune := VersionsToPrune(versions, s.cfg.Retention)
	pruned := s.pruneVersions(ctx, prune)
	versions = removeVersions(versions, prune)

	// 4) persiste o sidecar
	state.DriveFolderID = folderID
	state.LatestFileID = latest.ID
	state.LastBackupAt = now
	state.LastChecksumSHA256 = sha
	state.LastBackupSize = size
	state.LastError = nil
	state.Versions = versions
	if err := s.store.Save(state); err != nil {
		return BackupResult{State: StateError}, err
	}

	s.log.Info("backup: uploaded", "size", size, "fileId", latest.ID, "pruned", pruned)
	return BackupResult{
		Uploaded: true, Unchanged: false, BackupAt: now, Size: size,
		ChecksumSHA256: sha, DriveFileID: latest.ID,
		VersionsRetained: len(versions), VersionsPruned: pruned, State: StateIdle,
	}, nil
}

// Run executa um ciclo nos dois tiers (Drive + local), cada um só-se-mudou (RF-BKP-16) e
// best-effort ENTRE si: Drive offline NÃO impede o snapshot local (RF-BKP-18 / DA4). É o ponto
// de entrada dos três gatilhos (Ctrl+S, autosave, shutdown).
//
// Concorrência (DA4): Run NÃO segura s.mu — chama Backup e SnapshotLocal em sequência, cada um
// adquirindo s.mu por conta própria (Go não tem mutex reentrante).
func (s *Service) Run(ctx context.Context) (BackupResult, error) {
	driveOn := s.enabled
	localOn := s.cfg.LocalSnapshotEnabled
	if !driveOn && !localOn {
		return BackupResult{State: StateIdle}, ErrBackupDisabled
	}

	var driveRes BackupResult
	var driveErr error
	if driveOn {
		driveRes, driveErr = s.Backup(ctx)
	}

	var localRes LocalResult
	var localErr error
	if localOn {
		if localRes, localErr = s.SnapshotLocal(ctx); localErr != nil {
			s.log.Warn("backup: snapshot local falhou", "error", localErr)
		}
	}

	if driveOn {
		// Erro do Drive só vira erro do Run se NÃO houve rede de segurança local que tenha
		// salvado — preserva o 503/erro de hoje quando não existe fallback local.
		if driveErr != nil && (!localOn || localErr != nil) {
			return driveRes, driveErr
		}
		return driveRes, nil
	}

	// Só o tier local: sintetiza um BackupResult para manter a resposta compatível (DA4).
	if localErr != nil {
		return BackupResult{State: StateError}, localErr
	}
	return BackupResult{
		Uploaded:  localRes.Created,
		Unchanged: localRes.Unchanged,
		BackupAt:  localRes.At,
		Size:      localRes.Size,
		State:     StateIdle,
	}, nil
}

type fileReader = *os.File

// uploadFromFile abre o snapshot e passa o reader para a função de upload (cada upload
// consome um reader; reabrimos o arquivo a cada chamada).
func (s *Service) uploadFromFile(_ context.Context, path string, fn func(fileReader) (DriveFile, error)) (DriveFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return DriveFile{}, fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer f.Close()
	return fn(f)
}

func (s *Service) uploadLatest(ctx context.Context, tmp, folderID, knownFileID string) (DriveFile, error) {
	fileID := knownFileID
	if fileID == "" {
		found, err := s.drive.FindByName(ctx, folderID, s.cfg.LatestFilename)
		if err != nil {
			return DriveFile{}, err
		}
		if found != nil {
			fileID = found.ID
		}
	}
	if fileID != "" {
		return s.uploadFromFile(ctx, tmp, func(r fileReader) (DriveFile, error) {
			return s.drive.UpdateContents(ctx, fileID, r)
		})
	}
	return s.uploadFromFile(ctx, tmp, func(r fileReader) (DriveFile, error) {
		return s.drive.UploadNew(ctx, folderID, s.cfg.LatestFilename, r)
	})
}

func (s *Service) pruneVersions(ctx context.Context, prune []Version) int {
	pruned := 0
	for _, v := range prune {
		if err := s.drive.Delete(ctx, v.FileID); err != nil {
			s.log.Warn("backup: prune failed", "fileId", v.FileID, "error", err)
			continue
		}
		pruned++
	}
	return pruned
}

func (s *Service) fail(state BackupState, stage string, err error) (BackupResult, error) {
	msg := fmt.Sprintf("%s: %v", stage, err)
	state.LastError = &msg
	_ = s.store.Save(state)
	if isOffline(err) {
		s.log.Warn("backup: offline", "stage", stage, "error", err)
		return BackupResult{State: StateOffline}, ErrDriveOffline
	}
	// Erro de domínio (ex.: 403 do Drive traduzido no adapter) → propaga displayable,
	// em vez de virar um 500 genérico.
	if _, ok := domainerr.IsDomain(err); ok {
		s.log.Warn("backup: drive rejeitou", "stage", stage, "error", err)
		return BackupResult{State: StateError}, err
	}
	s.log.Error("backup: failed", "stage", stage, "error", err)
	return BackupResult{State: StateError}, fmt.Errorf("backup: %s: %w", stage, err)
}

func (s *Service) failMismatch(state BackupState) (BackupResult, error) {
	msg := "checksum divergente após upload"
	state.LastError = &msg
	_ = s.store.Save(state)
	s.log.Error("backup: checksum mismatch")
	return BackupResult{State: StateError}, ErrChecksumMismatch
}

// driveCallErr classifica um erro de chamada ao Drive nas operações de leitura/restauração:
// offline → ErrDriveOffline; erro de domínio (ex.: 403 traduzido no adapter) → propaga;
// caso contrário, embrulha com contexto.
func driveCallErr(stage string, err error) error {
	if isOffline(err) {
		return ErrDriveOffline
	}
	if _, ok := domainerr.IsDomain(err); ok {
		return err
	}
	return fmt.Errorf("backup: %s: %w", stage, err)
}

// ─── Status (RF-BKP-09/12) ──────────────────────────────────────────────────

func (s *Service) Status(ctx context.Context) (Status, error) {
	state, err := s.store.Load()
	if err != nil {
		return Status{}, err
	}

	// Tier local (RF-BKP-19): sempre reportado, mesmo com o Drive desabilitado.
	st := Status{
		State:                 StateIdle,
		LocalSnapshotsEnabled: s.cfg.LocalSnapshotEnabled,
		LocalSnapshotCount:    s.localSnapshotCount(),
	}
	if !state.LocalLastSnapshotAt.IsZero() {
		t := state.LocalLastSnapshotAt
		st.LocalLastSnapshotAt = &t
	}

	if !s.enabled {
		return st, nil // Drive desligado; só o tier local
	}

	st.SyncEnabled = true
	st.LastBackupSize = state.LastBackupSize
	st.LastChecksumSHA256 = state.LastChecksumSHA256
	st.DriveFolderID = state.DriveFolderID
	st.LastError = state.LastError
	if !state.LastBackupAt.IsZero() {
		t := state.LastBackupAt
		st.LastBackupAt = &t
	}
	if dirty, derr := s.computeDirty(ctx, state.LastChecksumSHA256); derr == nil {
		st.IsDirty = dirty
		if dirty {
			st.State = StateDirty
		}
	}
	if state.LastError != nil {
		st.State = StateError
	}
	return st, nil
}

// localSnapshotCount conta os snapshots locais válidos (0 em erro/dir inexistente).
func (s *Service) localSnapshotCount() int {
	snaps, err := listLocalSnapshots(s.snapshotsDir())
	if err != nil {
		return 0
	}
	return len(snaps)
}

// computeDirty gera um snapshot e compara o checksum com o último backup (D8).
func (s *Service) computeDirty(ctx context.Context, lastSHA string) (bool, error) {
	if lastSHA == "" {
		return true, nil // nunca houve backup → há dados não salvos
	}
	tmp := filepath.Join(s.cfg.DataDir, fmt.Sprintf(".dirtycheck-%d.sqlite", s.now().UnixNano()))
	if err := s.snap.Snapshot(ctx, tmp); err != nil {
		return false, err
	}
	defer os.Remove(tmp)
	sha, _, _, err := hashFileBoth(tmp)
	if err != nil {
		return false, err
	}
	return ShouldUpload(sha, lastSHA), nil
}

// ─── ListVersions (RF-BKP-08) ───────────────────────────────────────────────

func (s *Service) ListVersions(ctx context.Context, p shared.Pagination) (shared.PagedResult[Version], error) {
	if !s.enabled {
		return shared.PagedResult[Version]{}, ErrBackupDisabled
	}
	folderID, err := s.folderID(ctx)
	if err != nil {
		return shared.PagedResult[Version]{}, err
	}
	files, err := s.drive.List(ctx, folderID)
	if err != nil {
		return shared.PagedResult[Version]{}, driveCallErr("list versions", err)
	}

	versions := make([]Version, 0, len(files))
	for _, f := range files {
		if f.Name == s.cfg.LatestFilename {
			continue // "latest" não é uma versão datada do histórico
		}
		versions = append(versions, Version{FileID: f.ID, Name: f.Name, CreatedAt: f.CreatedTime, Size: f.Size})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].CreatedAt.After(versions[j].CreatedAt) })

	total := len(versions)
	start := p.Offset()
	if start > total {
		start = total
	}
	end := start + p.Limit
	if p.Limit <= 0 || end > total {
		end = total
	}
	return shared.NewPagedResult(versions[start:end], total, p), nil
}

// ─── Restore (RF-BKP-08, D7) ────────────────────────────────────────────────

func (s *Service) Restore(ctx context.Context, versionID string, confirm bool) (RestoreResult, error) {
	if !s.enabled {
		return RestoreResult{}, ErrBackupDisabled
	}
	if !confirm {
		return RestoreResult{}, ErrRestoreNotConfirmed
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	folderID, err := s.folderID(ctx)
	if err != nil {
		return RestoreResult{}, err
	}

	target, err := s.resolveTarget(ctx, folderID, versionID)
	if err != nil {
		return RestoreResult{}, err
	}

	// backup local de segurança (banco aberto → VACUUM INTO é a cópia consistente) — RNF-BKP-06
	safety := filepath.Join(s.cfg.DataDir, DatedFilename("financas-pre-restore", s.now().UTC()))
	if err := s.snap.Snapshot(ctx, safety); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: safety snapshot: %w", err)
	}

	// baixa para staging e valida integridade ANTES de promover (no boot — D7)
	pending := PendingRestorePath(s.dbPath)
	rc, err := s.drive.Download(ctx, target.ID)
	if err != nil {
		return RestoreResult{}, driveCallErr("download restore", err)
	}
	_, _, err = writeAndHashSHA(pending, rc)
	rc.Close()
	if err != nil {
		_ = os.Remove(pending)
		return RestoreResult{}, err
	}
	if err := integrityCheck(ctx, pending); err != nil {
		_ = os.Remove(pending)
		return RestoreResult{}, err
	}

	s.log.Info("backup: restore staged; will apply on restart", "from", target.Name, "safety", filepath.Base(safety))
	return RestoreResult{RestartRequired: true, RestoredFrom: target.Name}, nil
}

func (s *Service) resolveTarget(ctx context.Context, folderID, versionID string) (DriveFile, error) {
	if versionID == "" {
		found, err := s.drive.FindByName(ctx, folderID, s.cfg.LatestFilename)
		if err != nil {
			return DriveFile{}, driveCallErr("find latest", err)
		}
		if found == nil {
			return DriveFile{}, ErrVersionNotFound
		}
		return *found, nil
	}
	files, err := s.drive.List(ctx, folderID)
	if err != nil {
		return DriveFile{}, driveCallErr("list for restore", err)
	}
	for _, f := range files {
		if f.ID == versionID {
			return f, nil
		}
	}
	return DriveFile{}, ErrVersionNotFound
}

// Restart delega ao Restarter (chamado pelo handler APÓS escrever a resposta — D7).
func (s *Service) Restart() {
	if s.restarter != nil {
		s.restarter.Restart()
	}
}

// ─── BackupBestEffort (shutdown — RF-BKP-06/D15) ────────────────────────────

func (s *Service) BackupBestEffort(ctx context.Context) {
	if !s.enabled && !s.cfg.LocalSnapshotEnabled {
		return
	}
	res, err := s.Run(ctx) // cobre os dois tiers no shutdown (RF-BKP-16/18)
	if err != nil {
		s.log.Warn("backup: shutdown best-effort failed", "error", err)
		return
	}
	s.log.Info("backup: shutdown", "uploaded", res.Uploaded, "unchanged", res.Unchanged)
}

func (s *Service) folderID(ctx context.Context) (string, error) {
	state, err := s.store.Load()
	if err != nil {
		return "", err
	}
	if state.DriveFolderID != "" {
		return state.DriveFolderID, nil
	}
	id, err := s.drive.EnsureFolder(ctx, s.cfg.FolderName)
	if err != nil {
		return "", driveCallErr("ensure folder", err)
	}
	return id, nil
}

// removeVersions devolve `versions` sem as entradas presentes em `remove`.
func removeVersions(versions, remove []Version) []Version {
	if len(remove) == 0 {
		return versions
	}
	removed := make(map[string]struct{}, len(remove))
	for _, v := range remove {
		removed[v.FileID] = struct{}{}
	}
	kept := make([]Version, 0, len(versions))
	for _, v := range versions {
		if _, ok := removed[v.FileID]; !ok {
			kept = append(kept, v)
		}
	}
	return kept
}

// ─── SyncOnBoot ("mais recente vence" — D16) ────────────────────────────────

// SyncOnBoot, no boot e ANTES de database.Open, baixa o backup remoto se ele for mais
// novo que o local (ou se não houver banco local). Best-effort: qualquer falha (offline,
// timeout) é logada e o boot segue com o banco local (RNF-BKP-13). Não precisa de restart
// porque ninguém tem o banco aberto ainda.
func SyncOnBoot(ctx context.Context, drive DriveClient, store StateStore, cfg config.BackupConfig, dbPath string, log *slog.Logger) {
	if drive == nil {
		return
	}
	state, err := store.Load()
	if err != nil {
		log.Warn("backup boot-sync: load state", "error", err)
		return
	}
	folderID := state.DriveFolderID
	if folderID == "" {
		folderID, err = drive.EnsureFolder(ctx, cfg.FolderName)
		if err != nil {
			log.Warn("backup boot-sync: ensure folder (seguindo com local)", "error", err)
			return
		}
	}
	remote, err := drive.FindByName(ctx, folderID, cfg.LatestFilename)
	if err != nil {
		log.Warn("backup boot-sync: find remote (seguindo com local)", "error", err)
		return
	}
	if remote == nil {
		return // nada remoto ainda
	}

	// Local-first: o banco LOCAL é a fonte da verdade. Só restauramos do Drive
	// quando NÃO existe banco local (máquina nova / recuperação de desastre).
	// NUNCA sobrescrevemos automaticamente um banco existente no boot — isso já
	// causou perda de dados (o "mais recente vence" baixava versão velha por cima
	// quando o sidecar de estado se perdia). Restaurar sobre um banco existente é
	// ação EXPLÍCITA do usuário, com confirmação (RF-BKP-08), não comportamento de boot.
	if fileExists(dbPath) {
		return
	}

	tmp := dbPath + ".bootsync.tmp"
	rc, err := drive.Download(ctx, remote.ID)
	if err != nil {
		log.Warn("backup boot-sync: download (seguindo com local)", "error", err)
		return
	}
	sha, size, err := writeAndHashSHA(tmp, rc)
	rc.Close()
	if err != nil {
		_ = os.Remove(tmp)
		log.Warn("backup boot-sync: write tmp", "error", err)
		return
	}
	if err := integrityCheck(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		log.Warn("backup boot-sync: integridade do remoto falhou; mantendo local", "error", err)
		return
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(tmp, dbPath); err != nil {
		_ = os.Remove(tmp)
		log.Warn("backup boot-sync: swap falhou", "error", err)
		return
	}

	state.DriveFolderID = folderID
	state.LatestFileID = remote.ID
	state.LastBackupAt = remote.ModifiedTime
	state.LastChecksumSHA256 = sha
	state.LastBackupSize = size
	state.LastError = nil
	if err := store.Save(state); err != nil {
		log.Warn("backup boot-sync: save state", "error", err)
	}
	log.Info("backup boot-sync: restaurada versão mais recente do Drive", "size", size)
}
