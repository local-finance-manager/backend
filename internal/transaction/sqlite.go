package transaction

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

var _ TransactionRepository = (*SQLiteRepository)(nil)

// SQLiteRepository implements TransactionRepository using SQLite.
type SQLiteRepository struct{ db *sql.DB }

// NewSQLiteRepository creates a new SQLiteRepository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

// ─── Create ───────────────────────────────────────────────────────────────────

const createSQL = `
INSERT INTO transactions (
    id, title, description, amount, type, subcategory_id,
    payment_method, status, competence_date, payment_date,
    account_id, destination_account_id, credit_card_id, caixinha_id, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func (r *SQLiteRepository) Create(ctx context.Context, t Transaction) error {
	_, err := r.db.ExecContext(ctx, createSQL,
		t.ID, t.Title, nullStr(deref(t.Description)),
		t.Amount, string(t.Type), t.SubcategoryID,
		string(t.PaymentMethod), string(t.Status),
		t.CompetenceDate, nullStr(deref(t.PaymentDate)),
		nullStr(deref(t.AccountID)),
		nullStr(deref(t.DestinationAccountID)),
		nullStr(deref(t.CreditCardID)),
		nullStr(deref(t.CaixinhaID)),
		t.CreatedAt.UTC().Format(time.RFC3339),
		t.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("transaction sqlite: create: %w", err)
	}
	return nil
}

// ─── Get ──────────────────────────────────────────────────────────────────────

const getSQL = `
SELECT t.id, t.title, t.description, t.amount, t.type, t.subcategory_id,
       t.payment_method, t.status, t.competence_date, t.payment_date,
       t.account_id, t.destination_account_id, t.credit_card_id, t.caixinha_id,
       t.installment_group_id, t.installment_number, t.installment_total, t.created_at, t.updated_at,
       s.id, s.name, COALESCE(s.icon,''), COALESCE(s.color,''),
       c.id, c.name, COALESCE(c.icon,''), COALESCE(c.color,'')
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
JOIN categories    c ON c.id = s.category_id
WHERE t.id = ?`

func (r *SQLiteRepository) Get(ctx context.Context, id string) (TransactionDetail, error) {
	row := r.db.QueryRowContext(ctx, getSQL, id)
	d, err := scanDetail(row.Scan)
	if err == sql.ErrNoRows {
		return TransactionDetail{}, ErrTransactionNotFound
	}
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("transaction sqlite: get: %w", err)
	}
	return d, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

const updateSQL = `
UPDATE transactions SET
    title=?, description=?, amount=?, type=?, subcategory_id=?,
    payment_method=?, status=?, competence_date=?, payment_date=?,
    account_id=?, destination_account_id=?, credit_card_id=?, updated_at=?
WHERE id=?`

func (r *SQLiteRepository) Update(ctx context.Context, t Transaction) error {
	result, err := r.db.ExecContext(ctx, updateSQL,
		t.Title, nullStr(deref(t.Description)),
		t.Amount, string(t.Type), t.SubcategoryID,
		string(t.PaymentMethod), string(t.Status),
		t.CompetenceDate, nullStr(deref(t.PaymentDate)),
		nullStr(deref(t.AccountID)),
		nullStr(deref(t.DestinationAccountID)),
		nullStr(deref(t.CreditCardID)),
		t.UpdatedAt.UTC().Format(time.RFC3339),
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("transaction sqlite: update: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrTransactionNotFound
	}
	return nil
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("transaction sqlite: delete: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrTransactionNotFound
	}
	return nil
}

// ─── buildFilter ──────────────────────────────────────────────────────────────

// buildFilter constructs the parametrized WHERE clause from a TransactionFilter.
// The caller must JOIN subcategories s and categories c so that s.category_id is available.
func buildFilter(f TransactionFilter) (string, []any) {
	conds := []string{"1=1"}
	var args []any

	if f.Type != nil {
		conds = append(conds, "t.type = ?")
		args = append(args, string(*f.Type))
	}
	if f.Status != nil {
		conds = append(conds, "t.status = ?")
		args = append(args, string(*f.Status))
	}
	if f.PaymentMethod != nil {
		conds = append(conds, "t.payment_method = ?")
		args = append(args, string(*f.PaymentMethod))
	}
	if f.SubcategoryID != nil {
		conds = append(conds, "t.subcategory_id = ?")
		args = append(args, *f.SubcategoryID)
	}
	if f.CategoryID != nil {
		conds = append(conds, "s.category_id = ?")
		args = append(args, *f.CategoryID)
	}
	if f.AccountID != nil {
		conds = append(conds, "t.account_id = ?")
		args = append(args, *f.AccountID)
	}
	// A tela de Lançamentos navega por MÊS DE CAIXA: o período filtra a data efetiva de
	// caixa (pagamento p/ realizado, competência p/ pendente), não a competência crua.
	// Assim uma receita de competência 30/jun paga em 01/jul aparece em JULHO.
	if f.CompetenceDateFrom != nil {
		conds = append(conds, effectiveCashDate+" >= ?")
		args = append(args, *f.CompetenceDateFrom)
	}
	if f.CompetenceDateTo != nil {
		conds = append(conds, effectiveCashDate+" <= ?")
		args = append(args, *f.CompetenceDateTo)
	}
	if f.PaymentDateFrom != nil {
		conds = append(conds, "t.payment_date >= ?")
		args = append(args, *f.PaymentDateFrom)
	}
	if f.PaymentDateTo != nil {
		conds = append(conds, "t.payment_date <= ?")
		args = append(args, *f.PaymentDateTo)
	}
	if f.Search != nil && *f.Search != "" {
		conds = append(conds, "LOWER(t.title) LIKE ?")
		args = append(args, "%"+strings.ToLower(*f.Search)+"%")
	}
	if f.CreditCardID != nil {
		conds = append(conds, "t.credit_card_id = ?")
		args = append(args, *f.CreditCardID)
	}
	if f.InstallmentGroupID != nil {
		conds = append(conds, "t.installment_group_id = ?")
		args = append(args, *f.InstallmentGroupID)
	}
	// Movimentos de caixinha (aporte/resgate) só aparecem quando explicitamente pedidos
	// (extrato via CaixinhaID) ou quando IncludeCaixinha=true. Por padrão são escondidos
	// da lista de Lançamentos — o oposto do "transferência" que poluía a lista.
	if f.CaixinhaID != nil {
		conds = append(conds, "t.caixinha_id = ?")
		args = append(args, *f.CaixinhaID)
	} else if !f.IncludeCaixinha {
		conds = append(conds, "t.caixinha_id IS NULL")
	}

	return strings.Join(conds, " AND "), args
}

// ─── List ─────────────────────────────────────────────────────────────────────

const listBaseSQL = `
SELECT t.id, t.title, t.description, t.amount, t.type, t.subcategory_id,
       t.payment_method, t.status, t.competence_date, t.payment_date,
       t.account_id, t.destination_account_id, t.credit_card_id, t.caixinha_id,
       t.installment_group_id, t.installment_number, t.installment_total, t.created_at, t.updated_at,
       s.id, s.name, COALESCE(s.icon,''), COALESCE(s.color,''),
       c.id, c.name, COALESCE(c.icon,''), COALESCE(c.color,'')
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
JOIN categories    c ON c.id = s.category_id`

func (r *SQLiteRepository) List(ctx context.Context, f TransactionFilter, p shared.Pagination) ([]TransactionDetail, error) {
	where, args := buildFilter(f)
	// p.OrderBy and p.Order are validated by ParsePagination (allowlist + normalize).
	query := fmt.Sprintf("%s WHERE %s ORDER BY t.%s %s LIMIT ? OFFSET ?",
		listBaseSQL, where, p.OrderBy, p.Order)
	args = append(args, p.Limit, p.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("transaction sqlite: list: %w", err)
	}
	defer rows.Close()

	results := []TransactionDetail{}
	for rows.Next() {
		d, err := scanDetail(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("transaction sqlite: list scan: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// ─── GetSummary ───────────────────────────────────────────────────────────────

const summaryBaseSQL = `
SELECT t.type, t.status, COALESCE(SUM(t.amount), 0), COUNT(*)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
JOIN categories    c ON c.id = s.category_id`

// effectiveCashDate é a data que define em qual mês o lançamento pesa no CAIXA: pela DATA
// DE PAGAMENTO quando existe (todo realizado tem; compra de cartão usa a data de pagamento
// da fatura), e pela competência apenas quando não há pagamento (pendente = data esperada).
const effectiveCashDate = "(CASE WHEN t.payment_date IS NOT NULL THEN t.payment_date ELSE t.competence_date END)"

// buildFilterNoDate é o buildFilter sem as condições de data — o resumo de caixa aplica o
// período sobre a data efetiva de caixa (effectiveCashDate), não sobre a competência crua.
func buildFilterNoDate(f TransactionFilter) (string, []any) {
	nf := f
	nf.CompetenceDateFrom, nf.CompetenceDateTo = nil, nil
	nf.PaymentDateFrom, nf.PaymentDateTo = nil, nil
	return buildFilter(nf)
}

func (r *SQLiteRepository) GetSummary(ctx context.Context, f TransactionFilter) (Summary, error) {
	where, args := buildFilter(f)

	var s Summary

	// CountTotal = TOTAL de lançamentos que casam o filtro, INCLUINDO cartão. É o que
	// alimenta a paginação (total/total_pages); excluir cartão aqui esconderia compras
	// de cartão da 2ª página em diante (itens inacessíveis na tela).
	countQuery := fmt.Sprintf(
		"SELECT COUNT(*) FROM transactions t "+
			"JOIN subcategories s ON s.id = t.subcategory_id "+
			"JOIN categories c ON c.id = s.category_id WHERE %s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&s.CountTotal); err != nil {
		return Summary{}, fmt.Errorf("transaction sqlite: summary count: %w", err)
	}

	// D14 (regime de caixa, Opção 1): cada lançamento entra nos totais pela DATA EFETIVA DE
	// CAIXA — competência para lançamentos normais, e DATA DE PAGAMENTO para compras de
	// cartão (o dinheiro do cartão só sai quando a fatura é paga). Compra de cartão pendente
	// (sem payment_date) não entra no realizado. O período (datas do filtro) é aplicado sobre
	// essa data efetiva.
	noDate, ndArgs := buildFilterNoDate(f)
	query := fmt.Sprintf("%s WHERE %s", summaryBaseSQL, noDate)
	if f.CompetenceDateFrom != nil {
		query += " AND " + effectiveCashDate + " >= ?"
		ndArgs = append(ndArgs, *f.CompetenceDateFrom)
	}
	if f.CompetenceDateTo != nil {
		query += " AND " + effectiveCashDate + " <= ?"
		ndArgs = append(ndArgs, *f.CompetenceDateTo)
	}
	query += " GROUP BY t.type, t.status"

	rows, err := r.db.QueryContext(ctx, query, ndArgs...)
	if err != nil {
		return Summary{}, fmt.Errorf("transaction sqlite: get summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var typ, status string
		var totalAmount int64
		var count int
		if err := rows.Scan(&typ, &status, &totalAmount, &count); err != nil {
			return Summary{}, fmt.Errorf("transaction sqlite: summary scan: %w", err)
		}
		_ = count // a contagem real vem de countQuery; aqui só as somas importam
		switch {
		case status == string(StatusRealizado) && typ == string(TypeDespesa):
			s.TotalDespesas += totalAmount
		case status == string(StatusRealizado) && typ == string(TypeReceita):
			s.TotalReceitas += totalAmount
		}
	}
	if err := rows.Err(); err != nil {
		return Summary{}, err
	}
	s.SaldoPeriodo = s.TotalReceitas - s.TotalDespesas

	// TotalPendente = soma de TODOS os lançamentos pendentes do filtro, independente de
	// tipo (RF lancamentos.md §sumário). NÃO aplica o filtro de cartão (D14): D14 vale
	// para o saldo REALIZADO em caixa; "Pendente" é um indicador prospectivo e DEVE
	// incluir compras de cartão pendentes — senão, com lançamentos só de cartão, o
	// "Pendente" aparece zerado (enganoso). Query própria, sem `credit_card_id IS NULL`.
	pendQuery := fmt.Sprintf(
		"SELECT COALESCE(SUM(t.amount), 0) FROM transactions t "+
			"JOIN subcategories s ON s.id = t.subcategory_id "+
			"JOIN categories c ON c.id = s.category_id WHERE %s AND t.status = 'pendente'", where)
	if err := r.db.QueryRowContext(ctx, pendQuery, args...).Scan(&s.TotalPendente); err != nil {
		return Summary{}, fmt.Errorf("transaction sqlite: summary pendente: %w", err)
	}

	// E6 (saldo acumulado): SaldoInicial e SaldoFinal usam SÓ as datas do filtro (ignoram
	// tipo/categoria/cartão/busca) — um "saldo" filtrado por categoria não faria sentido.
	carryover, err := r.carryoverBalance(ctx, f.CompetenceDateFrom)
	if err != nil {
		return Summary{}, err
	}
	adjustments, err := r.adjustmentsTotal(ctx, f.CompetenceDateTo)
	if err != nil {
		return Summary{}, err
	}
	// Movimentos de caixinha no período (resgate +, aporte −), pela data de caixa. Como
	// SaldoInicial/SaldoFinal, usa SÓ as datas do filtro (ignora tipo/categoria/busca).
	mov, err := r.movimentacaoCaixinhas(ctx, f.CompetenceDateFrom, f.CompetenceDateTo)
	if err != nil {
		return Summary{}, err
	}
	s.MovimentacaoCaixinhas = mov
	s.SaldoInicial = carryover + adjustments
	s.SaldoFinal = s.SaldoInicial + s.SaldoPeriodo + s.MovimentacaoCaixinhas
	return s, nil
}

// movimentacaoCaixinhas soma o efeito líquido dos movimentos de caixinha REALIZADOS
// (resgate +, aporte −) com data de caixa (payment_date) no intervalo [from, to].
func (r *SQLiteRepository) movimentacaoCaixinhas(ctx context.Context, from, to *string) (int64, error) {
	// is_balance_adjustment=0 exclui o SALDO INICIAL da caixinha (abertura), que estabelece
	// patrimônio guardado sem mover o disponível.
	q := `
SELECT COALESCE(SUM(CASE
		WHEN s.caixinha_direction = 'resgate' THEN t.amount
		WHEN s.caixinha_direction = 'aporte'  THEN -t.amount
		ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND t.caixinha_id IS NOT NULL AND s.is_balance_adjustment = 0`
	args := []any{}
	if from != nil {
		q += " AND t.payment_date >= ?"
		args = append(args, *from)
	}
	if to != nil {
		q += " AND t.payment_date <= ?"
		args = append(args, *to)
	}
	var v int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&v); err != nil {
		return 0, fmt.Errorf("transaction sqlite: movimentacao caixinhas: %w", err)
	}
	return v, nil
}

// carryoverBalance soma o fluxo (receita - despesa) de lançamentos realizados, sem cartão
// (D14), com competência ANTES de `from`. É o saldo acumulado carregado para o período.
// Sem `from` (período aberto à esquerda) não há carryover → 0.
func (r *SQLiteRepository) carryoverBalance(ctx context.Context, from *string) (int64, error) {
	if from == nil {
		return 0, nil
	}
	// Saldo acumulado em caixa: inclui compras de cartão já pagas (pela data de pagamento),
	// os demais lançamentos pela competência, e os movimentos de caixinha (resgate +,
	// aporte −) — tudo com data efetiva ANTES de `from`.
	const q = `
SELECT COALESCE(SUM(CASE
		WHEN s.caixinha_direction = 'resgate' AND s.is_balance_adjustment = 0 THEN t.amount
		WHEN s.caixinha_direction = 'aporte'  AND s.is_balance_adjustment = 0 THEN -t.amount
		WHEN t.type = 'receita' THEN t.amount
		WHEN t.type = 'despesa' THEN -t.amount
		ELSE 0 END), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado'
  AND ` + effectiveCashDate + ` < ?`
	var v int64
	if err := r.db.QueryRowContext(ctx, q, *from).Scan(&v); err != nil {
		return 0, fmt.Errorf("transaction sqlite: carryover: %w", err)
	}
	return v, nil
}

// adjustmentsTotal soma os ajustes de saldo (is_balance_adjustment) realizados até `to`
// (inclusive). São transferências que estabelecem patrimônio → entram no saldo acumulado,
// não no fluxo. Sem `to` (período aberto à direita) soma todos os ajustes.
func (r *SQLiteRepository) adjustmentsTotal(ctx context.Context, to *string) (int64, error) {
	// caixinha_id IS NULL: o SALDO INICIAL da caixinha também é is_balance_adjustment, mas
	// alimenta o GUARDADO (não o disponível) — não pode entrar aqui.
	q := `
SELECT COALESCE(SUM(t.amount), 0)
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
WHERE t.status = 'realizado' AND s.is_balance_adjustment = 1 AND t.caixinha_id IS NULL`
	args := []any{}
	if to != nil {
		q += " AND " + effectiveCashDate + " <= ?"
		args = append(args, *to)
	}
	var v int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&v); err != nil {
		return 0, fmt.Errorf("transaction sqlite: adjustments total: %w", err)
	}
	return v, nil
}

// ─── Scan helper ──────────────────────────────────────────────────────────────

// scanFunc allows sharing scanDetail between QueryRow.Scan and Rows.Scan.
type scanFunc func(dest ...any) error

func scanDetail(scan scanFunc) (TransactionDetail, error) {
	var d TransactionDetail
	var desc, payDate, accID, destAccID, creditCardID, caixinhaID, installmentGroupID sql.NullString
	var installmentNumber, installmentTotal sql.NullInt64
	var createdAt, updatedAt string

	err := scan(
		&d.ID, &d.Title, &desc, &d.Amount, (*string)(&d.Type), &d.SubcategoryID,
		(*string)(&d.PaymentMethod), (*string)(&d.Status),
		&d.CompetenceDate, &payDate, &accID, &destAccID, &creditCardID, &caixinhaID,
		&installmentGroupID, &installmentNumber, &installmentTotal,
		&createdAt, &updatedAt,
		&d.Subcategory.ID, &d.Subcategory.Name, &d.Subcategory.Icon, &d.Subcategory.Color,
		&d.Subcategory.Category.ID, &d.Subcategory.Category.Name,
		&d.Subcategory.Category.Icon, &d.Subcategory.Category.Color,
	)
	if err != nil {
		return TransactionDetail{}, err
	}

	if desc.Valid {
		d.Description = &desc.String
	}
	if payDate.Valid {
		d.PaymentDate = &payDate.String
	}
	if accID.Valid {
		d.AccountID = &accID.String
	}
	if destAccID.Valid {
		d.DestinationAccountID = &destAccID.String
	}
	if creditCardID.Valid {
		d.CreditCardID = &creditCardID.String
	}
	if caixinhaID.Valid {
		d.CaixinhaID = &caixinhaID.String
	}
	if installmentGroupID.Valid {
		d.InstallmentGroupID = &installmentGroupID.String
	}
	if installmentNumber.Valid {
		n := int(installmentNumber.Int64)
		d.InstallmentNumber = &n
	}
	if installmentTotal.Valid {
		n := int(installmentTotal.Int64)
		d.InstallmentTotal = &n
	}

	d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return d, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
