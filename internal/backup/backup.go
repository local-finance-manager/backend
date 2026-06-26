// Package backup implements consistent SQLite snapshots backed up to Google Drive,
// with no-op detection, dated versioning/retention and restore. The domain here is
// pure (no I/O, no Google SDK): I/O lives behind ports (ver ports.go).
package backup

import (
	"fmt"
	"sort"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
)

// ─── Estados e tipos ────────────────────────────────────────────────────────

// State é o estado de salvamento reportado ao frontend (RF-BKP-09/10).
type State string

const (
	StateIdle    State = "idle"
	StateSaving  State = "saving"
	StateDirty   State = "dirty"
	StateOffline State = "offline"
	StateError   State = "error"
)

// BackupState é o conteúdo do sidecar JSON (Apêndice A do requisito).
type BackupState struct {
	DriveFolderID      string    `json:"driveFolderId"`
	LatestFileID       string    `json:"latestFileId"`
	LastBackupAt       time.Time `json:"lastBackupAt"`
	LastChecksumSHA256 string    `json:"lastChecksumSHA256"`
	LastBackupSize     int64     `json:"lastBackupSize"`
	LastError          *string   `json:"lastError"`
	Versions           []Version `json:"versions"`

	// Tier local (RF-BKP-18) — baseline do no-op local, independente do Drive.
	LocalLastChecksumSHA256 string    `json:"localLastChecksumSHA256"`
	LocalLastSnapshotAt     time.Time `json:"localLastSnapshotAt"`
}

// Version é uma cópia datada no Drive.
type Version struct {
	FileID    string    `json:"fileId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
}

// DriveFile é a projeção neutra de um arquivo no Drive (sem a SDK do Google).
type DriveFile struct {
	ID           string
	Name         string
	Size         int64
	MD5Checksum  string
	ModifiedTime time.Time
	CreatedTime  time.Time
}

// Status é a resposta de GET /api/backup/status (RF-BKP-09). JSON em snake_case
// (convenção do backend — category/transaction/creditcard).
type Status struct {
	SyncEnabled        bool       `json:"sync_enabled"`
	State              State      `json:"state"`
	IsDirty            bool       `json:"is_dirty"`
	LastBackupAt       *time.Time `json:"last_backup_at"`
	LastBackupSize     int64      `json:"last_backup_size"`
	LastChecksumSHA256 string     `json:"last_checksum_sha256"`
	DriveFolderID      string     `json:"drive_folder_id"`
	RemoteNewer        bool       `json:"remote_newer"`
	LastError          *string    `json:"last_error"`

	// Tier de snapshot local (RF-BKP-19) — aditivo, snake_case (consistência do módulo).
	LocalSnapshotsEnabled bool       `json:"local_snapshots_enabled"`
	LocalLastSnapshotAt   *time.Time `json:"local_last_snapshot_at"`
	LocalSnapshotCount    int        `json:"local_snapshot_count"`
}

// LocalSnapshot é um snapshot local datado (resposta de GET /api/backup/local-snapshots).
type LocalSnapshot struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

// BackupResult é a resposta de POST /api/backup.
type BackupResult struct {
	Uploaded         bool      `json:"uploaded"`
	Unchanged        bool      `json:"unchanged"`
	State            State     `json:"-"`
	BackupAt         time.Time `json:"backup_at"`
	Size             int64     `json:"size,omitempty"`
	ChecksumSHA256   string    `json:"checksum_sha256,omitempty"`
	DriveFileID      string    `json:"drive_file_id,omitempty"`
	VersionsRetained int       `json:"versions_retained,omitempty"`
	VersionsPruned   int       `json:"versions_pruned,omitempty"`
}

// RestoreResult é a resposta de POST /api/backup/restore.
type RestoreResult struct {
	RestartRequired bool   `json:"restart_required"`
	RestoredFrom    string `json:"restored_from"`
}

// ─── Erros de domínio ───────────────────────────────────────────────────────

var (
	ErrBackupDisabled = domainerr.NewConflict(
		"backup não configurado", domainerr.WithDisplayable())
	// ErrDriveOffline: ideal seria 503, mas o govalidator (v0.1.0) não expõe 503.
	// O handler detecta este sentinel e escreve 503 (ver handler.go / D11).
	ErrDriveOffline = domainerr.NewConflict(
		"sem conexão com o Google Drive", domainerr.WithDisplayable())
	ErrChecksumMismatch = domainerr.NewInternal(
		"falha de integridade no backup (checksum divergente)")
	ErrRestoreNotConfirmed = domainerr.NewBadRequest(
		"restauração exige confirmação explícita", domainerr.WithDisplayable())
	ErrVersionNotFound = domainerr.NewNotFound(
		"versão de backup não encontrada", domainerr.WithDisplayable())
	ErrSnapshotNotFound = domainerr.NewNotFound(
		"snapshot local não encontrado", domainerr.WithDisplayable())
	ErrInvalidSnapshotName = domainerr.NewBadRequest(
		"nome de snapshot inválido", domainerr.WithDisplayable())
)

// ─── Regras puras (testáveis sem I/O) ───────────────────────────────────────

// ShouldUpload reporta se o snapshot atual difere do último backup (no-op save, RF-BKP-03).
func ShouldUpload(currentSHA256, lastSHA256 string) bool {
	return currentSHA256 != lastSHA256
}

// VersionsToPrune devolve as versões que devem ser removidas para respeitar a
// retenção: mantém as `retention` mais recentes (por CreatedAt), poda o resto.
// retention <= 0 → poda todas (só a "latest" sobrevive). A ordem de entrada é irrelevante.
func VersionsToPrune(versions []Version, retention int) []Version {
	if len(versions) == 0 {
		return nil
	}
	sorted := make([]Version, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt) // mais novas primeiro
	})
	if retention < 0 {
		retention = 0
	}
	if retention >= len(sorted) {
		return nil
	}
	return sorted[retention:] // tudo além das N mais novas
}

// IsRemoteNewer reporta se o backup remoto é mais novo que o último backup local,
// com tolerância de skew de relógio (D9/D16).
func IsRemoteNewer(remoteModified, localLastBackup time.Time, skew time.Duration) bool {
	return remoteModified.After(localLastBackup.Add(skew))
}

// DatedFilename gera o nome de uma cópia datada, ex.: "financas-2026-06-12T14-30-00Z.sqlite".
// Usa '-' no lugar de ':' (caractere inválido em nome de arquivo).
func DatedFilename(prefix string, t time.Time) string {
	return fmt.Sprintf("%s-%s.sqlite", prefix, t.UTC().Format("2006-01-02T15-04-05Z"))
}

// LocalSnapshotsToPrune devolve os nomes de snapshots locais a remover para respeitar a
// retenção. Os nomes datados (DatedFilename) são lexicograficamente ordenáveis por data, então
// basta ordenar desc e podar além dos `retention` mais novos. retention <= 0 → poda todos.
// Função pura (testável sem I/O); RNF-BKP2-02: nunca devolve o mais novo quando retention >= 1.
func LocalSnapshotsToPrune(names []string, retention int) []string {
	if len(names) == 0 {
		return nil
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Sort(sort.Reverse(sort.StringSlice(sorted))) // mais novos primeiro (nome datado ISO)
	if retention < 0 {
		retention = 0
	}
	if retention >= len(sorted) {
		return nil
	}
	return sorted[retention:]
}
