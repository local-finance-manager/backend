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

// InvoicePaymentWriter marca/reverte as COMPRAS da fatura como pagas (não há lançamento
// sintético — Opção 1). Pagar uma fatura = marcar suas compras em aberto como `realizado`
// com a data de pagamento; desfazer = voltar para `pendente`. Escreve na tabela
// `transactions` (posse do módulo transaction) — exceção consciente de posse (Opção A,
// igual ao installment), pois a operação é só de status/data.
type InvoicePaymentWriter interface {
	// MarkInvoicePaid marca as compras (ids) como realizado com a data informada.
	MarkInvoicePaid(ctx context.Context, ids []string, paymentDate string) error
	// RevertInvoicePayment volta as compras (ids) para pendente, limpando a data.
	RevertInvoicePayment(ctx context.Context, ids []string) error
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
