package transaction

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CashAggregator satisfaz report.CashAggregator: agrega os lançamentos pelo REGIME DE
// CAIXA (por data de pagamento), para o relatório na lente "Caixa". Difere do
// ReportAggregator (competência). Lê a tabela transactions; devolve shared.* neutros.
//
// Realizado: por payment_date (todo realizado tem payment_date; compra de cartão usa a
// data de pagamento da fatura). Pendente (projetivo-caixa): por competence_date, como
// DATA ESPERADA do movimento (pendente ainda não tem payment_date). Exclui ajustes de
// saldo (is_balance_adjustment) do fluxo.
type CashAggregator struct{ db *sql.DB }

// NewCashAggregator cria o agregador de caixa.
func NewCashAggregator(db *sql.DB) *CashAggregator { return &CashAggregator{db: db} }

const cashAggSQL = `
SELECT t.subcategory_id, s.category_id, t.type, COALESCE(SUM(t.amount), 0), COUNT(*)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = ? AND s.is_balance_adjustment = 0
  AND %s >= ? AND %s <= ?
GROUP BY t.subcategory_id, s.category_id, t.type`

func (a *CashAggregator) aggregate(ctx context.Context, status, dateExpr, from, to string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	q := fmt.Sprintf(cashAggSQL, dateExpr, dateExpr)
	rows, err := a.db.QueryContext(ctx, q, status, from, to)
	if err != nil {
		return nil, shared.MonthlyTotals{}, fmt.Errorf("cash aggregator: query: %w", err)
	}
	defer rows.Close()

	aggs := []shared.SubcategoryAggregate{}
	var totals shared.MonthlyTotals
	for rows.Next() {
		var ag shared.SubcategoryAggregate
		if err := rows.Scan(&ag.SubcategoryID, &ag.CategoryID, &ag.Type, &ag.Total, &ag.TxCount); err != nil {
			return nil, shared.MonthlyTotals{}, fmt.Errorf("cash aggregator: scan: %w", err)
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

// AggregateCashPeriod agrega o REALIZADO por data de pagamento em [from,to] e calcula o
// saldo acumulado de caixa (saldoInicial por carryover até `from`; saldoFinal = inicial + período).
func (a *CashAggregator) AggregateCashPeriod(ctx context.Context, from, to string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	aggs, totals, err := a.aggregate(ctx, string(StatusRealizado), "t.payment_date", from, to)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	carry, err := a.cashCarryover(ctx, from)
	if err != nil {
		return nil, shared.MonthlyTotals{}, err
	}
	totals.SaldoInicial = carry
	totals.SaldoFinal = totals.SaldoInicial + totals.SaldoPeriodo
	return aggs, totals, nil
}

// AggregateCashPending agrega os PENDENTES por competência (data esperada) — projetivo-caixa.
func (a *CashAggregator) AggregateCashPending(ctx context.Context, from, to string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return a.aggregate(ctx, string(StatusPendente), "t.competence_date", from, to)
}

// PaymentBreakdownCash soma as despesas realizadas por forma de pagamento, por data de pagamento.
func (a *CashAggregator) PaymentBreakdownCash(ctx context.Context, from, to string) (map[string]int64, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT t.payment_method, COALESCE(SUM(t.amount), 0)
		FROM transactions t
		WHERE t.status = 'realizado' AND t.type = 'despesa'
		  AND t.payment_date >= ? AND t.payment_date <= ?
		GROUP BY t.payment_method`, from, to)
	if err != nil {
		return nil, fmt.Errorf("cash aggregator: payment breakdown: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var method string
		var total int64
		if err := rows.Scan(&method, &total); err != nil {
			return nil, fmt.Errorf("cash aggregator: scan payment: %w", err)
		}
		out[method] = total
	}
	return out, rows.Err()
}

// cashCarryover soma o fluxo de caixa (receita − despesa) realizado por data de pagamento
// ANTES de `from`, mais os ajustes de saldo (is_balance_adjustment, por competência).
func (a *CashAggregator) cashCarryover(ctx context.Context, from string) (int64, error) {
	const flowQ = `
SELECT COALESCE(SUM(CASE t.type WHEN 'receita' THEN t.amount WHEN 'despesa' THEN -t.amount ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND s.is_balance_adjustment = 0 AND t.payment_date < ?`
	var flow int64
	if err := a.db.QueryRowContext(ctx, flowQ, from).Scan(&flow); err != nil {
		return 0, fmt.Errorf("cash aggregator: carryover flow: %w", err)
	}
	// ajustes de saldo (E6) que não são de caixinha: estabelecem patrimônio de caixa.
	const adjQ = `
SELECT COALESCE(SUM(t.amount), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND s.is_balance_adjustment = 1 AND t.caixinha_id IS NULL AND t.competence_date < ?`
	var adj int64
	if err := a.db.QueryRowContext(ctx, adjQ, from).Scan(&adj); err != nil {
		return 0, fmt.Errorf("cash aggregator: carryover adj: %w", err)
	}
	return flow + adj, nil
}
