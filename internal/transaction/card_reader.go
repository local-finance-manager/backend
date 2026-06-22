package transaction

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CardReader satisfaz creditcard.CardTransactionReader via structural typing: lê a
// tabela transactions (dona da coluna credit_card_id) e devolve a projeção neutra
// shared.CardTransaction. Definido aqui (produtor) e injetado no main.go — o módulo
// transaction não importa creditcard.
type CardReader struct{ db *sql.DB }

// NewCardReader cria um CardReader.
func NewCardReader(db *sql.DB) *CardReader { return &CardReader{db: db} }

const cardTxnSQL = `
SELECT t.id, t.title, t.amount, t.competence_date, t.payment_date, t.status,
       t.subcategory_id, s.name, c.id, c.name, COALESCE(c.color,''), t.credit_card_id
FROM transactions t
JOIN subcategories s ON s.id = t.subcategory_id
JOIN categories    c ON c.id = s.category_id
WHERE t.credit_card_id IS NOT NULL`

// ListByCard retorna as compras de um cartão com competence_date em [from, to].
func (r *CardReader) ListByCard(ctx context.Context, cardID, from, to string) ([]shared.CardTransaction, error) {
	query := cardTxnSQL + " AND t.credit_card_id = ? AND t.competence_date >= ? AND t.competence_date <= ?"
	rows, err := r.db.QueryContext(ctx, query, cardID, from, to)
	if err != nil {
		return nil, fmt.Errorf("transaction card reader: list by card: %w", err)
	}
	defer rows.Close()

	out := []shared.CardTransaction{}
	for rows.Next() {
		t, err := scanCardTxn(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("transaction card reader: scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// HasTransactions informa se há qualquer lançamento vinculado ao cartão.
func (r *CardReader) HasTransactions(ctx context.Context, cardID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM transactions WHERE credit_card_id = ?)", cardID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("transaction card reader: has transactions: %w", err)
	}
	return exists == 1, nil
}

func scanCardTxn(scan scanFunc) (shared.CardTransaction, error) {
	var t shared.CardTransaction
	var payDate sql.NullString
	if err := scan(
		&t.ID, &t.Title, &t.Amount, &t.CompetenceDate, &payDate, &t.Status,
		&t.SubcategoryID, &t.SubcategoryName, &t.CategoryID, &t.CategoryName, &t.CategoryColor,
		&t.CreditCardID,
	); err != nil {
		return shared.CardTransaction{}, err
	}
	if payDate.Valid {
		t.PaymentDate = &payDate.String
	}
	return t, nil
}
