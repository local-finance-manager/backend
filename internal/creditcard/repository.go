package creditcard

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CreditCardRepository persiste cartões.
type CreditCardRepository interface {
	Create(ctx context.Context, c CreditCard) error
	Get(ctx context.Context, id string) (CreditCard, error)
	// List filtra por estado de arquivamento: archived=false → só ativos (default),
	// archived=true → só arquivados. Não há visão "todos" (a UI tem abas separadas).
	List(ctx context.Context, archived bool, p shared.Pagination) ([]CreditCard, int, error)
	Update(ctx context.Context, c CreditCard) error
	Delete(ctx context.Context, id string) error
	SetArchived(ctx context.Context, id string, archived bool) error
}

// InvoicePaymentRepository persiste o status de pagamento de faturas fechadas
// (única parte da fatura armazenada — D4).
type InvoicePaymentRepository interface {
	Get(ctx context.Context, cardID, reference string) (*InvoicePayment, error)
	ListByCard(ctx context.Context, cardID string) (map[string]*InvoicePayment, error)
	Upsert(ctx context.Context, cardID string, p InvoicePayment) error
	Delete(ctx context.Context, cardID, reference string) error
}

// CardTransactionReader é o port pelo qual o creditcard lê lançamentos de cartão.
// Implementado por um facade no módulo transaction e injetado no main.go (D1).
type CardTransactionReader interface {
	// ListByCard retorna os lançamentos vinculados a um cartão cuja competence_date
	// esteja em [fromCompetence, toCompetence] (inclusive, YYYY-MM-DD).
	ListByCard(ctx context.Context, cardID, fromCompetence, toCompetence string) ([]shared.CardTransaction, error)
	// HasTransactions informa se há qualquer lançamento vinculado ao cartão.
	HasTransactions(ctx context.Context, cardID string) (bool, error)
}
