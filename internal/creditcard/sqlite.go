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
	_ CreditCardRepository     = (*SQLiteCreditCardRepository)(nil)
	_ InvoicePaymentRepository = (*SQLiteInvoicePaymentRepository)(nil)
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

// SQLiteInvoicePaymentRepository implementa InvoicePaymentRepository.
type SQLiteInvoicePaymentRepository struct{ db *sql.DB }

// NewSQLiteInvoicePaymentRepository cria um SQLiteInvoicePaymentRepository.
func NewSQLiteInvoicePaymentRepository(db *sql.DB) *SQLiteInvoicePaymentRepository {
	return &SQLiteInvoicePaymentRepository{db: db}
}

const selectPaymentCols = `
SELECT reference, payment_date, transaction_id, created_at
FROM credit_card_invoice_payments`

func (r *SQLiteInvoicePaymentRepository) Get(ctx context.Context, cardID, reference string) (*InvoicePayment, error) {
	row := r.db.QueryRowContext(ctx,
		selectPaymentCols+" WHERE credit_card_id = ? AND reference = ?", cardID, reference)
	p, err := scanPayment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("creditcard repo: get payment: %w", err)
	}
	return &p, nil
}

func (r *SQLiteInvoicePaymentRepository) ListByCard(ctx context.Context, cardID string) (map[string]*InvoicePayment, error) {
	rows, err := r.db.QueryContext(ctx, selectPaymentCols+" WHERE credit_card_id = ?", cardID)
	if err != nil {
		return nil, fmt.Errorf("creditcard repo: list payments: %w", err)
	}
	defer rows.Close()

	out := map[string]*InvoicePayment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("creditcard repo: list payments scan: %w", err)
		}
		pCopy := p
		out[p.Reference] = &pCopy
	}
	return out, rows.Err()
}

const upsertPaymentSQL = `
INSERT INTO credit_card_invoice_payments (credit_card_id, reference, payment_date, transaction_id, created_at)
VALUES (?,?,?,?,?)
ON CONFLICT(credit_card_id, reference) DO UPDATE SET
    payment_date = excluded.payment_date,
    transaction_id = excluded.transaction_id`

func (r *SQLiteInvoicePaymentRepository) Upsert(ctx context.Context, cardID string, p InvoicePayment) error {
	_, err := r.db.ExecContext(ctx, upsertPaymentSQL,
		cardID, p.Reference, p.PaymentDate, toNullString(ptrVal(p.TransactionID)),
		p.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creditcard repo: upsert payment: %w", err)
	}
	return nil
}

func (r *SQLiteInvoicePaymentRepository) Delete(ctx context.Context, cardID, reference string) error {
	res, err := r.db.ExecContext(ctx,
		"DELETE FROM credit_card_invoice_payments WHERE credit_card_id = ? AND reference = ?",
		cardID, reference)
	if err != nil {
		return fmt.Errorf("creditcard repo: delete payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvoiceNotFound
	}
	return nil
}

// ─── Pagamento atômico de fatura (E1) ───────────────────────────────────────
// Estes dois métodos escrevem na tabela `transactions` (posse do módulo transaction)
// DENTRO da mesma tx do registro de pagamento. É a exceção consciente da Opção A (igual
// ao installment): a atomicidade cross-module (RF-PAGFAT-04) só é possível com uma única
// transação — ports em módulos distintos rodariam em txs separadas.

const insertPaymentTxnSQL = `
INSERT INTO transactions
	(id, title, description, amount, type, subcategory_id, payment_method, status,
	 competence_date, payment_date, created_at, updated_at)
VALUES (?,?,?,?,?,?,?, 'realizado', ?,?,?,?)`

func (r *SQLiteInvoicePaymentRepository) PayInvoiceAtomic(ctx context.Context, in AtomicPayInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("creditcard repo: pay atomic: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op após Commit

	ts := in.Payment.CreatedAt.UTC().Format(time.RFC3339)

	// 1. Baixa em lote: compras pendentes do ciclo → realizado, com a data informada.
	if len(in.RealizeIDs) > 0 {
		q := "UPDATE transactions SET status='realizado', payment_date=?, updated_at=? WHERE id IN (" +
			placeholders(len(in.RealizeIDs)) + ")"
		args := make([]any, 0, len(in.RealizeIDs)+2)
		args = append(args, in.RealizeAt, ts)
		for _, id := range in.RealizeIDs {
			args = append(args, id)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("creditcard repo: pay atomic: realize: %w", err)
		}
	}

	// 2. Lançamento de pagamento (realizado; sem cartão; type derivado da subcategoria).
	p := in.Payment
	if _, err := tx.ExecContext(ctx, insertPaymentTxnSQL,
		p.ID, p.Title, toNullString(ptrVal(p.Description)), p.Amount, p.Type, p.SubcategoryID,
		p.PaymentMethod, p.CompetenceDate, p.PaymentDate, ts, ts,
	); err != nil {
		return fmt.Errorf("creditcard repo: pay atomic: insert payment txn: %w", err)
	}

	// 3. Registro de pagamento da fatura, apontando para o lançamento criado.
	if _, err := tx.ExecContext(ctx, upsertPaymentSQL,
		in.CardID, in.Reference, p.PaymentDate, p.ID, ts,
	); err != nil {
		return fmt.Errorf("creditcard repo: pay atomic: upsert payment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("creditcard repo: pay atomic: commit: %w", err)
	}
	return nil
}

func (r *SQLiteInvoicePaymentRepository) UndoPaymentAtomic(ctx context.Context, in AtomicUndoInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("creditcard repo: undo atomic: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op após Commit

	now := time.Now().UTC().Format(time.RFC3339)

	// 1. Compras realizadas do ciclo → pendente, limpando payment_date.
	if len(in.RevertIDs) > 0 {
		q := "UPDATE transactions SET status='pendente', payment_date=NULL, updated_at=? WHERE id IN (" +
			placeholders(len(in.RevertIDs)) + ")"
		args := make([]any, 0, len(in.RevertIDs)+1)
		args = append(args, now)
		for _, id := range in.RevertIDs {
			args = append(args, id)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("creditcard repo: undo atomic: revert: %w", err)
		}
	}

	// 2. Exclui o lançamento de pagamento criado no pay.
	if in.PaymentTxnID != "" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE id = ?", in.PaymentTxnID); err != nil {
			return fmt.Errorf("creditcard repo: undo atomic: delete payment txn: %w", err)
		}
	}

	// 3. Remove o registro de pagamento da fatura.
	res, err := tx.ExecContext(ctx,
		"DELETE FROM credit_card_invoice_payments WHERE credit_card_id = ? AND reference = ?",
		in.CardID, in.Reference)
	if err != nil {
		return fmt.Errorf("creditcard repo: undo atomic: delete payment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvoiceNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("creditcard repo: undo atomic: commit: %w", err)
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

func scanPayment(s scanner) (InvoicePayment, error) {
	var p InvoicePayment
	var txnID sql.NullString
	var createdAt string
	if err := s.Scan(&p.Reference, &p.PaymentDate, &txnID, &createdAt); err != nil {
		return InvoicePayment{}, err
	}
	p.TransactionID = nullToPtr(txnID)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return p, nil
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
