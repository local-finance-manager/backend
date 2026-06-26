package backup

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/shared"
)

var backupAllowedOrderBy = []string{"createdAt"}

var backupPaginationDefaults = shared.Pagination{
	Page: 1, Limit: 100, OrderBy: "createdAt", Order: "DESC",
}

// Handler expõe os endpoints de backup.
type Handler struct{ svc *Service }

// NewHandler cria o Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── DTO de versão (snake_case) ─────────────────────────────────────────────

type versionResp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	Size      int64  `json:"size"`
}

type restoreReq struct {
	VersionID string `json:"version_id"`
	Confirm   bool   `json:"confirm"`
}

type restoreLocalReq struct {
	Snapshot string `json:"snapshot"`
	Confirm  bool   `json:"confirm"`
}

type localSnapshotResp struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	Size      int64  `json:"size"`
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// Backup trata POST /api/backup — roda os dois tiers (Drive + local), só-se-mudou.
func (h *Handler) Backup(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Run(r.Context())
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Status trata GET /api/backup/status
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context())
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ListVersions trata GET /api/backup/versions
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	p := shared.ParsePagination(r, backupPaginationDefaults, backupAllowedOrderBy)
	result, err := h.svc.ListVersions(r.Context(), p)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	data := make([]versionResp, len(result.Data))
	for i, v := range result.Data {
		data[i] = versionResp{
			ID: v.FileID, Name: v.Name,
			CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339), Size: v.Size,
		}
	}
	writeJSON(w, http.StatusOK, shared.PagedResult[versionResp]{Data: data, Pagination: result.Pagination})
}

// Restore trata POST /api/backup/restore. Responde e SÓ ENTÃO dispara o restart (D7).
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	var req restoreReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	res, err := h.svc.Restore(r.Context(), req.VersionID, req.Confirm)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
	if res.RestartRequired {
		h.svc.Restart() // agenda os.Exit; a resposta acima já foi enviada
	}
}

// ListLocalSnapshots trata GET /api/backup/local-snapshots (RF-BKP-18).
func (h *Handler) ListLocalSnapshots(w http.ResponseWriter, r *http.Request) {
	p := shared.ParsePagination(r, backupPaginationDefaults, backupAllowedOrderBy)
	result, err := h.svc.ListLocalSnapshots(r.Context(), p)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	data := make([]localSnapshotResp, len(result.Data))
	for i, sn := range result.Data {
		data[i] = localSnapshotResp{
			Name: sn.Name, CreatedAt: sn.CreatedAt.UTC().Format(time.RFC3339), Size: sn.Size,
		}
	}
	writeJSON(w, http.StatusOK, shared.PagedResult[localSnapshotResp]{Data: data, Pagination: result.Pagination})
}

// RestoreLocal trata POST /api/backup/restore-local. Responde e SÓ ENTÃO reinicia (D7).
func (h *Handler) RestoreLocal(w http.ResponseWriter, r *http.Request) {
	var req restoreLocalReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	res, err := h.svc.RestoreLocal(r.Context(), req.Snapshot, req.Confirm)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
	if res.RestartRequired {
		h.svc.Restart()
	}
}

// writeErr trata o caso offline como 503 (o govalidator não expõe 503 — D11); o
// restante usa o middleware padrão.
func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrDriveOffline) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      http.StatusServiceUnavailable,
			"message":     "sem conexão com o Google Drive",
			"displayable": true,
		})
		return
	}
	domainerr.WriteError(w, err)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
