package transaction

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Pagination config ────────────────────────────────────────────────────────

var transactionPaginationDefaults = shared.Pagination{
	Page:    1,
	Limit:   50,
	OrderBy: "competence_date",
	Order:   "DESC",
}

var transactionAllowedOrderBy = []string{
	"competence_date", "payment_date", "amount", "created_at", "title",
}

// ─── HandlerDeps / Handler ────────────────────────────────────────────────────

// HandlerDeps holds all use case dependencies for the transaction handler.
type HandlerDeps struct {
	GetTransaction    GetTransactionUseCase
	ListTransactions  ListTransactionsUseCase
	CreateTransaction CreateTransactionUseCase
	UpdateTransaction UpdateTransactionUseCase
	ConfirmTransaction ConfirmTransactionUseCase
	DeleteTransaction DeleteTransactionUseCase
}

// Handler handles HTTP requests for the transaction module.
type Handler struct{ deps HandlerDeps }

// NewHandler creates a new transaction Handler.
func NewHandler(deps HandlerDeps) *Handler {
	return &Handler{deps: deps}
}

// ─── Response types ───────────────────────────────────────────────────────────

type categoryInfoResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type subcategoryInfoResp struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Icon     string           `json:"icon,omitempty"`
	Color    string           `json:"color,omitempty"`
	Category categoryInfoResp `json:"category"`
}

type transactionDetailResp struct {
	ID                   string              `json:"id"`
	Title                string              `json:"title"`
	Description          *string             `json:"description"`
	Amount               int64               `json:"amount"`
	Type                 string              `json:"type"`
	PaymentMethod        string              `json:"payment_method"`
	Status               string              `json:"status"`
	CompetenceDate       string              `json:"competence_date"`
	PaymentDate          *string             `json:"payment_date"`
	AccountID            *string             `json:"account_id"`
	DestinationAccountID *string             `json:"destination_account_id"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
	Subcategory          subcategoryInfoResp `json:"subcategory"`
}

type listTransactionsResp struct {
	Data       []transactionDetailResp `json:"data"`
	Summary    Summary                 `json:"summary"`
	Pagination shared.PagedMeta        `json:"pagination"`
}

// ─── Request types ────────────────────────────────────────────────────────────

type createTransactionReq struct {
	Title                string  `json:"title"`
	Description          *string `json:"description"`
	Amount               int64   `json:"amount"`
	SubcategoryID        string  `json:"subcategory_id"`
	PaymentMethod        string  `json:"payment_method"`
	Status               string  `json:"status"`
	CompetenceDate       string  `json:"competence_date"`
	PaymentDate          *string `json:"payment_date"`
	AccountID            *string `json:"account_id"`
	DestinationAccountID *string `json:"destination_account_id"`
}

type updateTransactionReq struct {
	Title                string  `json:"title"`
	Description          *string `json:"description"`
	Amount               int64   `json:"amount"`
	SubcategoryID        string  `json:"subcategory_id"`
	PaymentMethod        string  `json:"payment_method"`
	Status               string  `json:"status"`
	CompetenceDate       string  `json:"competence_date"`
	PaymentDate          *string `json:"payment_date"`
	AccountID            *string `json:"account_id"`
	DestinationAccountID *string `json:"destination_account_id"`
}

type confirmTransactionReq struct {
	PaymentDate string `json:"payment_date"`
}

// ─── Converter ────────────────────────────────────────────────────────────────

func toDetailResp(d TransactionDetail) transactionDetailResp {
	return transactionDetailResp{
		ID:                   d.ID,
		Title:                d.Title,
		Description:          d.Description,
		Amount:               d.Amount,
		Type:                 string(d.Type),
		PaymentMethod:        string(d.PaymentMethod),
		Status:               string(d.Status),
		CompetenceDate:       d.CompetenceDate,
		PaymentDate:          d.PaymentDate,
		AccountID:            d.AccountID,
		DestinationAccountID: d.DestinationAccountID,
		CreatedAt:            d.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            d.UpdatedAt.UTC().Format(time.RFC3339),
		Subcategory: subcategoryInfoResp{
			ID:    d.Subcategory.ID,
			Name:  d.Subcategory.Name,
			Icon:  d.Subcategory.Icon,
			Color: d.Subcategory.Color,
			Category: categoryInfoResp{
				ID:    d.Subcategory.Category.ID,
				Name:  d.Subcategory.Category.Name,
				Icon:  d.Subcategory.Category.Icon,
				Color: d.Subcategory.Category.Color,
			},
		},
	}
}

// ─── Filter parser ────────────────────────────────────────────────────────────

// parseTransactionFilter reads explicit filter params from the query string.
// It does NOT use shared.Pagination date fields — transactions have their own date params.
func parseTransactionFilter(r *http.Request) TransactionFilter {
	q := r.URL.Query()
	var f TransactionFilter

	if v := q.Get("type"); v != "" {
		t := TransactionType(strings.ToLower(v))
		f.Type = &t
	}
	if v := q.Get("status"); v != "" {
		s := TransactionStatus(strings.ToLower(v))
		f.Status = &s
	}
	if v := q.Get("payment_method"); v != "" {
		pm := PaymentMethod(strings.ToLower(v))
		f.PaymentMethod = &pm
	}
	if v := q.Get("subcategory_id"); v != "" {
		f.SubcategoryID = &v
	}
	if v := q.Get("category_id"); v != "" {
		f.CategoryID = &v
	}
	if v := q.Get("account_id"); v != "" {
		f.AccountID = &v
	}
	if v := q.Get("competence_date_from"); v != "" {
		f.CompetenceDateFrom = &v
	}
	if v := q.Get("competence_date_to"); v != "" {
		f.CompetenceDateTo = &v
	}
	if v := q.Get("payment_date_from"); v != "" {
		f.PaymentDateFrom = &v
	}
	if v := q.Get("payment_date_to"); v != "" {
		f.PaymentDateTo = &v
	}
	if v := q.Get("search"); v != "" {
		f.Search = &v
	}

	return f
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// ListTransactions handles GET /api/transactions
func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	filter := parseTransactionFilter(r)
	p := shared.ParsePagination(r, transactionPaginationDefaults, transactionAllowedOrderBy)

	result, err := h.deps.ListTransactions.Execute(r.Context(), ListTransactionsInput{
		Filter:     filter,
		Pagination: p,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}

	data := make([]transactionDetailResp, len(result.Data))
	for i, d := range result.Data {
		data[i] = toDetailResp(d)
	}
	writeJSON(w, http.StatusOK, listTransactionsResp{
		Data:       data,
		Summary:    result.Summary,
		Pagination: result.Pagination,
	})
}

// GetTransaction handles GET /api/transactions/{id}
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := h.deps.GetTransaction.Execute(r.Context(), id)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDetailResp(d))
}

// CreateTransaction handles POST /api/transactions
func (h *Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req createTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}

	in := CreateTransactionInput{
		Title:                req.Title,
		Description:          req.Description,
		Amount:               req.Amount,
		SubcategoryID:        req.SubcategoryID,
		PaymentMethod:        PaymentMethod(strings.ToLower(req.PaymentMethod)),
		Status:               TransactionStatus(strings.ToLower(req.Status)),
		CompetenceDate:       req.CompetenceDate,
		PaymentDate:          req.PaymentDate,
		AccountID:            req.AccountID,
		DestinationAccountID: req.DestinationAccountID,
	}

	d, err := h.deps.CreateTransaction.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDetailResp(d))
}

// UpdateTransaction handles PUT /api/transactions/{id}
func (h *Handler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req updateTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}

	in := UpdateTransactionInput{
		ID:                   id,
		Title:                req.Title,
		Description:          req.Description,
		Amount:               req.Amount,
		SubcategoryID:        req.SubcategoryID,
		PaymentMethod:        PaymentMethod(strings.ToLower(req.PaymentMethod)),
		Status:               TransactionStatus(strings.ToLower(req.Status)),
		CompetenceDate:       req.CompetenceDate,
		PaymentDate:          req.PaymentDate,
		AccountID:            req.AccountID,
		DestinationAccountID: req.DestinationAccountID,
	}

	d, err := h.deps.UpdateTransaction.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDetailResp(d))
}

// ConfirmTransaction handles PATCH /api/transactions/{id}/confirm
func (h *Handler) ConfirmTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req confirmTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}

	d, err := h.deps.ConfirmTransaction.Execute(r.Context(), ConfirmTransactionInput{
		ID:          id,
		PaymentDate: req.PaymentDate,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDetailResp(d))
}

// DeleteTransaction handles DELETE /api/transactions/{id}
func (h *Handler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.deps.DeleteTransaction.Execute(r.Context(), id); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
