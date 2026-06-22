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
    account_id, destination_account_id, credit_card_id, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func (r *SQLiteRepository) Create(ctx context.Context, t Transaction) error {
	_, err := r.db.ExecContext(ctx, createSQL,
		t.ID, t.Title, nullStr(deref(t.Description)),
		t.Amount, string(t.Type), t.SubcategoryID,
		string(t.PaymentMethod), string(t.Status),
		t.CompetenceDate, nullStr(deref(t.PaymentDate)),
		nullStr(deref(t.AccountID)),
		nullStr(deref(t.DestinationAccountID)),
		nullStr(deref(t.CreditCardID)),
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
       t.account_id, t.destination_account_id, t.credit_card_id, t.created_at, t.updated_at,
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
	if f.CompetenceDateFrom != nil {
		conds = append(conds, "t.competence_date >= ?")
		args = append(args, *f.CompetenceDateFrom)
	}
	if f.CompetenceDateTo != nil {
		conds = append(conds, "t.competence_date <= ?")
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

	return strings.Join(conds, " AND "), args
}

// ─── List ─────────────────────────────────────────────────────────────────────

const listBaseSQL = `
SELECT t.id, t.title, t.description, t.amount, t.type, t.subcategory_id,
       t.payment_method, t.status, t.competence_date, t.payment_date,
       t.account_id, t.destination_account_id, t.credit_card_id, t.created_at, t.updated_at,
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

func (r *SQLiteRepository) GetSummary(ctx context.Context, f TransactionFilter) (Summary, error) {
	where, args := buildFilter(f)
	// D14 (regime de caixa): compras no cartão NÃO entram no saldo financeiro — só o
	// pagamento da fatura (lançamento normal, sem credit_card_id) conta. A cláusula vai
	// só aqui, no summary; a List continua mostrando as compras de cartão normalmente.
	query := fmt.Sprintf("%s WHERE %s AND t.credit_card_id IS NULL GROUP BY t.type, t.status", summaryBaseSQL, where)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Summary{}, fmt.Errorf("transaction sqlite: get summary: %w", err)
	}
	defer rows.Close()

	var s Summary
	for rows.Next() {
		var typ, status string
		var totalAmount int64
		var count int
		if err := rows.Scan(&typ, &status, &totalAmount, &count); err != nil {
			return Summary{}, fmt.Errorf("transaction sqlite: summary scan: %w", err)
		}
		s.CountTotal += count
		switch {
		case status == string(StatusRealizado) && typ == string(TypeDespesa):
			s.TotalDespesas += totalAmount
		case status == string(StatusRealizado) && typ == string(TypeReceita):
			s.TotalReceitas += totalAmount
		case status == string(StatusPendente):
			s.TotalPendente += totalAmount
		}
	}
	s.SaldoPeriodo = s.TotalReceitas - s.TotalDespesas
	return s, rows.Err()
}

// ─── Scan helper ──────────────────────────────────────────────────────────────

// scanFunc allows sharing scanDetail between QueryRow.Scan and Rows.Scan.
type scanFunc func(dest ...any) error

func scanDetail(scan scanFunc) (TransactionDetail, error) {
	var d TransactionDetail
	var desc, payDate, accID, destAccID, creditCardID sql.NullString
	var createdAt, updatedAt string

	err := scan(
		&d.ID, &d.Title, &desc, &d.Amount, (*string)(&d.Type), &d.SubcategoryID,
		(*string)(&d.PaymentMethod), (*string)(&d.Status),
		&d.CompetenceDate, &payDate, &accID, &destAccID, &creditCardID,
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
