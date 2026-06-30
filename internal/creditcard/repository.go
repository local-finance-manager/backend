package creditcard

import (
	"context"
	"time"

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

// InvoicePaymentRepository persiste o LEDGER de pagamentos de fatura (única parte da
// fatura armazenada — D4). Cada fatura tem 0..N pagamentos (parcial/antecipado).
type InvoicePaymentRepository interface {
	// ListByCard devolve os pagamentos do cartão agrupados por reference de fatura.
	ListByCard(ctx context.Context, cardID string) (map[string][]InvoicePayment, error)
	// AddPaymentAtomic registra um pagamento numa única transação: insere a entrada no
	// ledger, cria o lançamento de caixa (Payment) e, SE o pagamento quitou a fatura,
	// realiza em lote as compras do ciclo (RealizeIDs → realizado, payment_date).
	// Escreve em transactions — exceção de posse de tabela documentada (Opção A).
	AddPaymentAtomic(ctx context.Context, in AtomicAddPaymentInput) error
	// RemovePaymentAtomic desfaz um pagamento numa transação: exclui a entrada do ledger
	// (por id) e o lançamento de caixa, e — SE a fatura deixou de estar quitada — reverte
	// as compras do ciclo (RevertIDs → pendente). ErrPaymentNotFound se o id não existir.
	RemovePaymentAtomic(ctx context.Context, in AtomicRemovePaymentInput) error
}

// PaymentTxn descreve o lançamento de pagamento de fatura a ser criado (E1). Nasce
// `realizado`, sem credit_card_id; o Type é derivado da subcategoria escolhida (default
// transferência → neutro ao fluxo, anti-dupla-contagem RF-PAGFAT-06).
type PaymentTxn struct {
	ID             string
	Title          string
	Description    *string
	Amount         int64
	Type           string // despesa|receita|transferencia (derivado da subcategoria)
	SubcategoryID  string
	PaymentMethod  string // "outros" (DA3)
	CompetenceDate string // = PaymentDate
	PaymentDate    string
	CreatedAt      time.Time
}

// AtomicAddPaymentInput é o payload de AddPaymentAtomic.
type AtomicAddPaymentInput struct {
	CardID     string
	Entry      InvoicePayment // entrada do ledger (id, reference, amount, payment_date, transaction_id)
	Payment    PaymentTxn     // lançamento de caixa a criar (id == Entry.TransactionID)
	RealizeIDs []string       // compras pendentes a realizar (só quando o pagamento quita a fatura)
	RealizeAt  string         // payment_date aplicada às compras realizadas
}

// AtomicRemovePaymentInput é o payload de RemovePaymentAtomic.
type AtomicRemovePaymentInput struct {
	PaymentID    string   // id da entrada do ledger a excluir
	PaymentTxnID string   // lançamento de caixa a excluir
	RevertIDs    []string // compras a voltar para pendente (só quando deixa de estar quitada)
}

// SubcategoryReader fornece o tipo de uma subcategoria (despesa/receita/transferencia),
// usado para derivar o Type do lançamento de pagamento (E1). Implementado por
// category.SubcategoryFacade e injetado no main.go (guia §3.3).
type SubcategoryReader interface {
	GetSubcategoryType(ctx context.Context, subcategoryID string) (string, error)
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
