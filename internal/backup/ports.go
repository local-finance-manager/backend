package backup

import (
	"context"
	"io"
)

// DriveClient é tudo que o serviço precisa do Google Drive. Implementado por
// drive.go (SDK google) e injetado no main.go. O domínio/serviço não conhece a SDK.
type DriveClient interface {
	EnsureFolder(ctx context.Context, name string) (folderID string, err error)
	UploadNew(ctx context.Context, folderID, name string, r io.Reader) (DriveFile, error)
	UpdateContents(ctx context.Context, fileID string, r io.Reader) (DriveFile, error)
	FindByName(ctx context.Context, folderID, name string) (*DriveFile, error)
	List(ctx context.Context, folderID string) ([]DriveFile, error)
	Download(ctx context.Context, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, fileID string) error
}

// Snapshotter gera um snapshot íntegro do banco num arquivo (VACUUM INTO).
type Snapshotter interface {
	Snapshot(ctx context.Context, destPath string) error
}

// StateStore lê/grava o sidecar JSON de estado do backup.
type StateStore interface {
	Load() (BackupState, error)
	Save(s BackupState) error
}

// Restarter reinicia o processo (usado após uma restauração em runtime — D7).
// Implementado no main.go (os.Exit após um pequeno delay; Docker re-sobe).
type Restarter interface {
	Restart()
}
