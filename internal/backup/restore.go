package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// pendingRestoreName é o arquivo de staging deixado por uma restauração em runtime,
// aplicado no próximo boot (D7).
const pendingRestoreName = ".restore-pending.sqlite"

// PendingRestorePath retorna o caminho do arquivo de staging para o banco em dbPath.
func PendingRestorePath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), pendingRestoreName)
}

// ApplyPendingRestore aplica um restore deixado em staging, se existir. Deve ser
// chamado no main.go ANTES de database.Open — assim o swap acontece sem ninguém com
// o banco aberto (D7). Idempotente: no-op se não há pending.
func ApplyPendingRestore(dbPath string) (applied bool, err error) {
	pending := PendingRestorePath(dbPath)
	if _, statErr := os.Stat(pending); errors.Is(statErr, os.ErrNotExist) {
		return false, nil
	} else if statErr != nil {
		return false, fmt.Errorf("backup restore: stat pending: %w", statErr)
	}

	// Remove WAL/SHM órfãos do banco antigo antes de trocar o arquivo principal.
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	if err := os.Rename(pending, dbPath); err != nil {
		return false, fmt.Errorf("backup restore: apply pending: %w", err)
	}
	return true, nil
}
