package budget

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"
)

// Handler trata as requisições HTTP do módulo de alocação de receitas.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return false
	}
	return true
}

// ─── Plano ───────────────────────────────────────────────────────────────────

// GetPlan — GET /api/income/plan?reference=YYYY-MM
func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("reference")
	if ref == "" {
		ref = time.Now().UTC().Format("2006-01")
	}
	plan, err := h.svc.GetPlan(r.Context(), ref)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// ─── Destinos ────────────────────────────────────────────────────────────────

type destinationReq struct {
	Reference           string  `json:"reference"`
	Name                string  `json:"name"`
	Kind                string  `json:"kind"`
	Mode                string  `json:"mode"`
	Percentage          *int    `json:"percentage"`
	FixedAmount         *int64  `json:"fixedAmount"`
	PresetSubcategoryID *string `json:"presetSubcategoryId"`
	PresetPaymentMethod *string `json:"presetPaymentMethod"`
	PresetDescription   *string `json:"presetDescription"`
	CaixinhaID          *string `json:"caixinhaId"`
	DisplayOrder        int     `json:"displayOrder"`
}

func (req destinationReq) toInput() DestinationInput {
	return DestinationInput{
		Reference: req.Reference, Name: req.Name, Kind: Kind(req.Kind), Mode: Mode(req.Mode),
		Percentage: req.Percentage, FixedAmount: req.FixedAmount, PresetSubcategoryID: req.PresetSubcategoryID,
		PresetPaymentMethod: req.PresetPaymentMethod, PresetDescription: req.PresetDescription,
		CaixinhaID: req.CaixinhaID, DisplayOrder: req.DisplayOrder,
	}
}

// CreateDestination — POST /api/income/destinations
func (h *Handler) CreateDestination(w http.ResponseWriter, r *http.Request) {
	var req destinationReq
	if !decode(w, r, &req) {
		return
	}
	d, err := h.svc.CreateDestination(r.Context(), req.toInput())
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": d.ID})
}

// UpdateDestination — PUT /api/income/destinations/{id}
func (h *Handler) UpdateDestination(w http.ResponseWriter, r *http.Request) {
	var req destinationReq
	if !decode(w, r, &req) {
		return
	}
	if _, err := h.svc.UpdateDestination(r.Context(), chi.URLParam(r, "id"), req.toInput()); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteDestination — DELETE /api/income/destinations/{id}
func (h *Handler) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteDestination(r.Context(), chi.URLParam(r, "id")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Materialização ──────────────────────────────────────────────────────────

type materializeReq struct {
	SubcategoryID  *string `json:"subcategoryId"`
	Amount         *int64  `json:"amount"`
	CompetenceDate *string `json:"competenceDate"`
	PaymentDate    *string `json:"paymentDate"`
	Description    *string `json:"description"`
	PaymentMethod  *string `json:"paymentMethod"`
}

// Materialize — POST /api/income/destinations/{id}/materialize
func (h *Handler) Materialize(w http.ResponseWriter, r *http.Request) {
	var req materializeReq
	// corpo é opcional (overrides); aceita vazio.
	_ = json.NewDecoder(r.Body).Decode(&req)
	res, err := h.svc.Materialize(r.Context(), chi.URLParam(r, "id"), MaterializeInput{
		SubcategoryID: req.SubcategoryID, Amount: req.Amount, CompetenceDate: req.CompetenceDate,
		PaymentDate: req.PaymentDate, Description: req.Description, PaymentMethod: req.PaymentMethod,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Undo — DELETE /api/income/destinations/{id}/materialize
func (h *Handler) Undo(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Undo(r.Context(), chi.URLParam(r, "id")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MaterializeAll — POST /api/income/plan/{reference}/materialize-all
func (h *Handler) MaterializeAll(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.MaterializeAll(r.Context(), chi.URLParam(r, "reference"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ─── Templates / copiar mês ──────────────────────────────────────────────────

// ListTemplates — GET /api/income/templates
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toTemplateViews(ts)})
}

type templateItemReq struct {
	Name                string  `json:"name"`
	Kind                string  `json:"kind"`
	Mode                string  `json:"mode"`
	Percentage          *int    `json:"percentage"`
	FixedAmount         *int64  `json:"fixedAmount"`
	PresetSubcategoryID *string `json:"presetSubcategoryId"`
	PresetPaymentMethod *string `json:"presetPaymentMethod"`
	PresetDescription   *string `json:"presetDescription"`
	CaixinhaID          *string `json:"caixinhaId"`
}

type createTemplateReq struct {
	Name  string            `json:"name"`
	Items []templateItemReq `json:"items"`
}

// CreateTemplate — POST /api/income/templates
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateReq
	if !decode(w, r, &req) {
		return
	}
	items := make([]TemplateItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = TemplateItem{
			Name: it.Name, Kind: Kind(it.Kind), Mode: Mode(it.Mode), Percentage: it.Percentage,
			FixedAmount: it.FixedAmount, PresetSubcategoryID: it.PresetSubcategoryID,
			PresetPaymentMethod: it.PresetPaymentMethod, PresetDescription: it.PresetDescription,
			CaixinhaID: it.CaixinhaID,
		}
	}
	t, err := h.svc.CreateTemplate(r.Context(), req.Name, items)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": t.ID})
}

type applyTemplateReq struct {
	TemplateID string `json:"templateId"`
}

// ApplyTemplate — POST /api/income/plan/{reference}/apply-template
func (h *Handler) ApplyTemplate(w http.ResponseWriter, r *http.Request) {
	var req applyTemplateReq
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.ApplyTemplate(r.Context(), chi.URLParam(r, "reference"), req.TemplateID); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CopyPrevious — POST /api/income/plan/{reference}/copy-previous
func (h *Handler) CopyPrevious(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.CopyPrevious(r.Context(), chi.URLParam(r, "reference")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── view de template ────────────────────────────────────────────────────────

type templateItemView struct {
	Name                string  `json:"name"`
	Kind                string  `json:"kind"`
	Mode                string  `json:"mode"`
	Percentage          *int    `json:"percentage"`
	FixedAmount         *int64  `json:"fixedAmount"`
	PresetSubcategoryID *string `json:"presetSubcategoryId"`
	PresetPaymentMethod *string `json:"presetPaymentMethod"`
	PresetDescription   *string `json:"presetDescription"`
	CaixinhaID          *string `json:"caixinhaId"`
}

type templateView struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Items []templateItemView `json:"items"`
}

func toTemplateViews(ts []Template) []templateView {
	out := make([]templateView, len(ts))
	for i, t := range ts {
		items := make([]templateItemView, len(t.Items))
		for j, it := range t.Items {
			items[j] = templateItemView{
				Name: it.Name, Kind: string(it.Kind), Mode: string(it.Mode), Percentage: it.Percentage,
				FixedAmount: it.FixedAmount, PresetSubcategoryID: it.PresetSubcategoryID,
				PresetPaymentMethod: it.PresetPaymentMethod, PresetDescription: it.PresetDescription,
				CaixinhaID: it.CaixinhaID,
			}
		}
		out[i] = templateView{ID: t.ID, Name: t.Name, Items: items}
	}
	return out
}
