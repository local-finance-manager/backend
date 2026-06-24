package installment

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
)

var installmentPaginationDefaults = shared.Pagination{
	Page: 1, Limit: 100, OrderBy: "created_at", Order: "DESC",
}

var installmentAllowedOrderBy = []string{"purchase_date", "created_at", "total_amount"}

// Handler trata as requisições HTTP de compras parceladas.
type Handler struct{ svc *Service }

// NewHandler cria o Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── Request types (snake_case) ─────────────────────────────────────────────

type createReq struct {
	CreditCardID      string  `json:"credit_card_id"`
	SubcategoryID     string  `json:"subcategory_id"`
	Title             string  `json:"title"`
	Description       *string `json:"description"`
	InstallmentsCount int     `json:"installments_count"`
	InputMode         string  `json:"input_mode"`
	TotalAmount       int64   `json:"total_amount"`
	InstallmentAmount int64   `json:"installment_amount"`
	PrincipalAmount   *int64  `json:"principal_amount"`
	PurchaseDate      string  `json:"purchase_date"`
}

type updateSeriesReq struct {
	Title         string  `json:"title"`
	Description   *string `json:"description"`
	SubcategoryID string  `json:"subcategory_id"`
	// Campos imutáveis pela série (RF-PARC-07): se vierem preenchidos, erro claro.
	InstallmentsCount int    `json:"installments_count"`
	TotalAmount       int64  `json:"total_amount"`
	PurchaseDate      string `json:"purchase_date"`
}

// ─── Response types (snake_case) ────────────────────────────────────────────

type plannedResp struct {
	Number         int    `json:"number"`
	Amount         int64  `json:"amount"`
	CompetenceDate string `json:"competence_date"`
	Reference      string `json:"reference"`
}

type planResp struct {
	TotalAmount       int64         `json:"total_amount"`
	InstallmentsCount int           `json:"installments_count"`
	InterestAmount    int64         `json:"interest_amount"`
	Installments      []plannedResp `json:"installments"`
}

type installmentResp struct {
	TransactionID  string `json:"transaction_id"`
	Number         int    `json:"number"`
	Amount         int64  `json:"amount"`
	CompetenceDate string `json:"competence_date"`
	Reference      string `json:"reference"`
	Status         string `json:"status"`
}

type groupDetailResp struct {
	ID                string            `json:"id"`
	CreditCardID      string            `json:"credit_card_id"`
	SubcategoryID     string            `json:"subcategory_id"`
	Title             string            `json:"title"`
	Description       *string           `json:"description"`
	TotalAmount       int64             `json:"total_amount"`
	PrincipalAmount   *int64            `json:"principal_amount"`
	InterestAmount    int64             `json:"interest_amount"`
	InstallmentsCount int               `json:"installments_count"`
	PurchaseDate      string            `json:"purchase_date"`
	FirstReference    string            `json:"first_reference"`
	PaidCount         int               `json:"paid_count"`
	RemainingCount    int               `json:"remaining_count"`
	RemainingAmount   int64             `json:"remaining_amount"`
	Status            string            `json:"status"`
	Installments      []installmentResp `json:"installments"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
}

type groupSummaryResp struct {
	ID                string `json:"id"`
	CreditCardID      string `json:"credit_card_id"`
	Title             string `json:"title"`
	TotalAmount       int64  `json:"total_amount"`
	InstallmentsCount int    `json:"installments_count"`
	PaidCount         int    `json:"paid_count"`
	RemainingCount    int    `json:"remaining_count"`
	RemainingAmount   int64  `json:"remaining_amount"`
	Status            string `json:"status"`
	PurchaseDate      string `json:"purchase_date"`
}

// ─── Converters ─────────────────────────────────────────────────────────────

func toCreateInput(req createReq) CreateInput {
	return CreateInput{
		CreditCardID:      req.CreditCardID,
		SubcategoryID:     req.SubcategoryID,
		Title:             req.Title,
		Description:       req.Description,
		InstallmentsCount: req.InstallmentsCount,
		InputMode:         InputMode(req.InputMode),
		TotalAmount:       req.TotalAmount,
		InstallmentAmount: req.InstallmentAmount,
		PrincipalAmount:   req.PrincipalAmount,
		PurchaseDate:      req.PurchaseDate,
	}
}

func toPlanResp(p Plan) planResp {
	items := make([]plannedResp, len(p.Installments))
	for i, pl := range p.Installments {
		items[i] = plannedResp{Number: pl.Number, Amount: pl.Amount, CompetenceDate: pl.CompetenceDate, Reference: pl.Reference}
	}
	return planResp{TotalAmount: p.TotalAmount, InstallmentsCount: p.InstallmentsCount, InterestAmount: p.InterestAmount, Installments: items}
}

func toGroupDetailResp(d GroupDetail) groupDetailResp {
	items := make([]installmentResp, len(d.Installments))
	for i, p := range d.Installments {
		items[i] = installmentResp{
			TransactionID: p.TransactionID, Number: p.Number, Amount: p.Amount,
			CompetenceDate: p.CompetenceDate, Reference: p.Reference, Status: p.Status,
		}
	}
	return groupDetailResp{
		ID: d.ID, CreditCardID: d.CreditCardID, SubcategoryID: d.SubcategoryID,
		Title: d.Title, Description: d.Description, TotalAmount: d.TotalAmount,
		PrincipalAmount: d.PrincipalAmount, InterestAmount: d.InterestAmount,
		InstallmentsCount: d.InstallmentsCount, PurchaseDate: d.PurchaseDate, FirstReference: d.FirstReference,
		PaidCount: d.PaidCount, RemainingCount: d.RemainingCount, RemainingAmount: d.RemainingAmount,
		Status:       string(d.Status),
		Installments: items,
		CreatedAt:    d.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    d.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toGroupSummaryResp(s GroupSummary) groupSummaryResp {
	return groupSummaryResp{
		ID: s.Group.ID, CreditCardID: s.Group.CreditCardID, Title: s.Group.Title,
		TotalAmount: s.Group.TotalAmount, InstallmentsCount: s.Group.InstallmentsCount,
		PaidCount: s.PaidCount, RemainingCount: s.RemainingCount, RemainingAmount: s.RemainingAmount,
		Status: string(s.Status), PurchaseDate: s.Group.PurchaseDate,
	}
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// Preview trata POST /api/installments/preview
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	plan, err := h.svc.Preview(r.Context(), toCreateInput(req))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlanResp(plan))
}

// Create trata POST /api/installments
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	d, err := h.svc.Create(r.Context(), toCreateInput(req))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGroupDetailResp(d))
}

// List trata GET /api/installments
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := shared.ParsePagination(r, installmentPaginationDefaults, installmentAllowedOrderBy)
	f := parseFilter(r)
	res, err := h.svc.List(r.Context(), f, p)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]groupSummaryResp, len(res.Data))
	for i, s := range res.Data {
		data[i] = toGroupSummaryResp(s)
	}
	writeJSON(w, http.StatusOK, shared.PagedResult[groupSummaryResp]{Data: data, Pagination: res.Pagination})
}

// Get trata GET /api/installments/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupDetailResp(d))
}

// UpdateSeries trata PUT /api/installments/{id}
func (h *Handler) UpdateSeries(w http.ResponseWriter, r *http.Request) {
	var req updateSeriesReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	// RF-PARC-07: valor/parcelas/data não são editáveis pela série.
	if req.InstallmentsCount != 0 || req.TotalAmount != 0 || req.PurchaseDate != "" {
		domainerr.WriteError(w, ErrImmutableSeriesField)
		return
	}
	d, err := h.svc.UpdateSeries(r.Context(), UpdateSeriesInput{
		ID: chi.URLParam(r, "id"), Title: req.Title, Description: req.Description, SubcategoryID: req.SubcategoryID,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupDetailResp(d))
}

// CancelRemaining trata PATCH /api/installments/{id}/cancel-remaining
func (h *Handler) CancelRemaining(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.CancelRemaining(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroupDetailResp(d))
}

// Delete trata DELETE /api/installments/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseFilter(r *http.Request) Filter {
	var f Filter
	if v := r.URL.Query().Get("credit_card_id"); v != "" {
		f.CreditCardID = &v
	}
	if v := r.URL.Query().Get("status"); v != "" {
		st := GroupStatus(v)
		if st == GroupActive || st == GroupSettled || st == GroupCancelled {
			f.Status = &st
		}
	}
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
