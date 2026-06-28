package transaction

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ReportAggregator satisfaz os ports report.RealizedAggregator, report.PendingAggregator
// e report.PaymentBreakdownReader (structural typing). Lê a tabela transactions (posse
// deste módulo) e devolve agregados neutros (shared.*) — o módulo report não importa
// transaction. Injetado no main.go.
//
// Base do relatório (decisão documentada em report/ARCHITECTURE.md): ACRUAL por
// competência, INCLUINDO cartão de crédito — um relatório de gastos por categoria
// precisa mostrar o que foi gasto no cartão. Difere do regime de caixa (D14) do
// resumo de lançamentos, que é a lente de fluxo de caixa.
type ReportAggregator struct{ db *sql.DB }

// NewReportAggregator cria o agregador.
func NewReportAggregator(db *sql.DB) *ReportAggregator { return &ReportAggregator{db: db} }

// monthBounds devolve o primeiro e o último dia (YYYY-MM-DD) do mês de uma referência.
func monthBounds(reference string) (first, last string, err error) {
	t, err := time.Parse("2006-01", reference)
	if err != nil {
		return "", "", fmt.Errorf("referência inválida %q: %w", reference, err)
	}
	first = t.Format("2006-01-02")
	last = time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	return first, last, nil
}

const aggByStatusSQL = `
SELECT t.subcategory_id, s.category_id, t.type,
       COALESCE(SUM(t.amount), 0), COUNT(*)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = ?
  AND t.competence_date >= ? AND t.competence_date <= ?
  AND s.is_balance_adjustment = 0
GROUP BY t.subcategory_id, s.category_id, t.type`

func (a *ReportAggregator) aggregate(ctx context.Context, status, first, last string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	rows, err := a.db.QueryContext(ctx, aggByStatusSQL, status, first, last)
	if err != nil {
		return nil, shared.MonthlyTotals{}, fmt.Errorf("report aggregator: query: %w", err)
	}
	defer rows.Close()

	aggs := []shared.SubcategoryAggregate{}
	var totals shared.MonthlyTotals
	for rows.Next() {
		var ag shared.SubcategoryAggregate
		if err := rows.Scan(&ag.SubcategoryID, &ag.CategoryID, &ag.Type, &ag.Total, &ag.TxCount); err != nil {
			return nil, shared.MonthlyTotals{}, fmt.Errorf("report aggregator: scan: %w", err)
		}
		aggs = append(aggs, ag)
		totals.TxCount += ag.TxCount
		switch ag.Type {
		case string(TypeReceita):
			totals.Receitas += ag.Total
		case string(TypeDespesa):
			totals.Despesas += ag.Total
		case string(TypeTransferencia):
			totals.Transferencias += ag.Total
		}
	}
	if err := rows.Err(); err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	totals.SaldoPeriodo = totals.Receitas - totals.Despesas
	return aggs, totals, nil
}

// AggregateMonth agrega os lançamentos REALIZADOS do mês + saldo acumulado (E6).
func (a *ReportAggregator) AggregateMonth(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	first, last, err := monthBounds(reference)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	aggs, totals, err := a.aggregate(ctx, string(StatusRealizado), first, last)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	// Saldo acumulado (mesma base acrual do relatório):
	// saldoInicial = fluxo realizado (receita-despesa) com competência < início do mês
	//              + ajustes de saldo (is_balance_adjustment) realizados até o fim do mês.
	carry, err := a.carryover(ctx, first)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	adj, err := a.adjustments(ctx, last)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	totals.SaldoInicial = carry + adj
	totals.SaldoFinal = totals.SaldoInicial + totals.SaldoPeriodo
	return aggs, totals, nil
}

// AggregatePendingMonth agrega os lançamentos PENDENTES do mês (modo projetivo).
func (a *ReportAggregator) AggregatePendingMonth(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	first, last, err := monthBounds(reference)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	return a.aggregate(ctx, string(StatusPendente), first, last)
}

// PaymentBreakdownMonth soma as DESPESAS realizadas do mês por forma de pagamento.
func (a *ReportAggregator) PaymentBreakdownMonth(ctx context.Context, reference string) (map[string]int64, error) {
	first, last, err := monthBounds(reference)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT t.payment_method, COALESCE(SUM(t.amount), 0)
		FROM transactions t
		WHERE t.status = 'realizado' AND t.type = 'despesa'
		  AND t.competence_date >= ? AND t.competence_date <= ?
		GROUP BY t.payment_method`, first, last)
	if err != nil {
		return nil, fmt.Errorf("report aggregator: payment breakdown: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var method string
		var total int64
		if err := rows.Scan(&method, &total); err != nil {
			return nil, fmt.Errorf("report aggregator: scan payment: %w", err)
		}
		out[method] = total
	}
	return out, rows.Err()
}

func (a *ReportAggregator) carryover(ctx context.Context, monthFirstDay string) (int64, error) {
	const q = `
SELECT COALESCE(SUM(CASE t.type WHEN 'receita' THEN t.amount WHEN 'despesa' THEN -t.amount ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND s.is_balance_adjustment = 0 AND t.competence_date < ?`
	var v int64
	if err := a.db.QueryRowContext(ctx, q, monthFirstDay).Scan(&v); err != nil {
		return 0, fmt.Errorf("report aggregator: carryover: %w", err)
	}
	return v, nil
}

func (a *ReportAggregator) adjustments(ctx context.Context, monthLastDay string) (int64, error) {
	const q = `
SELECT COALESCE(SUM(t.amount), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND s.is_balance_adjustment = 1 AND t.competence_date <= ?`
	var v int64
	if err := a.db.QueryRowContext(ctx, q, monthLastDay).Scan(&v); err != nil {
		return 0, fmt.Errorf("report aggregator: adjustments: %w", err)
	}
	return v, nil
}
