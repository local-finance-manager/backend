package transaction

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CaixinhaReader satisfaz patrimonio.MovementReader e patrimonio.DisponivelReader.
// Lê diretamente do banco (DB-backed, como CardReader) os movimentos de caixinha
// (lançamentos com caixinha_id) e o saldo disponível corrente.
type CaixinhaReader struct{ db *sql.DB }

// NewCaixinhaReader cria o reader.
func NewCaixinhaReader(db *sql.DB) *CaixinhaReader { return &CaixinhaReader{db: db} }

// ListByCaixinha devolve o extrato paginado de uma caixinha (mais recente primeiro)
// e o total de movimentos.
func (r *CaixinhaReader) ListByCaixinha(ctx context.Context, caixinhaID string, p shared.Pagination) ([]shared.CaixinhaMovement, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM transactions WHERE caixinha_id = ?", caixinhaID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("transaction sqlite: extrato count: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT t.id, t.caixinha_id, COALESCE(s.caixinha_direction, ''), t.amount,
       COALESCE(t.payment_date, t.competence_date), COALESCE(t.description, '')
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.caixinha_id = ?
ORDER BY COALESCE(t.payment_date, t.competence_date) DESC, t.created_at DESC
LIMIT ? OFFSET ?`, caixinhaID, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("transaction sqlite: extrato: %w", err)
	}
	defer rows.Close()

	out := []shared.CaixinhaMovement{}
	for rows.Next() {
		var m shared.CaixinhaMovement
		if err := rows.Scan(&m.TransactionID, &m.CaixinhaID, &m.Direction, &m.Amount, &m.Date, &m.Description); err != nil {
			return nil, 0, fmt.Errorf("transaction sqlite: extrato scan: %w", err)
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// BalanceByCaixinha devolve o saldo GUARDADO de uma caixinha (aportes − resgates),
// considerando só movimentos realizados. Nunca negativo em uso normal (invariante do service).
func (r *CaixinhaReader) BalanceByCaixinha(ctx context.Context, caixinhaID string) (int64, error) {
	const q = `
SELECT COALESCE(SUM(CASE
		WHEN s.caixinha_direction = 'aporte'  THEN t.amount
		WHEN s.caixinha_direction = 'resgate' THEN -t.amount
		ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND t.caixinha_id = ?`
	var v int64
	if err := r.db.QueryRowContext(ctx, q, caixinhaID).Scan(&v); err != nil {
		return 0, fmt.Errorf("transaction sqlite: balance caixinha: %w", err)
	}
	return v, nil
}

// OpeningMovementIDs devolve os ids dos lançamentos de SALDO INICIAL de uma caixinha
// (subcategoria de abertura). Usado para substituir o saldo inicial (apaga o anterior).
func (r *CaixinhaReader) OpeningMovementIDs(ctx context.Context, caixinhaID string) ([]string, error) {
	// Específico da subcategoria de saldo inicial — NÃO pega rendimentos (que também são
	// is_balance_adjustment), senão o "definir saldo inicial" apagaria os rendimentos.
	rows, err := r.db.QueryContext(ctx, `
SELECT id FROM transactions
WHERE caixinha_id = ? AND subcategory_id = 'sub-caixinha-saldo-inicial'`, caixinhaID)
	if err != nil {
		return nil, fmt.Errorf("transaction sqlite: opening ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("transaction sqlite: opening ids scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// BalancesAll devolve o saldo guardado de cada caixinha (map caixinhaID -> saldo).
func (r *CaixinhaReader) BalancesAll(ctx context.Context) (map[string]int64, error) {
	const q = `
SELECT t.caixinha_id, COALESCE(SUM(CASE
		WHEN s.caixinha_direction = 'aporte'  THEN t.amount
		WHEN s.caixinha_direction = 'resgate' THEN -t.amount
		ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND t.caixinha_id IS NOT NULL
GROUP BY t.caixinha_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("transaction sqlite: balances all: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var v int64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, fmt.Errorf("transaction sqlite: balances all scan: %w", err)
		}
		out[id] = v
	}
	return out, rows.Err()
}

// DisponivelAtual devolve o saldo disponível de caixa corrente (todo o histórico),
// reusando GetSummary sem filtro de datas — SaldoFinal já reflete aporte(−)/resgate(+).
func (r *CaixinhaReader) DisponivelAtual(ctx context.Context) (int64, error) {
	repo := &SQLiteRepository{db: r.db}
	s, err := repo.GetSummary(ctx, TransactionFilter{})
	if err != nil {
		return 0, err
	}
	return s.SaldoFinal, nil
}
