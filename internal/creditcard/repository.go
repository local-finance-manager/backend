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

// InvoicePaymentRepository persiste o status de pagamento de faturas fechadas
// (única parte da fatura armazenada — D4).
type InvoicePaymentRepository interface {
	Get(ctx context.Context, cardID, reference string) (*InvoicePayment, error)
	ListByCard(ctx context.Context, cardID string) (map[string]*InvoicePayment, error)
	Delete(ctx context.Context, cardID, reference string) error
	// PayInvoiceAtomic executa, numa única transação (RF-PAGFAT-04): realiza em lote as
	// compras do ciclo (RealizeIDs → realizado, payment_date), insere o lançamento de
	// pagamento (Payment) e grava o registro em credit_card_invoice_payments. Escreve em
	// transactions — exceção de posse de tabela documentada (Opção A, como installment).
	PayInvoiceAtomic(ctx context.Context, in AtomicPayInput) error
	// UndoPaymentAtomic reverte tudo numa transação: compras realizado→pendente (RevertIDs,
	// limpando payment_date), exclui o lançamento de pagamento (PaymentTxnID) e remove o
	// registro de pagamento. Retorna ErrInvoiceNotFound se não havia registro de pagamento.
	UndoPaymentAtomic(ctx context.Context, in AtomicUndoInput) error
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

// AtomicPayInput é o payload de PayInvoiceAtomic.
type AtomicPayInput struct {
	CardID     string
	Reference  string
	RealizeIDs []string // compras pendentes do ciclo a realizar
	RealizeAt  string   // payment_date aplicada às compras realizadas (= PaymentDate)
	Payment    PaymentTxn
}

// AtomicUndoInput é o payload de UndoPaymentAtomic.
type AtomicUndoInput struct {
	CardID       string
	Reference    string
	RevertIDs    []string // compras realizadas do ciclo a voltar para pendente
	PaymentTxnID string   // lançamento de pagamento a excluir
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
