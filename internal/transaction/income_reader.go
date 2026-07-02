package transaction

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local-finance-manager/backend/internal/shared"
)

// IncomeReader satisfaz budget.IncomeReader (structural typing): lê a renda do mês
// (receitas por competência, excluindo ajustes de saldo) para o plano de alocação.
// Definido aqui (produtor, dono da tabela); injetado no main.go. Retorna shared.*.
type IncomeReader struct{ db *sql.DB }

// NewIncomeReader cria o leitor de renda.
func NewIncomeReader(db *sql.DB) *IncomeReader { return &IncomeReader{db: db} }

// MonthIncome retorna a renda do mês (soma das receitas pendentes+realizadas),
// se toda a renda está realizada (sem pendentes) e a lista de itens.
func (r *IncomeReader) MonthIncome(ctx context.Context, reference string) (int64, bool, []shared.IncomeItem, error) {
	first, last, err := monthBounds(reference)
	if err != nil {
		return 0, false, nil, err
	}
	// Renda por REGIME DE CAIXA: uma receita realizada pertence ao mês em que o dinheiro
	// caiu (data de pagamento); pendente pertence ao mês da competência (data esperada).
	// Assim salário de competência 30/jun pago em 01/jul conta como renda de JULHO.
	const cashDate = "(CASE WHEN t.payment_date IS NOT NULL THEN t.payment_date ELSE t.competence_date END)"
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.title, t.amount, t.status
		FROM transactions t
		JOIN subcategories s ON s.id = t.subcategory_id
		WHERE t.type = 'receita' AND t.status IN ('pendente','realizado')
		  AND s.is_balance_adjustment = 0
		  AND `+cashDate+` >= ? AND `+cashDate+` <= ?
		ORDER BY `+cashDate+`, t.created_at`, first, last)
	if err != nil {
		return 0, false, nil, fmt.Errorf("income reader: query: %w", err)
	}
	defer rows.Close()

	items := []shared.IncomeItem{}
	var total int64
	pending := 0
	for rows.Next() {
		var it shared.IncomeItem
		if err := rows.Scan(&it.TransactionID, &it.Title, &it.Amount, &it.Status); err != nil {
			return 0, false, nil, fmt.Errorf("income reader: scan: %w", err)
		}
		total += it.Amount
		if it.Status == string(StatusPendente) {
			pending++
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, false, nil, err
	}
	return total, pending == 0, items, nil
}
