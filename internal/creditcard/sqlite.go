package creditcard

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

var (
	_ CreditCardRepository = (*SQLiteCreditCardRepository)(nil)
	_ InvoicePaymentWriter = (*SQLiteInvoicePaymentRepository)(nil)
)

// ─── SQLiteCreditCardRepository ─────────────────────────────────────────────

// SQLiteCreditCardRepository implementa CreditCardRepository usando SQLite.
type SQLiteCreditCardRepository struct{ db *sql.DB }

// NewSQLiteCreditCardRepository cria um SQLiteCreditCardRepository.
func NewSQLiteCreditCardRepository(db *sql.DB) *SQLiteCreditCardRepository {
	return &SQLiteCreditCardRepository{db: db}
}

const insertCardSQL = `
INSERT INTO credit_cards (
    id, name, brand, last_four_digits, issuer, credit_limit,
    closing_day, due_day, color, icon, archived, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`

func (r *SQLiteCreditCardRepository) Create(ctx context.Context, c CreditCard) error {
	_, err := r.db.ExecContext(ctx, insertCardSQL,
		c.ID, c.Name, string(c.Brand), toNullString(ptrVal(c.LastFourDigits)),
		toNullString(ptrVal(c.Issuer)), c.CreditLimit, c.ClosingDay, c.DueDay,
		toNullString(ptrVal(c.Color)), toNullString(ptrVal(c.Icon)),
		boolToInt(c.Archived),
		c.CreatedAt.UTC().Format(time.RFC3339), c.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creditcard repo: create: %w", err)
	}
	return nil
}

const selectCardCols = `
SELECT id, name, brand, last_four_digits, issuer, credit_limit,
       closing_day, due_day, color, icon, archived, created_at, updated_at
FROM credit_cards`

func (r *SQLiteCreditCardRepository) Get(ctx context.Context, id string) (CreditCard, error) {
	row := r.db.QueryRowContext(ctx, selectCardCols+" WHERE id = ?", id)
	c, err := scanCreditCard(row)
	if err == sql.ErrNoRows {
		return CreditCard{}, ErrCreditCardNotFound
	}
	if err != nil {
		return CreditCard{}, fmt.Errorf("creditcard repo: get: %w", err)
	}
	return c, nil
}

func (r *SQLiteCreditCardRepository) List(ctx context.Context, archived bool, p shared.Pagination) ([]CreditCard, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM credit_cards WHERE archived = ?", boolToInt(archived),
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("creditcard repo: count: %w", err)
	}

	// p.OrderBy e p.Order são validados na borda (allowlist em ParsePagination).
	query := fmt.Sprintf("%s WHERE archived = ? ORDER BY %s %s LIMIT ? OFFSET ?",
		selectCardCols, p.OrderBy, p.Order)
	rows, err := r.db.QueryContext(ctx, query, boolToInt(archived), p.Limit, p.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("creditcard repo: list: %w", err)
	}
	defer rows.Close()

	cards := []CreditCard{}
	for rows.Next() {
		c, err := scanCreditCard(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("creditcard repo: list scan: %w", err)
		}
		cards = append(cards, c)
	}
	return cards, total, rows.Err()
}

const updateCardSQL = `
UPDATE credit_cards SET
    name=?, brand=?, last_four_digits=?, issuer=?, credit_limit=?,
    closing_day=?, due_day=?, color=?, icon=?, updated_at=?
WHERE id=?`

func (r *SQLiteCreditCardRepository) Update(ctx context.Context, c CreditCard) error {
	res, err := r.db.ExecContext(ctx, updateCardSQL,
		c.Name, string(c.Brand), toNullString(ptrVal(c.LastFourDigits)),
		toNullString(ptrVal(c.Issuer)), c.CreditLimit, c.ClosingDay, c.DueDay,
		toNullString(ptrVal(c.Color)), toNullString(ptrVal(c.Icon)),
		c.UpdatedAt.UTC().Format(time.RFC3339), c.ID,
	)
	if err != nil {
		return fmt.Errorf("creditcard repo: update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCreditCardNotFound
	}
	return nil
}

func (r *SQLiteCreditCardRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM credit_cards WHERE id = ?", id)
	if err != nil {
		if isForeignKeyConstraintError(err) {
			return ErrCardHasTransactions
		}
		return fmt.Errorf("creditcard repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCreditCardNotFound
	}
	return nil
}

func (r *SQLiteCreditCardRepository) SetArchived(ctx context.Context, id string, archived bool) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE credit_cards SET archived = ?, updated_at = ? WHERE id = ?",
		boolToInt(archived), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("creditcard repo: set archived: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCreditCardNotFound
	}
	return nil
}

// ─── SQLiteInvoicePaymentRepository ─────────────────────────────────────────

// SQLiteInvoicePaymentRepository implementa InvoicePaymentWriter (marca/reverte compras).
type SQLiteInvoicePaymentRepository struct{ db *sql.DB }

// NewSQLiteInvoicePaymentRepository cria um SQLiteInvoicePaymentRepository.
func NewSQLiteInvoicePaymentRepository(db *sql.DB) *SQLiteInvoicePaymentRepository {
	return &SQLiteInvoicePaymentRepository{db: db}
}

// ─── Marcação de pagamento de fatura (Opção 1) ──────────────────────────────
// Pagar/desfazer uma fatura é só mudar status/data das COMPRAS dela — não há lançamento
// sintético. Escreve na tabela `transactions` (posse do módulo transaction): exceção
// consciente de posse (Opção A, igual ao installment).

// MarkInvoicePaid marca as compras (ids) como realizado com a data de pagamento informada.
func (r *SQLiteInvoicePaymentRepository) MarkInvoicePaid(ctx context.Context, ids []string, paymentDate string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	q := "UPDATE transactions SET status='realizado', payment_date=?, updated_at=? WHERE id IN (" +
		placeholders(len(ids)) + ")"
	args := make([]any, 0, len(ids)+2)
	args = append(args, paymentDate, now)
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("creditcard repo: mark invoice paid: %w", err)
	}
	return nil
}

// RevertInvoicePayment volta as compras (ids) para pendente, limpando a data de pagamento.
func (r *SQLiteInvoicePaymentRepository) RevertInvoicePayment(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	q := "UPDATE transactions SET status='pendente', payment_date=NULL, updated_at=? WHERE id IN (" +
		placeholders(len(ids)) + ")"
	args := make([]any, 0, len(ids)+1)
	args = append(args, now)
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("creditcard repo: revert invoice payment: %w", err)
	}
	return nil
}

// placeholders devolve "?,?,...,?" com n interrogações para cláusulas IN parametrizadas.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// ─── Scan helpers ───────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanCreditCard(s scanner) (CreditCard, error) {
	var c CreditCard
	var brand string
	var lastFour, issuer, color, icon sql.NullString
	var archived int
	var createdAt, updatedAt string

	if err := s.Scan(&c.ID, &c.Name, &brand, &lastFour, &issuer, &c.CreditLimit,
		&c.ClosingDay, &c.DueDay, &color, &icon, &archived, &createdAt, &updatedAt); err != nil {
		return CreditCard{}, err
	}
	c.Brand = Brand(brand)
	c.LastFourDigits = nullToPtr(lastFour)
	c.Issuer = nullToPtr(issuer)
	c.Color = nullToPtr(color)
	c.Icon = nullToPtr(icon)
	c.Archived = archived != 0
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func ptrVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isForeignKeyConstraintError(err error) bool {
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
