package creditcard_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/creditcard"
	"github.com/local-finance-manager/backend/internal/shared"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	stmts := []string{
		`CREATE TABLE credit_cards (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, brand TEXT NOT NULL,
			last_four_digits TEXT, issuer TEXT, credit_limit INTEGER NOT NULL,
			closing_day INTEGER NOT NULL, due_day INTEGER NOT NULL,
			color TEXT, icon TEXT, archived INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE credit_card_invoice_payment (
			id TEXT PRIMARY KEY,
			credit_card_id TEXT NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
			reference TEXT NOT NULL, amount INTEGER NOT NULL,
			payment_date TEXT NOT NULL, transaction_id TEXT, created_at TEXT NOT NULL
		)`,
		`CREATE TABLE transactions (
			id TEXT PRIMARY KEY, title TEXT NOT NULL,
			description TEXT,
			amount INTEGER NOT NULL DEFAULT 0,
			type TEXT,
			subcategory_id TEXT,
			payment_method TEXT,
			status TEXT NOT NULL DEFAULT 'pendente',
			competence_date TEXT,
			payment_date TEXT,
			credit_card_id TEXT REFERENCES credit_cards(id) ON DELETE RESTRICT,
			created_at TEXT,
			updated_at TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

func mkCard(id, name string, archived bool) creditcard.CreditCard {
	now := time.Now().UTC()
	return creditcard.CreditCard{
		ID: id, Name: name, Brand: creditcard.BrandMastercard,
		CreditLimit: 500000, ClosingDay: 3, DueDay: 10, Archived: archived,
		CreatedAt: now, UpdatedAt: now,
	}
}

func ptrS(s string) *string { return &s }

// ─── CreditCard CRUD ────────────────────────────────────────────────────────

func TestCardRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()

	c := mkCard("c1", "Nubank", false)
	c.LastFourDigits = ptrS("1234")
	c.Issuer = ptrS("Nubank")
	c.Color = ptrS("#820AD1")
	c.Icon = ptrS("credit-card")
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "c1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Nubank" || got.CreditLimit != 500000 || got.ClosingDay != 3 {
		t.Errorf("unexpected: %+v", got)
	}
	if got.LastFourDigits == nil || *got.LastFourDigits != "1234" {
		t.Errorf("lastFour: %v", got.LastFourDigits)
	}
	if got.Issuer == nil || *got.Issuer != "Nubank" {
		t.Errorf("issuer: %v", got.Issuer)
	}
}

func TestCardRepo_GetNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	if _, err := repo.Get(context.Background(), "nope"); err == nil {
		t.Error("expected not-found")
	}
}

func TestCardRepo_NullableFields(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()

	if err := repo.Create(ctx, mkCard("c1", "Bare", false)); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := repo.Get(ctx, "c1")
	if got.LastFourDigits != nil || got.Issuer != nil || got.Color != nil || got.Icon != nil {
		t.Errorf("expected nil optionals: %+v", got)
	}
}

func TestCardRepo_Update(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkCard("c1", "Old", false))
	c := mkCard("c1", "New", false)
	c.CreditLimit = 999900
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := repo.Get(ctx, "c1")
	if got.Name != "New" || got.CreditLimit != 999900 {
		t.Errorf("update not reflected: %+v", got)
	}
}

func TestCardRepo_UpdateNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	if err := repo.Update(context.Background(), mkCard("ghost", "X", false)); err == nil {
		t.Error("expected not-found")
	}
}

func TestCardRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkCard("c1", "Del", false))
	if err := repo.Delete(ctx, "c1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, "c1"); err == nil {
		t.Error("expected not-found after delete")
	}
}

func TestCardRepo_DeleteNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	if err := repo.Delete(context.Background(), "nope"); err == nil {
		t.Error("expected not-found")
	}
}

func TestCardRepo_DeleteBlockedByFK(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkCard("c1", "WithTxn", false))
	if _, err := db.Exec(`INSERT INTO transactions (id,title,credit_card_id) VALUES ('t1','Compra','c1')`); err != nil {
		t.Fatalf("insert txn: %v", err)
	}
	err := repo.Delete(ctx, "c1")
	if err != creditcard.ErrCardHasTransactions {
		t.Errorf("expected ErrCardHasTransactions, got %v", err)
	}
}

func TestCardRepo_SetArchived(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkCard("c1", "ToArchive", false))

	if err := repo.SetArchived(ctx, "c1", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	got, _ := repo.Get(ctx, "c1")
	if !got.Archived {
		t.Error("expected archived=true")
	}
	repo.SetArchived(ctx, "c1", false)
	got, _ = repo.Get(ctx, "c1")
	if got.Archived {
		t.Error("expected archived=false")
	}
}

func TestCardRepo_SetArchivedNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	if err := repo.SetArchived(context.Background(), "nope", true); err == nil {
		t.Error("expected not-found")
	}
}

func TestCardRepo_List_FilterArchived(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkCard("active1", "Active 1", false))
	repo.Create(ctx, mkCard("active2", "Active 2", false))
	repo.Create(ctx, mkCard("arch1", "Archived 1", true))

	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "name", Order: "ASC"}

	actives, total, err := repo.List(ctx, false, p)
	if err != nil {
		t.Fatalf("list actives: %v", err)
	}
	if total != 2 || len(actives) != 2 {
		t.Errorf("actives: got total=%d len=%d, want 2/2", total, len(actives))
	}

	archived, total, err := repo.List(ctx, true, p)
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if total != 1 || len(archived) != 1 || archived[0].ID != "arch1" {
		t.Errorf("archived: got total=%d len=%d", total, len(archived))
	}
}

func TestCardRepo_List_Pagination(t *testing.T) {
	db := newTestDB(t)
	repo := creditcard.NewSQLiteCreditCardRepository(db)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		repo.Create(ctx, mkCard(fmt.Sprintf("c%d", i), fmt.Sprintf("Card %02d", i), false))
	}
	p := shared.Pagination{Page: 2, Limit: 2, OrderBy: "name", Order: "ASC"}
	cards, total, err := repo.List(ctx, false, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 || len(cards) != 2 {
		t.Errorf("got total=%d len=%d, want 5/2", total, len(cards))
	}
}

// ─── InvoicePayment ─────────────────────────────────────────────────────────

// ─── Marcação de pagamento de fatura (Opção 1) ──────────────────────────────

// insertCompra cria uma compra de cartão (lançamento) no estado informado.
func insertCompra(t *testing.T, db *sql.DB, id, status string, amount int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO transactions (id, title, amount, type, subcategory_id, payment_method,
			status, competence_date, credit_card_id, created_at, updated_at)
		 VALUES (?, ?, ?, 'despesa', 'sub-x', 'cartao_credito', ?, '2026-06-20', 'c1', ?, ?)`,
		id, id, amount, status, "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z")
	if err != nil {
		t.Fatalf("insert compra %s: %v", id, err)
	}
}

func txnStatus(t *testing.T, db *sql.DB, id string) (status string, payDate sql.NullString) {
	t.Helper()
	if err := db.QueryRow("SELECT status, payment_date FROM transactions WHERE id = ?", id).
		Scan(&status, &payDate); err != nil {
		t.Fatalf("query status %s: %v", id, err)
	}
	return status, payDate
}

func TestInvoicePaymentRepo_MarkInvoicePaid(t *testing.T) {
	db := newTestDB(t)
	creditcard.NewSQLiteCreditCardRepository(db).Create(context.Background(), mkCard("c1", "C", false))
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	insertCompra(t, db, "compra-1", "pendente", 20000)
	insertCompra(t, db, "compra-2", "pendente", 10000)

	if err := payRepo.MarkInvoicePaid(ctx, []string{"compra-1", "compra-2"}, "2026-07-15"); err != nil {
		t.Fatalf("MarkInvoicePaid: %v", err)
	}
	for _, id := range []string{"compra-1", "compra-2"} {
		st, pd := txnStatus(t, db, id)
		if st != "realizado" || pd.String != "2026-07-15" {
			t.Errorf("compra %s: status=%s payment_date=%s", id, st, pd.String)
		}
	}
	// lista vazia é no-op.
	if err := payRepo.MarkInvoicePaid(ctx, nil, "2026-07-15"); err != nil {
		t.Errorf("MarkInvoicePaid vazio: %v", err)
	}
}

func TestInvoicePaymentRepo_RevertInvoicePayment(t *testing.T) {
	db := newTestDB(t)
	creditcard.NewSQLiteCreditCardRepository(db).Create(context.Background(), mkCard("c1", "C", false))
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	insertCompra(t, db, "compra-1", "realizado", 20000)
	// realizada com data → reverte para pendente sem data.
	db.Exec("UPDATE transactions SET payment_date='2026-07-15' WHERE id='compra-1'")

	if err := payRepo.RevertInvoicePayment(ctx, []string{"compra-1"}); err != nil {
		t.Fatalf("RevertInvoicePayment: %v", err)
	}
	st, pd := txnStatus(t, db, "compra-1")
	if st != "pendente" || pd.Valid {
		t.Errorf("revert: status=%s payment_date=%v", st, pd)
	}
	if err := payRepo.RevertInvoicePayment(ctx, nil); err != nil {
		t.Errorf("RevertInvoicePayment vazio: %v", err)
	}
}
