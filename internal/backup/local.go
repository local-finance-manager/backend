package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// localSnapshotPrefix é o prefixo dos snapshots locais datados (financas-<data>.sqlite).
const localSnapshotPrefix = "financas"

// snapshotTimeLayout é o layout de data usado por DatedFilename (sem ':', ordenável).
const snapshotTimeLayout = "2006-01-02T15-04-05Z"

// LocalResult resume o que SnapshotLocal fez num run (RF-BKP-18).
type LocalResult struct {
	Skipped   bool      // tier desabilitado
	Unchanged bool      // no-op: SHA igual ao último snapshot local (RF-BKP-16)
	Created   bool      // snapshot novo gravado
	Name      string    // nome do snapshot criado
	Size      int64     // bytes do snapshot
	At        time.Time // data do snapshot (ou do último, no no-op)
}

// snapshotsDir é o diretório dos snapshots locais — no MESMO volume do .sqlite, para o
// VACUUM INTO + os.Rename serem atômicos (RNF-BKP2-03 / DA9).
func (s *Service) snapshotsDir() string {
	return filepath.Join(s.cfg.DataDir, "snapshots")
}

// SnapshotLocal gera um snapshot local datado SE o conteúdo mudou desde o último (no-op por
// SHA-256 do arquivo — RF-BKP-16). Independe do Drive (RF-BKP-18). Serializa em s.mu.
func (s *Service) SnapshotLocal(ctx context.Context) (LocalResult, error) {
	if !s.cfg.LocalSnapshotEnabled {
		return LocalResult{Skipped: true}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.snapshotsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return LocalResult{}, fmt.Errorf("backup local: mkdir snapshots: %w", err)
	}

	now := s.now().UTC()
	tmp := filepath.Join(s.cfg.DataDir, fmt.Sprintf(".local-snapshot-%d.sqlite", now.UnixNano()))
	if err := s.snap.Snapshot(ctx, tmp); err != nil {
		return LocalResult{}, fmt.Errorf("backup local: snapshot: %w", err)
	}
	defer os.Remove(tmp) // no-op/erro: limpa; sucesso: já foi renomeado, Remove é inócuo

	sha, _, size, err := hashFileBoth(tmp)
	if err != nil {
		return LocalResult{}, err
	}

	state, err := s.store.Load()
	if err != nil {
		return LocalResult{}, err
	}
	if !ShouldUpload(sha, state.LocalLastChecksumSHA256) {
		return LocalResult{Unchanged: true, At: state.LocalLastSnapshotAt, Size: size}, nil
	}

	// DatedFilename tem precisão de segundo: dois snapshots no mesmo segundo colidem e o
	// os.Rename sobrescreve (dedup benigno — mantém o mais recente do segundo). Os gatilhos
	// reais (autosave 15 min, Ctrl+S, shutdown) tornam isso irrelevante na prática.
	name := DatedFilename(localSnapshotPrefix, now)
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return LocalResult{}, fmt.Errorf("backup local: promote snapshot: %w", err)
	}

	pruned := s.pruneLocalSnapshots(dir)

	state.LocalLastChecksumSHA256 = sha
	state.LocalLastSnapshotAt = now
	if err := s.store.Save(state); err != nil {
		return LocalResult{}, err
	}
	s.log.Info("backup local: snapshot criado", "name", name, "size", size, "pruned", pruned)
	return LocalResult{Created: true, Name: name, Size: size, At: now}, nil
}

// pruneLocalSnapshots remove os snapshots locais além da retenção (RNF-BKP2-02: nunca o mais
// novo; falha de remoção loga e segue). Retorna quantos removeu.
func (s *Service) pruneLocalSnapshots(dir string) int {
	snaps, err := listLocalSnapshots(dir)
	if err != nil {
		s.log.Warn("backup local: prune list", "error", err)
		return 0
	}
	names := make([]string, len(snaps))
	for i, sn := range snaps {
		names[i] = sn.Name
	}
	pruned := 0
	for _, name := range LocalSnapshotsToPrune(names, s.cfg.LocalSnapshotRetention) {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			s.log.Warn("backup local: prune remove", "name", name, "error", err)
			continue
		}
		pruned++
	}
	return pruned
}

// ListLocalSnapshots lista os snapshots locais (mais novos primeiro), paginado (guia §9).
// A lista é pequena (<= retenção) → paginação em memória.
func (s *Service) ListLocalSnapshots(_ context.Context, p shared.Pagination) (shared.PagedResult[LocalSnapshot], error) {
	all, err := listLocalSnapshots(s.snapshotsDir())
	if err != nil {
		return shared.PagedResult[LocalSnapshot]{}, err
	}
	total := len(all)
	start := p.Offset()
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	return shared.NewPagedResult(all[start:end], total, p), nil
}

// RestoreLocal estagia um snapshot local para restauração no próximo boot, com backup de
// segurança do estado atual antes (RNF-BKP-06 / RF-BKP-18). Espelha o Restore do Drive.
func (s *Service) RestoreLocal(ctx context.Context, name string, confirm bool) (RestoreResult, error) {
	if !confirm {
		return RestoreResult{}, ErrRestoreNotConfirmed
	}
	// Segurança: o nome vem do cliente — só basename, padrão datado e existência no dir.
	clean := filepath.Base(name)
	if clean != name || clean == "" {
		return RestoreResult{}, ErrInvalidSnapshotName
	}
	if _, ok := parseSnapshotTime(clean); !ok {
		return RestoreResult{}, ErrInvalidSnapshotName
	}
	src := filepath.Join(s.snapshotsDir(), clean)
	if !fileExists(src) {
		return RestoreResult{}, ErrSnapshotNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// backup local de segurança do estado atual (banco aberto → VACUUM INTO consistente).
	safety := filepath.Join(s.cfg.DataDir, DatedFilename("financas-pre-restore", s.now().UTC()))
	if err := s.snap.Snapshot(ctx, safety); err != nil {
		return RestoreResult{}, fmt.Errorf("backup local: safety snapshot: %w", err)
	}

	// estagia a cópia e valida integridade ANTES de promover (aplicado no boot).
	pending := PendingRestorePath(s.dbPath)
	if err := copyFile(src, pending); err != nil {
		return RestoreResult{}, err
	}
	if err := integrityCheck(ctx, pending); err != nil {
		_ = os.Remove(pending)
		return RestoreResult{}, err
	}
	s.log.Info("backup local: restore staged; will apply on restart", "from", clean, "safety", filepath.Base(safety))
	return RestoreResult{RestartRequired: true, RestoredFrom: clean}, nil
}

// ─── Helpers de leitura do diretório ────────────────────────────────────────

// listLocalSnapshots lê o diretório e devolve os snapshots válidos, mais novos primeiro.
// Dir inexistente → lista vazia (não é erro).
func listLocalSnapshots(dir string) ([]LocalSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup local: read snapshots dir: %w", err)
	}
	out := make([]LocalSnapshot, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		t, ok := parseSnapshotTime(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, LocalSnapshot{Name: e.Name(), CreatedAt: t, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// parseSnapshotTime extrai a data do nome (autoritativa), independente do prefixo: o nome é
// "<prefixo>-<data>.sqlite" e a data tem largura fixa (snapshotTimeLayout).
func parseSnapshotTime(name string) (time.Time, bool) {
	if !strings.HasSuffix(name, ".sqlite") {
		return time.Time{}, false
	}
	base := strings.TrimSuffix(name, ".sqlite")
	if len(base) < len(snapshotTimeLayout) {
		return time.Time{}, false
	}
	t, err := time.Parse(snapshotTimeLayout, base[len(base)-len(snapshotTimeLayout):])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
