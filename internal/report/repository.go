package report

import (
	"context"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Closing é o registro de um mês fechado + seus totais congelados.
type Closing struct {
	Reference      string
	ClosedAt       time.Time
	MonthLastDay   string
	HardLockAt     string
	Totals         shared.MonthlyTotals
	RecalculatedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ─── Repository (owner das tabelas de fechamento/snapshot) ───────────────────

// Repository persiste fechamentos e snapshots. O fechamento/recálculo é atômico.
type Repository interface {
	// GetClosing retorna o fechamento de um mês (ok=false se aberto).
	GetClosing(ctx context.Context, reference string) (Closing, bool, error)
	// ListClosings retorna todos os meses fechados (ordem desc por referência).
	ListClosings(ctx context.Context) ([]Closing, error)
	// ClosingsForRefs retorna os fechamentos existentes entre as referências dadas.
	ClosingsForRefs(ctx context.Context, refs []string) (map[string]Closing, error)
	// SaveClosing grava (upsert) o fechamento e REGENERA o snapshot do mês numa única
	// transação: apaga o snapshot anterior do mês, insere as novas linhas e grava o
	// closing (idempotente — RNF-REL-03/07).
	SaveClosing(ctx context.Context, c Closing, rows []shared.SubcategoryAggregate) error
	// Snapshot retorna os agregados por subcategoria de um mês fechado.
	Snapshot(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, error)
	// SnapshotForRefs soma os agregados por subcategoria sobre as referências dadas
	// (períodos longos — soma de snapshots, nunca toca em transações).
	SnapshotForRefs(ctx context.Context, refs []string) ([]shared.SubcategoryAggregate, error)
}

// ─── Ports consumidos (injetados no main.go) ─────────────────────────────────

// RealizedAggregator varre os lançamentos REALIZADOS de um mês (por competência),
// agregando por subcategoria + totais do período. Usado no fechamento/recálculo e
// no mensal de mês ABERTO. Implementado pelo módulo transaction.
type RealizedAggregator interface {
	AggregateMonth(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error)
}

// PendingAggregator varre os lançamentos PENDENTES de um mês (modo projetivo).
type PendingAggregator interface {
	AggregatePendingMonth(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error)
}

// CategoryTreeReader fornece nomes/cores de categorias e nomes de subcategorias
// para compor a resposta do relatório. Implementado pelo módulo category.
type CategoryTreeReader interface {
	Tree(ctx context.Context) ([]shared.CategoryNode, error)
}

// PaymentBreakdownReader devolve o total de DESPESAS realizadas por forma de
// pagamento num mês (para % no crédito e distribuição — só mensal). Implementado
// pelo módulo transaction.
type PaymentBreakdownReader interface {
	PaymentBreakdownMonth(ctx context.Context, reference string) (map[string]int64, error)
}

// CashAggregator agrega pelo regime de CAIXA (por data de pagamento) sobre um intervalo
// arbitrário [from,to] (YYYY-MM-DD). Apurado ao vivo (não usa snapshot), em todos os
// períodos. Implementado pelo módulo transaction.
type CashAggregator interface {
	AggregateCashPeriod(ctx context.Context, from, to string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error)
	AggregateCashPending(ctx context.Context, from, to string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error)
	PaymentBreakdownCash(ctx context.Context, from, to string) (map[string]int64, error)
}
