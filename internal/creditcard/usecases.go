package creditcard

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Tipos de aplicação (I/O dos use cases) ─────────────────────────────────

// CreditCardDetail é o cartão acrescido dos indicadores derivados (D10).
type CreditCardDetail struct {
	CreditCard
	BestPurchaseDay    int
	UsedLimit          int64
	AvailableLimit     int64
	UtilizationPercent int
	UtilizationLevel   UtilizationLevel
	OpenInvoice        *Invoice // fatura do ciclo corrente; nunca nil (pode estar zerada)
}

// ListInput é o filtro da listagem de cartões.
type ListInput struct {
	Archived   bool
	Pagination shared.Pagination
}

// ListCreditCardsResult carrega os cartões com indicadores e o meta de paginação.
type ListCreditCardsResult struct {
	Data       []CreditCardDetail
	Pagination shared.PagedMeta
}

// InvoiceDetail é a fatura com os lançamentos do ciclo paginados (D6).
type InvoiceDetail struct {
	Invoice
	Data       []shared.CardTransaction
	Pagination shared.PagedMeta
}

// AddInvoicePaymentInput é o payload para registrar UM pagamento de fatura (parcial ou
// total, em fatura aberta/fechada/vencida). Cria o lançamento de caixa na data e, se o
// pagamento quitar a fatura, realiza em lote as compras do ciclo.
type AddInvoicePaymentInput struct {
	CardID        string
	Reference     string
	Amount        int64 // valor do pagamento (centavos); ≤ saldo devedor
	PaymentDate   string
	SubcategoryID string  // subcategoria do lançamento de pagamento (default: transferência)
	Title         string  // opcional; default "Pagamento de Fatura — <reference>"
	Description   *string // opcional
}

// ─── Interfaces de use case ─────────────────────────────────────────────────

type CreateCreditCardUseCase interface {
	Execute(ctx context.Context, in CreateCreditCardInput) (CreditCard, error)
}

type GetCreditCardUseCase interface {
	Execute(ctx context.Context, id string) (CreditCardDetail, error)
}

type ListCreditCardsUseCase interface {
	Execute(ctx context.Context, in ListInput) (ListCreditCardsResult, error)
}

type UpdateCreditCardUseCase interface {
	Execute(ctx context.Context, in UpdateCreditCardInput) (CreditCard, error)
}

type DeleteCreditCardUseCase interface {
	Execute(ctx context.Context, id string) error
}

// ArchiveCreditCardUseCase cobre arquivar (archived=true) e desarquivar (false).
type ArchiveCreditCardUseCase interface {
	Execute(ctx context.Context, id string, archived bool) error
}

type ListInvoicesUseCase interface {
	Execute(ctx context.Context, cardID string) ([]Invoice, error)
}

type GetInvoiceUseCase interface {
	Execute(ctx context.Context, cardID, reference string, p shared.Pagination) (InvoiceDetail, error)
}

type AddInvoicePaymentUseCase interface {
	Execute(ctx context.Context, in AddInvoicePaymentInput) (Invoice, error)
}

type UndoInvoicePaymentUseCase interface {
	Execute(ctx context.Context, cardID, reference, paymentID string) (Invoice, error)
}

type MonthlyCardSummaryUseCase interface {
	Execute(ctx context.Context, cardID string, year, month int) (MonthlyCardSummary, error)
}
