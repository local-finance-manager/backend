package installment_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/installment"
	"github.com/local-finance-manager/backend/internal/shared"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("fk on: %v", err)
	}
	stmts := []string{
		`CREATE TABLE categories (id TEXT PRIMARY KEY, name TEXT, type TEXT, can_be_deleted INTEGER, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE subcategories (id TEXT PRIMARY KEY, category_id TEXT, name TEXT, can_be_deleted INTEGER, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE credit_cards (id TEXT PRIMARY KEY, name TEXT, brand TEXT, credit_limit INTEGER, closing_day INTEGER, due_day INTEGER, archived INTEGER, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE installment_groups (
			id TEXT PRIMARY KEY,
			credit_card_id TEXT NOT NULL REFERENCES credit_cards(id) ON DELETE RESTRICT,
			subcategory_id TEXT NOT NULL REFERENCES subcategories(id) ON DELETE RESTRICT,
			title TEXT NOT NULL, description TEXT,
			total_amount INTEGER NOT NULL CHECK(total_amount > 0),
			principal_amount INTEGER,
			installments_count INTEGER NOT NULL CHECK(installments_count BETWEEN 2 AND 72),
			purchase_date TEXT NOT NULL, first_reference TEXT NOT NULL,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE transactions (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT,
			amount INTEGER NOT NULL CHECK(amount > 0), type TEXT NOT NULL,
			subcategory_id TEXT NOT NULL, payment_method TEXT NOT NULL,
			status TEXT NOT NULL, competence_date TEXT NOT NULL, payment_date TEXT,
			account_id TEXT, destination_account_id TEXT, credit_card_id TEXT,
			installment_group_id TEXT REFERENCES installment_groups(id) ON DELETE CASCADE,
			installment_number INTEGER, installment_total INTEGER,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	// seed cartão + subcategoria de despesa
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(`INSERT INTO categories (id,name,type,can_be_deleted,created_at,updated_at) VALUES ('cat-1','Eletrônicos','despesa',1,?,?)`, now, now)
	db.Exec(`INSERT INTO subcategories (id,category_id,name,can_be_deleted,created_at,updated_at) VALUES ('sub-1','cat-1','Notebook',1,?,?)`, now, now)
	db.Exec(`INSERT INTO credit_cards (id,name,brand,credit_limit,closing_day,due_day,archived,created_at,updated_at) VALUES ('card-1','Nubank','visa',5000000,3,10,0,?,?)`, now, now)
	return db
}

func mkGroup(id string) installment.InstallmentGroup {
	now := time.Now().UTC()
	return installment.InstallmentGroup{
		ID: id, CreditCardID: "card-1", SubcategoryID: "sub-1", Title: "Notebook Dell",
		TotalAmount: 100000, InstallmentsCount: 3, PurchaseDate: "2026-06-22",
		FirstReference: "2026-07", CreatedAt: now, UpdatedAt: now,
	}
}

func mkParcelas() []installment.Parcela {
	return []installment.Parcela{
		{ID: "p1", Number: 1, Amount: 33334, CompetenceDate: "2026-06-22"},
		{ID: "p2", Number: 2, Amount: 33333, CompetenceDate: "2026-07-22"},
		{ID: "p3", Number: 3, Amount: 33333, CompetenceDate: "2026-08-22"},
	}
}

func TestRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()

	if err := repo.Create(ctx, mkGroup("g1"), mkParcelas()); err != nil {
		t.Fatalf("create: %v", err)
	}
	g, parcelas, err := repo.Get(ctx, "g1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if g.Title != "Notebook Dell" || g.InstallmentsCount != 3 {
		t.Errorf("grupo inesperado: %+v", g)
	}
	if len(parcelas) != 3 {
		t.Fatalf("esperava 3 parcelas, got %d", len(parcelas))
	}
	if parcelas[0].Number != 1 || parcelas[0].Amount != 33334 || parcelas[0].Status != "pendente" {
		t.Errorf("parcela 1 inesperada: %+v", parcelas[0])
	}
	// soma das parcelas == total
	var total int64
	for _, p := range parcelas {
		total += p.Amount
	}
	if total != g.TotalAmount {
		t.Errorf("soma parcelas %d != total %d", total, g.TotalAmount)
	}
}

func TestRepo_Create_AtomicRollback(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()

	// 2ª parcela com amount 0 viola CHECK(amount>0) → rollback total
	bad := []installment.Parcela{
		{ID: "p1", Number: 1, Amount: 50000, CompetenceDate: "2026-06-22"},
		{ID: "p2", Number: 2, Amount: 0, CompetenceDate: "2026-07-22"},
	}
	if err := repo.Create(ctx, mkGroup("g1"), bad); err == nil {
		t.Fatal("esperava erro na parcela inválida")
	}
	// grupo NÃO deve existir (rollback)
	if _, _, err := repo.Get(ctx, "g1"); err != installment.ErrInstallmentGroupNotFound {
		t.Errorf("grupo deveria ter sido revertido, got %v", err)
	}
	// nenhuma parcela órfã
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE installment_group_id='g1'`).Scan(&n)
	if n != 0 {
		t.Errorf("parcelas órfãs após rollback: %d", n)
	}
}

func TestRepo_Delete_Cascade(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkGroup("g1"), mkParcelas())

	if err := repo.Delete(ctx, "g1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE installment_group_id='g1'`).Scan(&n)
	if n != 0 {
		t.Errorf("CASCADE falhou: %d parcelas restaram", n)
	}
}

func TestRepo_DeleteNotFound(t *testing.T) {
	repo := installment.NewSQLiteRepository(newTestDB(t))
	if err := repo.Delete(context.Background(), "ghost"); err != installment.ErrInstallmentGroupNotFound {
		t.Errorf("got %v, want ErrInstallmentGroupNotFound", err)
	}
}

func TestRepo_CancelRemaining(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkGroup("g1"), mkParcelas())
	// paga a 1ª
	db.Exec(`UPDATE transactions SET status='realizado' WHERE id='p1'`)

	cancelled, err := repo.CancelRemaining(ctx, "g1")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled != 2 {
		t.Errorf("esperava 2 canceladas, got %d", cancelled)
	}
	_, parcelas, _ := repo.Get(ctx, "g1")
	statusOf := map[string]string{}
	for _, p := range parcelas {
		statusOf[p.TransactionID] = p.Status
	}
	if statusOf["p1"] != "realizado" || statusOf["p2"] != "cancelado" || statusOf["p3"] != "cancelado" {
		t.Errorf("status inesperado: %v", statusOf)
	}
}

func TestRepo_UpdateSeries(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()
	repo.Create(ctx, mkGroup("g1"), mkParcelas())
	// outra subcategoria
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(`INSERT INTO subcategories (id,category_id,name,can_be_deleted,created_at,updated_at) VALUES ('sub-2','cat-1','Outro',1,?,?)`, now, now)

	if err := repo.UpdateSeries(ctx, "g1", "Notebook Novo", nil, "sub-2", "despesa"); err != nil {
		t.Fatalf("update: %v", err)
	}
	g, parcelas, _ := repo.Get(ctx, "g1")
	if g.Title != "Notebook Novo" || g.SubcategoryID != "sub-2" {
		t.Errorf("grupo não atualizado: %+v", g)
	}
	// parcelas refletem
	var title, subcat string
	db.QueryRow(`SELECT title, subcategory_id FROM transactions WHERE id='p1'`).Scan(&title, &subcat)
	if title != "Notebook Novo" || subcat != "sub-2" {
		t.Errorf("parcela não atualizada: title=%q subcat=%q", title, subcat)
	}
	_ = parcelas
}

func TestRepo_UpdateSeries_NotFound(t *testing.T) {
	repo := installment.NewSQLiteRepository(newTestDB(t))
	if err := repo.UpdateSeries(context.Background(), "ghost", "X", nil, "sub-1", "despesa"); err != installment.ErrInstallmentGroupNotFound {
		t.Errorf("got %v, want ErrInstallmentGroupNotFound", err)
	}
}

func TestRepo_List_AggregatesAndFilter(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()

	// g1: ativo (1 paga, 2 pendentes)
	repo.Create(ctx, mkGroup("g1"), mkParcelas())
	db.Exec(`UPDATE transactions SET status='realizado' WHERE id='p1'`)
	// g2: quitado (todas realizadas)
	g2 := mkGroup("g2")
	repo.Create(ctx, g2, []installment.Parcela{
		{ID: "q1", Number: 1, Amount: 50000, CompetenceDate: "2026-06-22"},
		{ID: "q2", Number: 2, Amount: 50000, CompetenceDate: "2026-07-22"},
	})
	db.Exec(`UPDATE transactions SET status='realizado' WHERE installment_group_id='g2'`)

	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "created_at", Order: "ASC"}

	all, total, err := repo.List(ctx, installment.Filter{}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Fatalf("got total=%d len=%d, want 2/2", total, len(all))
	}
	byID := map[string]installment.GroupSummary{}
	for _, s := range all {
		byID[s.Group.ID] = s
	}
	if byID["g1"].Status != installment.GroupActive || byID["g1"].PaidCount != 1 || byID["g1"].RemainingCount != 2 {
		t.Errorf("g1 agregados inesperados: %+v", byID["g1"])
	}
	if byID["g1"].RemainingAmount != 33333+33333 {
		t.Errorf("g1 remaining_amount: got %d", byID["g1"].RemainingAmount)
	}
	if byID["g2"].Status != installment.GroupSettled {
		t.Errorf("g2 deveria ser quitado, got %s", byID["g2"].Status)
	}

	// filtro por status ativo
	active := installment.GroupActive
	got, total, err := repo.List(ctx, installment.Filter{Status: &active}, p)
	if err != nil {
		t.Fatalf("list ativo: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].Group.ID != "g1" {
		t.Errorf("filtro ativo: got total=%d %v", total, got)
	}
}

func TestRepo_FKRestrict_CardWithInstallments(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	repo.Create(context.Background(), mkGroup("g1"), mkParcelas())
	// não pode excluir cartão com compra parcelada (FK RESTRICT)
	if _, err := db.Exec(`DELETE FROM credit_cards WHERE id='card-1'`); err == nil {
		t.Error("esperava FK RESTRICT ao excluir cartão com parcelamento")
	}
}
