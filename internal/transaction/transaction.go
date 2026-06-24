package transaction

import (
	"fmt"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Tipos / Enums ────────────────────────────────────────────────────────────

// TransactionType classifies the financial direction of a transaction.
type TransactionType string

const (
	TypeDespesa       TransactionType = "despesa"
	TypeReceita       TransactionType = "receita"
	TypeTransferencia TransactionType = "transferencia"
)

// PaymentMethod identifies how a transaction was settled.
type PaymentMethod string

const (
	MethodPix           PaymentMethod = "pix"
	MethodCartaoCredito PaymentMethod = "cartao_credito"
	MethodCartaoDebito  PaymentMethod = "cartao_debito"
	MethodDinheiro      PaymentMethod = "dinheiro"
	MethodTed           PaymentMethod = "ted"
	MethodBoleto        PaymentMethod = "boleto"
	MethodOutros        PaymentMethod = "outros"
)

var validPaymentMethods = map[PaymentMethod]struct{}{
	MethodPix: {}, MethodCartaoCredito: {}, MethodCartaoDebito: {},
	MethodDinheiro: {}, MethodTed: {}, MethodBoleto: {}, MethodOutros: {},
}

// TransactionStatus tracks the lifecycle state of a transaction.
type TransactionStatus string

const (
	StatusPendente  TransactionStatus = "pendente"
	StatusRealizado TransactionStatus = "realizado"
	StatusCancelado TransactionStatus = "cancelado"
)

var validStatuses = map[TransactionStatus]struct{}{
	StatusPendente: {}, StatusRealizado: {}, StatusCancelado: {},
}

// validTransitions defines allowed status transitions.
// cancelado → realizado is PROHIBITED; must go through pendente first.
var validTransitions = map[TransactionStatus]map[TransactionStatus]bool{
	StatusPendente:  {StatusRealizado: true, StatusCancelado: true},
	StatusRealizado: {StatusPendente: true, StatusCancelado: true},
	StatusCancelado: {StatusPendente: true},
}

// ─── Entidade ─────────────────────────────────────────────────────────────────

// Transaction is the core domain entity.
type Transaction struct {
	ID                   string
	Title                string
	Description          *string
	Amount               int64 // centavos; always positive
	Type                 TransactionType
	SubcategoryID        string
	PaymentMethod        PaymentMethod
	Status               TransactionStatus
	CompetenceDate       string  // YYYY-MM-DD; business date
	PaymentDate          *string // YYYY-MM-DD; required when status=realizado
	AccountID            *string // nullable v1
	DestinationAccountID *string // nullable v1
	CreditCardID         *string // nullable; só quando paymentMethod=cartao_credito
	InstallmentGroupID   *string // nullable; preenchido quando é parcela de uma compra parcelada
	InstallmentNumber    *int    // k (1-based), nullable
	InstallmentTotal     *int    // N, nullable
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ─── Tipos de resposta ────────────────────────────────────────────────────────

// SubcategoryInfo is the transaction module's view of a subcategory.
// Defined at the consumer site to avoid importing internal/category.
type SubcategoryInfo struct {
	ID       string
	Name     string
	Icon     string
	Color    string
	Category CategoryInfo
}

// CategoryInfo is the transaction module's view of a parent category.
type CategoryInfo struct {
	ID    string
	Name  string
	Icon  string
	Color string
}

// TransactionDetail is the full projection returned by Get, Create, Update, Confirm.
type TransactionDetail struct {
	Transaction
	Subcategory SubcategoryInfo
}

// ─── Sumário financeiro ───────────────────────────────────────────────────────

// Summary aggregates financial totals for a filtered set of transactions.
type Summary struct {
	TotalDespesas int64 `json:"totalDespesas"` // realizado + despesa
	TotalReceitas int64 `json:"totalReceitas"` // realizado + receita
	SaldoPeriodo  int64 `json:"saldoPeriodo"`  // totalReceitas - totalDespesas
	TotalPendente int64 `json:"totalPendente"` // pendente (any type)
	CountTotal    int   `json:"countTotal"`    // total records matching the filter
}

// ─── Filtros ──────────────────────────────────────────────────────────────────

// TransactionFilter holds optional filter criteria for list/summary queries.
type TransactionFilter struct {
	Type               *TransactionType
	Status             *TransactionStatus
	PaymentMethod      *PaymentMethod
	SubcategoryID      *string
	CategoryID         *string // filters all transactions of subcategories in this category
	AccountID          *string
	CompetenceDateFrom *string // YYYY-MM-DD inclusive
	CompetenceDateTo   *string // YYYY-MM-DD inclusive
	PaymentDateFrom    *string // YYYY-MM-DD inclusive
	PaymentDateTo      *string // YYYY-MM-DD inclusive
	Search             *string // LOWER(title) LIKE '%search%'
	CreditCardID       *string // filtra lançamentos vinculados a um cartão
	InstallmentGroupID *string // filtra as parcelas de uma compra parcelada
}

// ─── Inputs de listagem ───────────────────────────────────────────────────────

// ListTransactionsInput is the input for the list use case.
type ListTransactionsInput struct {
	Filter     TransactionFilter
	Pagination shared.Pagination
}

// ListTransactionsResult is the response shape for the list endpoint.
// Not a PagedResult[T] because it carries the financial summary alongside data.
type ListTransactionsResult struct {
	Data       []TransactionDetail `json:"data"`
	Summary    Summary             `json:"summary"`
	Pagination shared.PagedMeta    `json:"pagination"`
}

// ─── Inputs de mutação ────────────────────────────────────────────────────────

// CreateTransactionInput carries all fields required to create a transaction.
type CreateTransactionInput struct {
	Title                string
	Description          *string
	Amount               int64
	SubcategoryID        string
	PaymentMethod        PaymentMethod
	Status               TransactionStatus
	CompetenceDate       string  // YYYY-MM-DD
	PaymentDate          *string // YYYY-MM-DD; required when status=realizado
	AccountID            *string
	DestinationAccountID *string
	CreditCardID         *string
}

// UpdateTransactionInput carries all mutable fields for a PUT full-replace.
type UpdateTransactionInput struct {
	ID                   string
	Title                string
	Description          *string
	Amount               int64
	SubcategoryID        string
	PaymentMethod        PaymentMethod
	Status               TransactionStatus
	CompetenceDate       string
	PaymentDate          *string
	AccountID            *string
	DestinationAccountID *string
	CreditCardID         *string
}

// ConfirmTransactionInput is the minimal body for PATCH .../confirm.
type ConfirmTransactionInput struct {
	ID          string
	PaymentDate string // YYYY-MM-DD
}

// ─── Erros de domínio ─────────────────────────────────────────────────────────

var (
	ErrTransactionNotFound = domainerr.NewNotFound(
		"lançamento não encontrado", domainerr.WithDisplayable())
	ErrTypeChangeForbidden = domainerr.NewBadRequest(
		"não é possível alterar o tipo de um lançamento existente", domainerr.WithDisplayable())
)

// ErrInvalidTransition returns a displayable error for a forbidden status change.
func ErrInvalidTransition(from, to TransactionStatus) error {
	return domainerr.NewBadRequest(
		fmt.Sprintf("transição de status inválida: %s → %s", from, to),
		domainerr.WithDisplayable(),
	)
}

// ─── Regras de domínio ────────────────────────────────────────────────────────

// CanTransitionTo reports whether the from→to status transition is allowed.
func CanTransitionTo(from, to TransactionStatus) bool {
	return validTransitions[from][to]
}

// ─── Validação ────────────────────────────────────────────────────────────────

const (
	maxTitleLen       = 150
	maxDescriptionLen = 1000
)

// ValidateCreate validates a CreateTransactionInput using an error accumulator.
// All broken rules are returned at once — never fail-fast.
func ValidateCreate(in CreateTransactionInput) error {
	title := strings.TrimSpace(in.Title)
	_, pmOK := validPaymentMethods[in.PaymentMethod]
	_, stOK := validStatuses[in.Status]

	acc := validation.NewAccumulator().
		Check(title != "", "título é obrigatório").
		Check(len([]rune(title)) <= maxTitleLen, "título deve ter no máximo 150 caracteres").
		Check(in.Amount > 0, "valor deve ser maior que zero").
		Check(pmOK, "forma de pagamento inválida").
		Check(stOK, "status inválido: use pendente, realizado ou cancelado").
		Check(in.SubcategoryID != "", "subcategoryId é obrigatório").
		Check(in.CompetenceDate != "", "data de competência é obrigatória").
		Check(in.CreditCardID == nil || in.PaymentMethod == MethodCartaoCredito,
			"cartão de crédito só pode ser vinculado a lançamentos com forma de pagamento cartão de crédito")

	if in.CompetenceDate != "" {
		acc.Check(isValidDate(in.CompetenceDate), "data de competência inválida: use YYYY-MM-DD")
	}
	if in.Description != nil {
		acc.Check(len([]rune(*in.Description)) <= maxDescriptionLen,
			"descrição deve ter no máximo 1.000 caracteres")
	}

	// paymentDate rules depend on status — conditional checks per guia §5.1.
	if in.Status == StatusRealizado {
		acc.Check(in.PaymentDate != nil && *in.PaymentDate != "",
			"data de pagamento é obrigatória para lançamentos realizados")
		if in.PaymentDate != nil && *in.PaymentDate != "" {
			acc.Check(isValidDate(*in.PaymentDate), "data de pagamento inválida: use YYYY-MM-DD")
		}
	} else {
		acc.Check(in.PaymentDate == nil,
			"data de pagamento deve ser nula para lançamentos pendentes ou cancelados")
	}

	return acc.Result()
}

// ValidateUpdate applies the same format and paymentDate×status rules as ValidateCreate.
// Status transition logic is handled in the use case (requires current state).
func ValidateUpdate(in UpdateTransactionInput) error {
	title := strings.TrimSpace(in.Title)
	_, pmOK := validPaymentMethods[in.PaymentMethod]
	_, stOK := validStatuses[in.Status]

	acc := validation.NewAccumulator().
		Check(title != "", "título é obrigatório").
		Check(len([]rune(title)) <= maxTitleLen, "título deve ter no máximo 150 caracteres").
		Check(in.Amount > 0, "valor deve ser maior que zero").
		Check(pmOK, "forma de pagamento inválida").
		Check(stOK, "status inválido: use pendente, realizado ou cancelado").
		Check(in.SubcategoryID != "", "subcategoryId é obrigatório").
		Check(in.CompetenceDate != "", "data de competência é obrigatória").
		Check(in.CreditCardID == nil || in.PaymentMethod == MethodCartaoCredito,
			"cartão de crédito só pode ser vinculado a lançamentos com forma de pagamento cartão de crédito")

	if in.CompetenceDate != "" {
		acc.Check(isValidDate(in.CompetenceDate), "data de competência inválida: use YYYY-MM-DD")
	}
	if in.Description != nil {
		acc.Check(len([]rune(*in.Description)) <= maxDescriptionLen,
			"descrição deve ter no máximo 1.000 caracteres")
	}
	if in.Status == StatusRealizado {
		acc.Check(in.PaymentDate != nil && *in.PaymentDate != "",
			"data de pagamento é obrigatória para lançamentos realizados")
		if in.PaymentDate != nil && *in.PaymentDate != "" {
			acc.Check(isValidDate(*in.PaymentDate), "data de pagamento inválida: use YYYY-MM-DD")
		}
	} else {
		acc.Check(in.PaymentDate == nil,
			"data de pagamento deve ser nula para lançamentos pendentes ou cancelados")
	}

	return acc.Result()
}

// ValidateConfirm validates the minimal payload for the confirm endpoint.
func ValidateConfirm(in ConfirmTransactionInput) error {
	acc := validation.NewAccumulator().
		Check(in.PaymentDate != "", "data de pagamento é obrigatória")
	if in.PaymentDate != "" {
		acc.Check(isValidDate(in.PaymentDate), "data de pagamento inválida: use YYYY-MM-DD")
	}
	return acc.Result()
}

// isValidDate returns true if s is a valid YYYY-MM-DD date string.
func isValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
