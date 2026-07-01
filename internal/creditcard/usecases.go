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

// PayInvoiceInput é o payload para pagar uma fatura: marca as compras EM ABERTO dela como
// pagas (realizado) na data informada. Não há valor nem lançamento sintético — paga-se o
// saldo aberto inteiro do momento (Opção 1).
type PayInvoiceInput struct {
	CardID      string
	Reference   string
	PaymentDate string
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

type PayInvoiceUseCase interface {
	Execute(ctx context.Context, in PayInvoiceInput) (Invoice, error)
}

type UndoInvoicePaymentUseCase interface {
	// Desfaz o pagamento de uma data: volta as compras pagas naquela data para pendente.
	Execute(ctx context.Context, cardID, reference, paymentDate string) (Invoice, error)
}

type MonthlyCardSummaryUseCase interface {
	Execute(ctx context.Context, cardID string, year, month int) (MonthlyCardSummary, error)
}
