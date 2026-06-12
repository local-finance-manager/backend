package transaction_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

// ─── DB setup ─────────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS categories (
		id             TEXT PRIMARY KEY,
		name           TEXT NOT NULL,
		type           TEXT NOT NULL CHECK(type IN ('despesa','receita','transferencia')),
		icon           TEXT,
		color          TEXT,
		can_be_deleted INTEGER NOT NULL DEFAULT 1,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create categories: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS subcategories (
		id             TEXT PRIMARY KEY,
		category_id    TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
		name           TEXT NOT NULL,
		icon           TEXT,
		color          TEXT,
		can_be_deleted INTEGER NOT NULL DEFAULT 1,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create subcategories: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS transactions (
		id                      TEXT    PRIMARY KEY,
		title                   TEXT    NOT NULL,
		description             TEXT,
		amount                  INTEGER NOT NULL CHECK(amount > 0),
		type                    TEXT    NOT NULL CHECK(type IN ('despesa','receita','transferencia')),
		subcategory_id          TEXT    NOT NULL REFERENCES subcategories(id) ON DELETE RESTRICT,
		payment_method          TEXT    NOT NULL CHECK(payment_method IN (
		                            'pix','cartao_credito','cartao_debito',
		                            'dinheiro','ted','boleto','outros'
		                        )),
		status                  TEXT    NOT NULL DEFAULT 'pendente' CHECK(status IN ('pendente','realizado','cancelado')),
		competence_date         TEXT    NOT NULL,
		payment_date            TEXT,
		account_id              TEXT,
		destination_account_id  TEXT,
		created_at              TEXT    NOT NULL,
		updated_at              TEXT    NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create transactions: %v", err)
	}

	return db
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func insertTestSub(t *testing.T, db *sql.DB, catID, catName, catType, subID, subName string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT OR IGNORE INTO categories (id,name,type,can_be_deleted,created_at,updated_at)
		VALUES (?,?,?,1,?,?)`, catID, catName, catType, now, now)
	if err != nil {
		t.Fatalf("insert category %s: %v", catID, err)
	}
	_, err = db.Exec(`INSERT INTO subcategories (id,category_id,name,can_be_deleted,created_at,updated_at)
		VALUES (?,?,?,1,?,?)`, subID, catID, subName, now, now)
	if err != nil {
		t.Fatalf("insert subcategory %s: %v", subID, err)
	}
}

func mkTransaction(id, title, subID string, amount int64, typ transaction.TransactionType,
	pm transaction.PaymentMethod, status transaction.TransactionStatus,
	competenceDate string, paymentDate *string) transaction.Transaction {
	now := time.Now().UTC()
	return transaction.Transaction{
		ID:             id,
		Title:          title,
		Amount:         amount,
		Type:           typ,
		SubcategoryID:  subID,
		PaymentMethod:  pm,
		Status:         status,
		CompetenceDate: competenceDate,
		PaymentDate:    paymentDate,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func strPtr(s string) *string { return &s }

// ─── Create / Get ─────────────────────────────────────────────────────────────

func TestTransactionRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	txn := mkTransaction("txn-1", "Aluguel Jan", "sub-1", 150000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil)
	if err := repo.Create(ctx, txn); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "txn-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "txn-1" || got.Title != "Aluguel Jan" || got.Amount != 150000 {
		t.Errorf("unexpected: %+v", got)
	}
	if got.Subcategory.ID != "sub-1" || got.Subcategory.Category.ID != "cat-1" {
		t.Errorf("join data missing: %+v", got.Subcategory)
	}
}

func TestTransactionRepo_GetNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)

	_, err := repo.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestTransactionRepo_NullableFields(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	txn := mkTransaction("txn-null", "Sem desc", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil)
	txn.Description = nil
	txn.AccountID = nil
	txn.DestinationAccountID = nil
	repo.Create(ctx, txn)

	got, err := repo.Get(ctx, "txn-null")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != nil || got.AccountID != nil || got.DestinationAccountID != nil {
		t.Errorf("expected nil optional fields, got: %+v", got)
	}
}

func TestTransactionRepo_PaymentDateStored(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	txn := mkTransaction("txn-pd", "Pago", "sub-1", 2000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusRealizado, "2026-01-01", strPtr("2026-01-05"))
	repo.Create(ctx, txn)

	got, err := repo.Get(ctx, "txn-pd")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PaymentDate == nil || *got.PaymentDate != "2026-01-05" {
		t.Errorf("paymentDate: got %v, want 2026-01-05", got.PaymentDate)
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func TestTransactionRepo_Update(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	txn := mkTransaction("txn-upd", "Original", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil)
	repo.Create(ctx, txn)

	txn.Title = "Atualizado"
	txn.Amount = 2000
	txn.Status = transaction.StatusRealizado
	txn.PaymentDate = strPtr("2026-01-10")
	txn.UpdatedAt = time.Now().UTC()
	if err := repo.Update(ctx, txn); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := repo.Get(ctx, "txn-upd")
	if got.Title != "Atualizado" || got.Amount != 2000 || got.Status != transaction.StatusRealizado {
		t.Errorf("update not reflected: %+v", got)
	}
}

func TestTransactionRepo_UpdateNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)
	txn := mkTransaction("no-id", "X", "sub-x", 100,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil)
	err := repo.Update(context.Background(), txn)
	if err == nil {
		t.Error("expected not-found when updating nonexistent transaction")
	}
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestTransactionRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	txn := mkTransaction("txn-del", "Del", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil)
	repo.Create(ctx, txn)

	if err := repo.Delete(ctx, "txn-del"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := repo.Get(ctx, "txn-del")
	if err == nil {
		t.Error("expected not-found after delete")
	}
}

func TestTransactionRepo_DeleteNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)
	err := repo.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error when deleting nonexistent")
	}
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestTransactionRepo_List_Basic(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		txn := mkTransaction(fmt.Sprintf("txn-%d", i), fmt.Sprintf("Txn %d", i), "sub-1",
			int64(1000*i), transaction.TypeDespesa, transaction.MethodPix,
			transaction.StatusPendente, fmt.Sprintf("2026-01-%02d", i), nil)
		repo.Create(ctx, txn)
	}

	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("len: got %d, want 3", len(list))
	}
}

func TestTransactionRepo_List_Pagination(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		txn := mkTransaction(fmt.Sprintf("txn-%d", i), fmt.Sprintf("Txn %02d", i), "sub-1",
			1000, transaction.TypeDespesa, transaction.MethodPix,
			transaction.StatusPendente, fmt.Sprintf("2026-01-%02d", i), nil)
		repo.Create(ctx, txn)
	}

	p := shared.Pagination{Page: 2, Limit: 2, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("page 2 len: got %d, want 2", len(list))
	}
}

func TestTransactionRepo_List_FilterByStatus(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkTransaction("txn-p", "Pendente", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil))
	repo.Create(ctx, mkTransaction("txn-r", "Realizado", "sub-1", 2000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusRealizado, "2026-01-02", strPtr("2026-01-02")))

	status := transaction.StatusPendente
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{Status: &status}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Status != transaction.StatusPendente {
		t.Errorf("expected 1 pendente, got %d", len(list))
	}
}

func TestTransactionRepo_List_FilterByCategoryID(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-expense", "Moradia", "despesa", "sub-exp", "Aluguel")
	insertTestSub(t, db, "cat-income", "Salário", "receita", "sub-inc", "Freelance")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkTransaction("txn-exp", "Aluguel", "sub-exp", 150000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil))
	repo.Create(ctx, mkTransaction("txn-inc", "Freelance", "sub-inc", 50000,
		transaction.TypeReceita, transaction.MethodTed, transaction.StatusRealizado, "2026-01-02", strPtr("2026-01-02")))

	catID := "cat-expense"
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{CategoryID: &catID}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "txn-exp" {
		t.Errorf("expected only txn-exp, got %v", list)
	}
}

func TestTransactionRepo_List_FilterByCompetenceDate(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkTransaction("txn-jan", "Jan", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-15", nil))
	repo.Create(ctx, mkTransaction("txn-feb", "Feb", "sub-1", 2000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-02-15", nil))

	from := "2026-02-01"
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{CompetenceDateFrom: &from}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "txn-feb" {
		t.Errorf("expected only txn-feb, got %v", list)
	}
}

func TestTransactionRepo_List_SearchByTitle(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkTransaction("txn-a", "Aluguel Mensal", "sub-1", 1000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusPendente, "2026-01-01", nil))
	repo.Create(ctx, mkTransaction("txn-b", "Conta de Luz", "sub-1", 500,
		transaction.TypeDespesa, transaction.MethodBoleto, transaction.StatusPendente, "2026-01-02", nil))

	search := "aluguel"
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(ctx, transaction.TransactionFilter{Search: &search}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "txn-a" {
		t.Errorf("expected only txn-a, got %v", list)
	}
}

func TestTransactionRepo_List_Empty(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)

	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"}
	list, err := repo.List(context.Background(), transaction.TransactionFilter{}, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty slice, got %v", list)
	}
}

// ─── GetSummary ───────────────────────────────────────────────────────────────

// insertTxn inserts a row with minimal boilerplate for summary tests.
func insertTxn(t *testing.T, repo *transaction.SQLiteRepository, id, subID string, amount int64,
	typ transaction.TransactionType, status transaction.TransactionStatus, payDate *string) {
	t.Helper()
	txn := mkTransaction(id, id, subID, amount, typ, transaction.MethodPix, status, "2026-01-01", payDate)
	if err := repo.Create(context.Background(), txn); err != nil {
		t.Fatalf("insertTxn %s: %v", id, err)
	}
}

func TestTransactionRepo_GetSummary(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-exp", "Despesas", "despesa", "sub-exp", "Aluguel")
	insertTestSub(t, db, "cat-inc", "Receitas", "receita", "sub-inc", "Salário")
	repo := transaction.NewSQLiteRepository(db)

	// realizado: 2 despesas (150000+50000=200000), 1 receita (300000)
	// pendente:  1 despesa (80000)
	// cancelado: 1 despesa (999999) — not counted in any total
	insertTxn(t, repo, "r-exp-1", "sub-exp", 150000, transaction.TypeDespesa, transaction.StatusRealizado, strPtr("2026-01-01"))
	insertTxn(t, repo, "r-exp-2", "sub-exp", 50000, transaction.TypeDespesa, transaction.StatusRealizado, strPtr("2026-01-01"))
	insertTxn(t, repo, "r-inc-1", "sub-inc", 300000, transaction.TypeReceita, transaction.StatusRealizado, strPtr("2026-01-01"))
	insertTxn(t, repo, "p-exp-1", "sub-exp", 80000, transaction.TypeDespesa, transaction.StatusPendente, nil)
	insertTxn(t, repo, "c-exp-1", "sub-exp", 999999, transaction.TypeDespesa, transaction.StatusCancelado, nil)

	s, err := repo.GetSummary(context.Background(), transaction.TransactionFilter{})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.TotalDespesas != 200000 {
		t.Errorf("TotalDespesas: got %d, want 200000", s.TotalDespesas)
	}
	if s.TotalReceitas != 300000 {
		t.Errorf("TotalReceitas: got %d, want 300000", s.TotalReceitas)
	}
	if s.SaldoPeriodo != 100000 {
		t.Errorf("SaldoPeriodo: got %d, want 100000", s.SaldoPeriodo)
	}
	if s.TotalPendente != 80000 {
		t.Errorf("TotalPendente: got %d, want 80000", s.TotalPendente)
	}
	if s.CountTotal != 5 {
		t.Errorf("CountTotal: got %d, want 5", s.CountTotal)
	}
}

func TestTransactionRepo_GetSummary_WithFilter(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-exp", "Despesas", "despesa", "sub-exp", "Aluguel")
	insertTestSub(t, db, "cat-inc", "Receitas", "receita", "sub-inc", "Salário")
	repo := transaction.NewSQLiteRepository(db)

	insertTxn(t, repo, "exp-1", "sub-exp", 100000, transaction.TypeDespesa, transaction.StatusRealizado, strPtr("2026-01-01"))
	insertTxn(t, repo, "inc-1", "sub-inc", 200000, transaction.TypeReceita, transaction.StatusRealizado, strPtr("2026-01-01"))

	typ := transaction.TypeDespesa
	s, err := repo.GetSummary(context.Background(), transaction.TransactionFilter{Type: &typ})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.TotalDespesas != 100000 {
		t.Errorf("TotalDespesas: got %d, want 100000", s.TotalDespesas)
	}
	if s.TotalReceitas != 0 {
		t.Errorf("TotalReceitas: got %d, want 0", s.TotalReceitas)
	}
	if s.CountTotal != 1 {
		t.Errorf("CountTotal: got %d, want 1", s.CountTotal)
	}
}

func TestTransactionRepo_GetSummary_Empty(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)

	s, err := repo.GetSummary(context.Background(), transaction.TransactionFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.TotalDespesas != 0 || s.TotalReceitas != 0 || s.SaldoPeriodo != 0 ||
		s.TotalPendente != 0 || s.CountTotal != 0 {
		t.Errorf("expected zero summary, got: %+v", s)
	}
}
