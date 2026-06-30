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

// addEntry registra um pagamento no ledger via AddPaymentAtomic (helper de teste).
func addEntry(t *testing.T, payRepo *creditcard.SQLiteInvoicePaymentRepository, cardID, ref, payID string, amount int64) {
	t.Helper()
	pay := mkPaymentTxn(payID)
	pay.Amount = amount
	entry := creditcard.InvoicePayment{
		ID: "e-" + payID, Reference: ref, Amount: amount, PaymentDate: pay.PaymentDate,
		TransactionID: &pay.ID, CreatedAt: time.Now().UTC(),
	}
	if err := payRepo.AddPaymentAtomic(context.Background(), creditcard.AtomicAddPaymentInput{
		CardID: cardID, Entry: entry, Payment: pay,
	}); err != nil {
		t.Fatalf("addEntry %s: %v", payID, err)
	}
}

func TestPaymentRepo_ListByCard(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Card", false))

	addEntry(t, payRepo, "c1", "2026-05", "pay-a", 10000)
	addEntry(t, payRepo, "c1", "2026-06", "pay-b", 20000)
	addEntry(t, payRepo, "c1", "2026-06", "pay-c", 5000) // 2º pagamento da mesma fatura (ledger)

	m, err := payRepo.ListByCard(ctx, "c1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(m["2026-05"]) != 1 || len(m["2026-06"]) != 2 {
		t.Errorf("ledger por referência inesperado: %+v", m)
	}
}

func TestPaymentRepo_CascadeOnCardDelete(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Card", false))
	addEntry(t, payRepo, "c1", "2026-06", "pay-a", 10000)

	// cartão sem lançamentos de compra pode ser excluído → pagamentos vão junto (CASCADE)
	if err := cardRepo.Delete(ctx, "c1"); err != nil {
		t.Fatalf("delete card: %v", err)
	}
	m, _ := payRepo.ListByCard(ctx, "c1")
	if len(m) != 0 {
		t.Errorf("expected payments cascaded away, got %d", len(m))
	}
}

// ─── Pagamento atômico de fatura (E1) ───────────────────────────────────────

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

func mkPaymentTxn(id string) creditcard.PaymentTxn {
	return creditcard.PaymentTxn{
		ID: id, Title: "Pagamento de Fatura — 2026-07", Amount: 30000,
		Type: "transferencia", SubcategoryID: "sub-trf-pgto", PaymentMethod: "outros",
		CompetenceDate: "2026-07-15", PaymentDate: "2026-07-15", CreatedAt: time.Now().UTC(),
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

func TestInvoicePaymentRepo_AddPaymentAtomic_FullSettle(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Nubank", false))
	insertCompra(t, db, "compra-1", "pendente", 20000)
	insertCompra(t, db, "compra-2", "pendente", 10000)

	pay := mkPaymentTxn("pay-1") // amount 30000
	entry := creditcard.InvoicePayment{ID: "e1", Reference: "2026-07", Amount: 30000, PaymentDate: "2026-07-15", TransactionID: &pay.ID, CreatedAt: time.Now().UTC()}
	err := payRepo.AddPaymentAtomic(ctx, creditcard.AtomicAddPaymentInput{
		CardID: "c1", Entry: entry, Payment: pay,
		RealizeIDs: []string{"compra-1", "compra-2"}, RealizeAt: "2026-07-15",
	})
	if err != nil {
		t.Fatalf("AddPaymentAtomic: %v", err)
	}
	// 1. compras realizadas com a data informada (quitou a fatura).
	for _, id := range []string{"compra-1", "compra-2"} {
		st, pd := txnStatus(t, db, id)
		if st != "realizado" || pd.String != "2026-07-15" {
			t.Errorf("compra %s: status=%s payment_date=%s", id, st, pd.String)
		}
	}
	// 2. lançamento de pagamento criado (transferencia, realizado, sem cartão).
	var typ, status string
	var cardID sql.NullString
	if err := db.QueryRow("SELECT type, status, credit_card_id FROM transactions WHERE id = 'pay-1'").
		Scan(&typ, &status, &cardID); err != nil {
		t.Fatalf("query payment txn: %v", err)
	}
	if typ != "transferencia" || status != "realizado" || cardID.Valid {
		t.Errorf("payment txn inesperado: type=%s status=%s card=%v", typ, status, cardID)
	}
	// 3. entrada no ledger.
	m, _ := payRepo.ListByCard(ctx, "c1")
	if len(m["2026-07"]) != 1 || m["2026-07"][0].Amount != 30000 {
		t.Errorf("ledger inesperado: %+v", m["2026-07"])
	}
}

func TestInvoicePaymentRepo_AddPaymentAtomic_Partial(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Nubank", false))
	insertCompra(t, db, "compra-1", "pendente", 20000)

	pay := mkPaymentTxn("pay-1")
	pay.Amount = 8000
	entry := creditcard.InvoicePayment{ID: "e1", Reference: "2026-07", Amount: 8000, PaymentDate: "2026-07-15", TransactionID: &pay.ID, CreatedAt: time.Now().UTC()}
	// parcial → sem RealizeIDs (não quita).
	if err := payRepo.AddPaymentAtomic(ctx, creditcard.AtomicAddPaymentInput{CardID: "c1", Entry: entry, Payment: pay}); err != nil {
		t.Fatalf("AddPaymentAtomic: %v", err)
	}
	if st, _ := txnStatus(t, db, "compra-1"); st != "pendente" {
		t.Errorf("compra deveria seguir pendente no parcial, got %s", st)
	}
}

// _Rollback força falha na inserção do lançamento de pagamento (id duplicado) e prova
// que TUDO é revertido (RF-PAGFAT-04).
func TestInvoicePaymentRepo_AddPaymentAtomic_Rollback(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Nubank", false))
	insertCompra(t, db, "compra-1", "pendente", 20000)
	insertCompra(t, db, "dup", "realizado", 5000) // colide com o id do lançamento de pagamento

	pay := mkPaymentTxn("dup")
	entry := creditcard.InvoicePayment{ID: "e1", Reference: "2026-07", Amount: 30000, PaymentDate: "2026-07-15", TransactionID: &pay.ID, CreatedAt: time.Now().UTC()}
	err := payRepo.AddPaymentAtomic(ctx, creditcard.AtomicAddPaymentInput{
		CardID: "c1", Entry: entry, Payment: pay, RealizeIDs: []string{"compra-1"}, RealizeAt: "2026-07-15",
	})
	if err == nil {
		t.Fatal("esperava erro por id duplicado")
	}
	// rollback total: compra-1 segue pendente; sem entrada no ledger.
	st, pd := txnStatus(t, db, "compra-1")
	if st != "pendente" || pd.Valid {
		t.Errorf("rollback falhou: compra-1 status=%s payment_date=%v", st, pd)
	}
	if m, _ := payRepo.ListByCard(ctx, "c1"); len(m["2026-07"]) != 0 {
		t.Errorf("ledger não deveria ter entrada após rollback: %+v", m["2026-07"])
	}
}

func TestInvoicePaymentRepo_RemovePaymentAtomic(t *testing.T) {
	db := newTestDB(t)
	cardRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	cardRepo.Create(ctx, mkCard("c1", "Nubank", false))
	insertCompra(t, db, "compra-1", "pendente", 20000)
	pay := mkPaymentTxn("pay-1")
	pay.Amount = 20000
	entry := creditcard.InvoicePayment{ID: "e1", Reference: "2026-07", Amount: 20000, PaymentDate: "2026-07-15", TransactionID: &pay.ID, CreatedAt: time.Now().UTC()}
	if err := payRepo.AddPaymentAtomic(ctx, creditcard.AtomicAddPaymentInput{
		CardID: "c1", Entry: entry, Payment: pay, RealizeIDs: []string{"compra-1"}, RealizeAt: "2026-07-15",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// desfazer: reabre compra-1 e exclui o lançamento de pagamento.
	if err := payRepo.RemovePaymentAtomic(ctx, creditcard.AtomicRemovePaymentInput{
		PaymentID: "e1", PaymentTxnID: "pay-1", RevertIDs: []string{"compra-1"},
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	st, pd := txnStatus(t, db, "compra-1")
	if st != "pendente" || pd.Valid {
		t.Errorf("undo: compra-1 status=%s payment_date=%v", st, pd)
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM transactions WHERE id = 'pay-1'").Scan(&n)
	if n != 0 {
		t.Errorf("lançamento de pagamento deveria ter sido excluído")
	}
	if m, _ := payRepo.ListByCard(ctx, "c1"); len(m["2026-07"]) != 0 {
		t.Errorf("entrada do ledger deveria ter sido removida")
	}
}

func TestInvoicePaymentRepo_RemovePaymentAtomic_NotFound(t *testing.T) {
	db := newTestDB(t)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	if err := payRepo.RemovePaymentAtomic(context.Background(),
		creditcard.AtomicRemovePaymentInput{PaymentID: "nope"}); err != creditcard.ErrPaymentNotFound {
		t.Errorf("esperava ErrPaymentNotFound, got %v", err)
	}
}
