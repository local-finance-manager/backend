package installment

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

var _ Repository = (*SQLiteRepository)(nil)

// SQLiteRepository implementa Repository. Possui installment_groups e escreve as
// parcelas como linhas em transactions na mesma tx (Opção A — ver ARCHITECTURE.md).
type SQLiteRepository struct{ db *sql.DB }

// NewSQLiteRepository cria um SQLiteRepository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

const insertGroupSQL = `
INSERT INTO installment_groups (
    id, credit_card_id, subcategory_id, title, description, total_amount,
    principal_amount, installments_count, purchase_date, first_reference, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`

const insertParcelaSQL = `
INSERT INTO transactions (
    id, title, amount, type, subcategory_id, payment_method, status, competence_date,
    credit_card_id, installment_group_id, installment_number, installment_total, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

// Create grava o grupo + as N parcelas numa única transação (tudo-ou-nada — RNF-PARC-03).
func (r *SQLiteRepository) Create(ctx context.Context, g InstallmentGroup, parcelas []Parcela) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("installment repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op após Commit

	created := g.CreatedAt.UTC().Format(time.RFC3339)
	updated := g.UpdatedAt.UTC().Format(time.RFC3339)

	if _, err := tx.ExecContext(ctx, insertGroupSQL,
		g.ID, g.CreditCardID, g.SubcategoryID, g.Title, nullStr(g.Description), g.TotalAmount,
		nullInt64(g.PrincipalAmount), g.InstallmentsCount, g.PurchaseDate, g.FirstReference, created, updated,
	); err != nil {
		return fmt.Errorf("installment repo: insert group: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, insertParcelaSQL)
	if err != nil {
		return fmt.Errorf("installment repo: prepare parcela: %w", err)
	}
	defer stmt.Close()

	for _, p := range parcelas {
		if _, err := stmt.ExecContext(ctx,
			p.ID, g.Title, p.Amount, expenseType, g.SubcategoryID, "cartao_credito", parcelaPending,
			p.CompetenceDate, g.CreditCardID, g.ID, p.Number, g.InstallmentsCount, created, updated,
		); err != nil {
			return fmt.Errorf("installment repo: insert parcela %d: %w", p.Number, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("installment repo: commit: %w", err)
	}
	return nil
}

const selectGroupCols = `
SELECT id, credit_card_id, subcategory_id, title, description, total_amount,
       principal_amount, installments_count, purchase_date, first_reference, created_at, updated_at
FROM installment_groups`

func (r *SQLiteRepository) Get(ctx context.Context, id string) (InstallmentGroup, []Installment, error) {
	row := r.db.QueryRowContext(ctx, selectGroupCols+" WHERE id = ?", id)
	g, err := scanGroup(row)
	if err == sql.ErrNoRows {
		return InstallmentGroup{}, nil, ErrInstallmentGroupNotFound
	}
	if err != nil {
		return InstallmentGroup{}, nil, fmt.Errorf("installment repo: get group: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, installment_number, amount, competence_date, status
		 FROM transactions WHERE installment_group_id = ? ORDER BY installment_number`, id)
	if err != nil {
		return InstallmentGroup{}, nil, fmt.Errorf("installment repo: get parcelas: %w", err)
	}
	defer rows.Close()

	var parcelas []Installment
	for rows.Next() {
		var p Installment
		var number sql.NullInt64
		if err := rows.Scan(&p.TransactionID, &number, &p.Amount, &p.CompetenceDate, &p.Status); err != nil {
			return InstallmentGroup{}, nil, fmt.Errorf("installment repo: scan parcela: %w", err)
		}
		p.Number = int(number.Int64)
		parcelas = append(parcelas, p)
	}
	return g, parcelas, rows.Err()
}

const listAggSQL = `
SELECT g.id, g.credit_card_id, g.subcategory_id, g.title, g.description, g.total_amount,
       g.principal_amount, g.installments_count, g.purchase_date, g.first_reference, g.created_at, g.updated_at,
       COALESCE(SUM(CASE WHEN t.status='realizado' THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN t.status='pendente'  THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN t.status='cancelado' THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN t.status='pendente'  THEN t.amount ELSE 0 END),0)
FROM installment_groups g
LEFT JOIN transactions t ON t.installment_group_id = g.id`

func (r *SQLiteRepository) List(ctx context.Context, f Filter, p shared.Pagination) ([]GroupSummary, int, error) {
	where, args := "1=1", []any{}
	if f.CreditCardID != nil {
		where += " AND g.credit_card_id = ?"
		args = append(args, *f.CreditCardID)
	}
	having := statusHaving(f.Status)

	// total (grupos que casam WHERE + HAVING)
	countSQL := fmt.Sprintf(
		`SELECT COUNT(*) FROM (SELECT g.id FROM installment_groups g
		 LEFT JOIN transactions t ON t.installment_group_id = g.id
		 WHERE %s GROUP BY g.id HAVING %s) sub`, where, having)
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("installment repo: count: %w", err)
	}

	query := fmt.Sprintf("%s WHERE %s GROUP BY g.id HAVING %s ORDER BY g.%s %s LIMIT ? OFFSET ?",
		listAggSQL, where, having, p.OrderBy, p.Order)
	pageArgs := append(append([]any{}, args...), p.Limit, p.Offset())

	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("installment repo: list: %w", err)
	}
	defer rows.Close()

	out := []GroupSummary{}
	for rows.Next() {
		var s GroupSummary
		g, err := scanGroupSummary(rows.Scan, &s)
		if err != nil {
			return nil, 0, fmt.Errorf("installment repo: scan summary: %w", err)
		}
		s.Group = g
		s.Status = DeriveGroupStatus(StatusCounts{Pending: s.RemainingCount, Realized: s.PaidCount, Cancelled: s.CancelledCount})
		out = append(out, s)
	}
	return out, total, rows.Err()
}

// statusHaving devolve a cláusula HAVING para o filtro de status do grupo (sem args).
func statusHaving(status *GroupStatus) string {
	if status == nil {
		return "1=1"
	}
	const pending = "SUM(CASE WHEN t.status='pendente' THEN 1 ELSE 0 END)"
	const cancelled = "SUM(CASE WHEN t.status='cancelado' THEN 1 ELSE 0 END)"
	switch *status {
	case GroupActive:
		return pending + " > 0"
	case GroupSettled:
		return pending + " = 0 AND " + cancelled + " = 0"
	case GroupCancelled:
		return pending + " = 0 AND " + cancelled + " > 0"
	default:
		return "1=1"
	}
}

func (r *SQLiteRepository) UpdateSeries(ctx context.Context, id, title string, description *string, subcategoryID, parcelaType string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("installment repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx,
		`UPDATE installment_groups SET title=?, description=?, subcategory_id=?, updated_at=? WHERE id=?`,
		title, nullStr(description), subcategoryID, now, id)
	if err != nil {
		return fmt.Errorf("installment repo: update group: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInstallmentGroupNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE transactions SET title=?, subcategory_id=?, type=?, updated_at=? WHERE installment_group_id=?`,
		title, subcategoryID, parcelaType, now, id); err != nil {
		return fmt.Errorf("installment repo: update parcelas: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("installment repo: commit: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) CancelRemaining(ctx context.Context, id string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx,
		`UPDATE transactions SET status=?, updated_at=? WHERE installment_group_id=? AND status=?`,
		parcelaCancelled, now, id, parcelaPending)
	if err != nil {
		return 0, fmt.Errorf("installment repo: cancel remaining: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM installment_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("installment repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInstallmentGroupNotFound
	}
	return nil
}

// ─── Scan helpers ───────────────────────────────────────────────────────────

type scanner interface{ Scan(dest ...any) error }

func scanGroup(s scanner) (InstallmentGroup, error) {
	var g InstallmentGroup
	var desc sql.NullString
	var principal sql.NullInt64
	var createdAt, updatedAt string
	if err := s.Scan(&g.ID, &g.CreditCardID, &g.SubcategoryID, &g.Title, &desc, &g.TotalAmount,
		&principal, &g.InstallmentsCount, &g.PurchaseDate, &g.FirstReference, &createdAt, &updatedAt); err != nil {
		return InstallmentGroup{}, err
	}
	if desc.Valid {
		g.Description = &desc.String
	}
	if principal.Valid {
		g.PrincipalAmount = &principal.Int64
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	g.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return g, nil
}

// scanGroupSummary lê as colunas do grupo + os agregados para uma linha da listagem.
func scanGroupSummary(scan func(dest ...any) error, s *GroupSummary) (InstallmentGroup, error) {
	var g InstallmentGroup
	var desc sql.NullString
	var principal sql.NullInt64
	var createdAt, updatedAt string
	if err := scan(&g.ID, &g.CreditCardID, &g.SubcategoryID, &g.Title, &desc, &g.TotalAmount,
		&principal, &g.InstallmentsCount, &g.PurchaseDate, &g.FirstReference, &createdAt, &updatedAt,
		&s.PaidCount, &s.RemainingCount, &s.CancelledCount, &s.RemainingAmount); err != nil {
		return InstallmentGroup{}, err
	}
	if desc.Valid {
		g.Description = &desc.String
	}
	if principal.Valid {
		g.PrincipalAmount = &principal.Int64
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	g.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return g, nil
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
