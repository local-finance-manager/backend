package creditcard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
)

var creditCardPaginationDefaults = shared.Pagination{
	Page: 1, Limit: 100, OrderBy: "created_at", Order: "DESC",
}

var creditCardAllowedOrderBy = []string{"name", "created_at"}

var invoicePaginationDefaults = shared.Pagination{
	Page: 1, Limit: 100, OrderBy: "competence_date", Order: "ASC",
}

// ─── HandlerDeps / Handler ──────────────────────────────────────────────────

// HandlerDeps reúne os use cases do módulo.
type HandlerDeps struct {
	Create       CreateCreditCardUseCase
	Get          GetCreditCardUseCase
	List         ListCreditCardsUseCase
	Update       UpdateCreditCardUseCase
	Delete       DeleteCreditCardUseCase
	Archive      ArchiveCreditCardUseCase
	ListInvoices ListInvoicesUseCase
	GetInvoice   GetInvoiceUseCase
	PayInvoice   PayInvoiceUseCase
	UndoPayment  UndoInvoicePaymentUseCase
	MonthSummary MonthlyCardSummaryUseCase
}

// Handler trata as requisições HTTP do módulo de cartão de crédito.
type Handler struct{ deps HandlerDeps }

// NewHandler cria um Handler.
func NewHandler(deps HandlerDeps) *Handler { return &Handler{deps: deps} }

// ─── Tipos de resposta (snake_case) ─────────────────────────────────────────

type cardResp struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Brand          string  `json:"brand"`
	LastFourDigits *string `json:"last_four_digits"`
	Issuer         *string `json:"issuer"`
	CreditLimit    int64   `json:"credit_limit"`
	ClosingDay     int     `json:"closing_day"`
	DueDay         int     `json:"due_day"`
	Color          *string `json:"color"`
	Icon           *string `json:"icon"`
	Archived       bool    `json:"archived"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type cardDetailResp struct {
	cardResp
	BestPurchaseDay    int          `json:"best_purchase_day"`
	UsedLimit          int64        `json:"used_limit"`
	AvailableLimit     int64        `json:"available_limit"`
	UtilizationPercent int          `json:"utilization_percent"`
	UtilizationLevel   string       `json:"utilization_level"`
	OpenInvoice        *invoiceResp `json:"open_invoice"`
}

type categoryBreakdownResp struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	Color        string `json:"color"`
	Total        int64  `json:"total"`
	Percent      int    `json:"percent"`
}

type paymentResp struct {
	PaymentDate string `json:"payment_date"`
	Amount      int64  `json:"amount"`
}

type invoiceResp struct {
	Reference         string                  `json:"reference"`
	CycleStart        string                  `json:"cycle_start"`
	ClosingDate       string                  `json:"closing_date"`
	DueDate           string                  `json:"due_date"`
	Status            string                  `json:"status"`
	Total             int64                   `json:"total"`
	PaidAmount        int64                   `json:"paid_amount"`
	OutstandingAmount int64                   `json:"outstanding_amount"`
	PaymentStatus     string                  `json:"payment_status"`
	Count             int                     `json:"count"`
	Payments          []paymentResp           `json:"payments"`
	CategoryBreakdown []categoryBreakdownResp `json:"category_breakdown"`
}

type cardTxnResp struct {
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	Amount             int64   `json:"amount"`
	CompetenceDate     string  `json:"competence_date"`
	PaymentDate        *string `json:"payment_date"`
	Status             string  `json:"status"`
	SubcategoryID      string  `json:"subcategory_id"`
	SubcategoryName    string  `json:"subcategory_name"`
	CategoryID         string  `json:"category_id"`
	CategoryName       string  `json:"category_name"`
	CategoryColor      string  `json:"category_color"`
	CreditCardID       string  `json:"credit_card_id"`
	InstallmentGroupID *string `json:"installment_group_id"`
	InstallmentNumber  *int    `json:"installment_number"`
	InstallmentTotal   *int    `json:"installment_total"`
}

type invoiceDetailResp struct {
	invoiceResp
	Data       []cardTxnResp    `json:"data"`
	Pagination shared.PagedMeta `json:"pagination"`
}

type monthlySummaryResp struct {
	CreditCardID      string                  `json:"credit_card_id"`
	Year              int                     `json:"year"`
	Month             int                     `json:"month"`
	Total             int64                   `json:"total"`
	Count             int                     `json:"count"`
	AverageTicket     int64                   `json:"average_ticket"`
	CategoryBreakdown []categoryBreakdownResp `json:"category_breakdown"`
}

// ─── Tipos de request ───────────────────────────────────────────────────────

type cardReq struct {
	Name           string  `json:"name"`
	Brand          string  `json:"brand"`
	LastFourDigits *string `json:"last_four_digits"`
	Issuer         *string `json:"issuer"`
	CreditLimit    int64   `json:"credit_limit"`
	ClosingDay     int     `json:"closing_day"`
	DueDay         int     `json:"due_day"`
	Color          *string `json:"color"`
	Icon           *string `json:"icon"`
}

type payInvoiceReq struct {
	PaymentDate string `json:"payment_date"`
}

// ─── Converters ─────────────────────────────────────────────────────────────

func toCardResp(c CreditCard) cardResp {
	return cardResp{
		ID: c.ID, Name: c.Name, Brand: string(c.Brand),
		LastFourDigits: c.LastFourDigits, Issuer: c.Issuer,
		CreditLimit: c.CreditLimit, ClosingDay: c.ClosingDay, DueDay: c.DueDay,
		Color: c.Color, Icon: c.Icon, Archived: c.Archived,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toBreakdownResp(bs []CategoryBreakdown) []categoryBreakdownResp {
	out := make([]categoryBreakdownResp, len(bs))
	for i, b := range bs {
		out[i] = categoryBreakdownResp{
			CategoryID: b.CategoryID, CategoryName: b.CategoryName,
			Color: b.Color, Total: b.Total, Percent: b.Percent,
		}
	}
	return out
}

func toInvoiceResp(inv Invoice) invoiceResp {
	payments := make([]paymentResp, len(inv.Payments))
	for i, p := range inv.Payments {
		payments[i] = paymentResp{PaymentDate: p.PaymentDate, Amount: p.Amount}
	}
	return invoiceResp{
		Reference: inv.Reference, CycleStart: inv.CycleStart,
		ClosingDate: inv.ClosingDate, DueDate: inv.DueDate,
		Status: string(inv.Status), Total: inv.Total,
		PaidAmount: inv.PaidAmount, OutstandingAmount: inv.OutstandingAmount,
		PaymentStatus: string(inv.PaymentStatus), Count: inv.Count,
		Payments: payments, CategoryBreakdown: toBreakdownResp(inv.CategoryBreakdown),
	}
}

func toCardDetailResp(d CreditCardDetail) cardDetailResp {
	var open *invoiceResp
	if d.OpenInvoice != nil {
		r := toInvoiceResp(*d.OpenInvoice)
		open = &r
	}
	return cardDetailResp{
		cardResp:           toCardResp(d.CreditCard),
		BestPurchaseDay:    d.BestPurchaseDay,
		UsedLimit:          d.UsedLimit,
		AvailableLimit:     d.AvailableLimit,
		UtilizationPercent: d.UtilizationPercent,
		UtilizationLevel:   string(d.UtilizationLevel),
		OpenInvoice:        open,
	}
}

func toCardTxnResp(t shared.CardTransaction) cardTxnResp {
	return cardTxnResp{
		ID: t.ID, Title: t.Title, Amount: t.Amount, CompetenceDate: t.CompetenceDate,
		PaymentDate: t.PaymentDate, Status: t.Status,
		SubcategoryID: t.SubcategoryID, SubcategoryName: t.SubcategoryName,
		CategoryID: t.CategoryID, CategoryName: t.CategoryName, CategoryColor: t.CategoryColor,
		CreditCardID:       t.CreditCardID,
		InstallmentGroupID: t.InstallmentGroupID,
		InstallmentNumber:  t.InstallmentNumber,
		InstallmentTotal:   t.InstallmentTotal,
	}
}

func toInput(req cardReq) (Brand, *string, *string, *string, *string) {
	return Brand(strings.ToLower(req.Brand)), req.LastFourDigits, req.Issuer, req.Color, req.Icon
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// ListCreditCards trata GET /api/credit-cards
func (h *Handler) ListCreditCards(w http.ResponseWriter, r *http.Request) {
	archived := strings.EqualFold(r.URL.Query().Get("archived"), "true")
	p := shared.ParsePagination(r, creditCardPaginationDefaults, creditCardAllowedOrderBy)

	res, err := h.deps.List.Execute(r.Context(), ListInput{Archived: archived, Pagination: p})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]cardDetailResp, len(res.Data))
	for i, d := range res.Data {
		data[i] = toCardDetailResp(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "pagination": res.Pagination})
}

// CreateCreditCard trata POST /api/credit-cards
func (h *Handler) CreateCreditCard(w http.ResponseWriter, r *http.Request) {
	var req cardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	brand, lastFour, issuer, color, icon := toInput(req)
	c, err := h.deps.Create.Execute(r.Context(), CreateCreditCardInput{
		Name: req.Name, Brand: brand, LastFourDigits: lastFour, Issuer: issuer,
		CreditLimit: req.CreditLimit, ClosingDay: req.ClosingDay, DueDay: req.DueDay,
		Color: color, Icon: icon,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCardResp(c))
}

// GetCreditCard trata GET /api/credit-cards/{id}
func (h *Handler) GetCreditCard(w http.ResponseWriter, r *http.Request) {
	d, err := h.deps.Get.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCardDetailResp(d))
}

// UpdateCreditCard trata PUT /api/credit-cards/{id}
func (h *Handler) UpdateCreditCard(w http.ResponseWriter, r *http.Request) {
	var req cardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	brand, lastFour, issuer, color, icon := toInput(req)
	c, err := h.deps.Update.Execute(r.Context(), UpdateCreditCardInput{
		ID: chi.URLParam(r, "id"), Name: req.Name, Brand: brand,
		LastFourDigits: lastFour, Issuer: issuer, CreditLimit: req.CreditLimit,
		ClosingDay: req.ClosingDay, DueDay: req.DueDay, Color: color, Icon: icon,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCardResp(c))
}

// DeleteCreditCard trata DELETE /api/credit-cards/{id}
func (h *Handler) DeleteCreditCard(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Delete.Execute(r.Context(), chi.URLParam(r, "id")); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ArchiveCreditCard trata PATCH /api/credit-cards/{id}/archive
func (h *Handler) ArchiveCreditCard(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Archive.Execute(r.Context(), chi.URLParam(r, "id"), true); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnarchiveCreditCard trata PATCH /api/credit-cards/{id}/unarchive
func (h *Handler) UnarchiveCreditCard(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Archive.Execute(r.Context(), chi.URLParam(r, "id"), false); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListInvoices trata GET /api/credit-cards/{id}/invoices
func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	invs, err := h.deps.ListInvoices.Execute(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]invoiceResp, len(invs))
	for i, inv := range invs {
		data[i] = toInvoiceResp(inv)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetInvoice trata GET /api/credit-cards/{id}/invoices/{reference}
func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	p := shared.ParsePagination(r, invoicePaginationDefaults, []string{"competence_date"})
	det, err := h.deps.GetInvoice.Execute(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "reference"), p)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]cardTxnResp, len(det.Data))
	for i, t := range det.Data {
		data[i] = toCardTxnResp(t)
	}
	writeJSON(w, http.StatusOK, invoiceDetailResp{
		invoiceResp: toInvoiceResp(det.Invoice),
		Data:        data,
		Pagination:  det.Pagination,
	})
}

// PayInvoice trata POST /api/credit-cards/{id}/invoices/{reference}/pay
func (h *Handler) PayInvoice(w http.ResponseWriter, r *http.Request) {
	var req payInvoiceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest("corpo da requisição inválido", domainerr.WithDisplayable()))
		return
	}
	inv, err := h.deps.PayInvoice.Execute(r.Context(), PayInvoiceInput{
		CardID:      chi.URLParam(r, "id"),
		Reference:   chi.URLParam(r, "reference"),
		PaymentDate: req.PaymentDate,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvoiceResp(inv))
}

// UndoInvoicePayment trata DELETE /api/credit-cards/{id}/invoices/{reference}/payments/{paymentDate}
func (h *Handler) UndoInvoicePayment(w http.ResponseWriter, r *http.Request) {
	inv, err := h.deps.UndoPayment.Execute(r.Context(),
		chi.URLParam(r, "id"), chi.URLParam(r, "reference"), chi.URLParam(r, "paymentDate"))
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInvoiceResp(inv))
}

// CardSummary trata GET /api/credit-cards/{id}/summary?year=&month= (mensal, ambos obrigatórios)
func (h *Handler) CardSummary(w http.ResponseWriter, r *http.Request) {
	year, errY := strconv.Atoi(r.URL.Query().Get("year"))
	month, errM := strconv.Atoi(r.URL.Query().Get("month"))
	if errY != nil || year < 1 {
		domainerr.WriteError(w, domainerr.NewBadRequest("year é obrigatório", domainerr.WithDisplayable()))
		return
	}
	if errM != nil || month < 1 || month > 12 {
		domainerr.WriteError(w, domainerr.NewBadRequest("month é obrigatório e deve estar entre 1 e 12", domainerr.WithDisplayable()))
		return
	}
	s, err := h.deps.MonthSummary.Execute(r.Context(), chi.URLParam(r, "id"), year, month)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, monthlySummaryResp{
		CreditCardID: s.CreditCardID, Year: s.Year, Month: s.Month,
		Total: s.Total, Count: s.Count, AverageTicket: s.AverageTicket,
		CategoryBreakdown: toBreakdownResp(s.CategoryBreakdown),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
