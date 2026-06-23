package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var _ StateStore = (*FileStateStore)(nil)

// FileStateStore persiste o BackupState num arquivo JSON (sidecar), fora do banco.
type FileStateStore struct{ path string }

// NewFileStateStore cria um FileStateStore no caminho dado.
func NewFileStateStore(path string) *FileStateStore {
	return &FileStateStore{path: path}
}

// Load lê o sidecar. Arquivo ausente NÃO é erro: devolve o estado zero (máquina nova).
func (s *FileStateStore) Load() (BackupState, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return BackupState{}, nil
	}
	if err != nil {
		return BackupState{}, fmt.Errorf("backup statestore: read: %w", err)
	}
	var st BackupState
	if err := json.Unmarshal(data, &st); err != nil {
		return BackupState{}, fmt.Errorf("backup statestore: unmarshal: %w", err)
	}
	return st, nil
}

// Save grava o sidecar de forma atômica (escreve em .tmp e renomeia), para não
// corromper o arquivo em caso de crash no meio da escrita.
func (s *FileStateStore) Save(st BackupState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("backup statestore: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("backup statestore: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("backup statestore: rename: %w", err)
	}
	return nil
}
