package report

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"
)

// Handler trata as requisições HTTP do módulo de relatórios.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func qInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Monthly — GET /api/reports/monthly?reference=YYYY-MM[&mode=realizado|projetivo]
func (h *Handler) Monthly(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("reference")
	if ref == "" {
		ref = time.Now().UTC().Format("2006-01")
	}
	mode := r.URL.Query().Get("mode")
	rep, err := h.svc.Monthly(r.Context(), ref, mode, r.URL.Query().Get("regime"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// Quarterly — GET /api/reports/quarterly?year=YYYY&quarter=1..4[&regime=caixa|competencia]
func (h *Handler) Quarterly(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	year := qInt(r, "year", now.Year())
	quarter := qInt(r, "quarter", (int(now.Month())-1)/3+1)
	rep, err := h.svc.Quarterly(r.Context(), year, quarter, r.URL.Query().Get("regime"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// Semiannual — GET /api/reports/semiannual?year=YYYY&half=1|2
func (h *Handler) Semiannual(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	defHalf := 1
	if now.Month() > 6 {
		defHalf = 2
	}
	year := qInt(r, "year", now.Year())
	half := qInt(r, "half", defHalf)
	rep, err := h.svc.Semiannual(r.Context(), year, half, r.URL.Query().Get("regime"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// Annual — GET /api/reports/annual?year=YYYY[&regime=caixa|competencia]
func (h *Handler) Annual(w http.ResponseWriter, r *http.Request) {
	year := qInt(r, "year", time.Now().UTC().Year())
	rep, err := h.svc.Annual(r.Context(), year, r.URL.Query().Get("regime"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// ListClosings — GET /api/reports/closings
func (h *Handler) ListClosings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListClosings(r.Context())
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

type closeReq struct {
	Reference string `json:"reference"`
}

// CloseMonth — POST /api/reports/closings { reference }
func (h *Handler) CloseMonth(w http.ResponseWriter, r *http.Request) {
	var req closeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	c, err := h.svc.Close(r.Context(), req.Reference)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ClosingView{
		Reference:  c.Reference,
		Status:     StateAdjustable,
		ClosedAt:   c.ClosedAt.UTC().Format(time.RFC3339),
		HardLockAt: c.HardLockAt,
	})
}

// Recalculate — POST /api/reports/closings/{reference}/recalculate
func (h *Handler) Recalculate(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "reference")
	if err := h.svc.Recalculate(r.Context(), ref); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LockState — GET /api/reports/closings/{reference}/lock-state
func (h *Handler) LockState(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "reference")
	lastDay, err := MonthLastDay(ref)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	st, err := h.svc.LockState(r.Context(), lastDay)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reference": ref, "status": st})
}
