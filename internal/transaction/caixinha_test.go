package transaction_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

// ─── Helpers de caixinha ──────────────────────────────────────────────────────

func insertCaixinhaSeed(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT OR IGNORE INTO categories (id,name,type,can_be_deleted,created_at,updated_at)
		VALUES ('cat-caixinha','Movimentação de Caixinha','transferencia',0,?,?)`, now, now); err != nil {
		t.Fatalf("seed cat-caixinha: %v", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO subcategories
		(id,category_id,name,can_be_deleted,is_balance_adjustment,caixinha_direction,created_at,updated_at)
		VALUES ('sub-caixinha-aporte','cat-caixinha','Aporte em caixinha',0,0,'aporte',?,?),
		       ('sub-caixinha-resgate','cat-caixinha','Resgate de caixinha',0,0,'resgate',?,?),
		       ('sub-caixinha-saldo-inicial','cat-caixinha','Saldo inicial da caixinha',0,1,'aporte',?,?),
		       ('sub-caixinha-rendimento','cat-caixinha','Rendimento da caixinha',0,1,'aporte',?,?)`,
		now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed subs caixinha: %v", err)
	}
}

// insertOpening insere um SALDO INICIAL (abertura): conta no guardado, neutro ao disponível.
func insertOpening(t *testing.T, db *sql.DB, id, caixinhaID string, amount int64, date string) {
	t.Helper()
	insertNeutral(t, db, id, caixinhaID, "sub-caixinha-saldo-inicial", "Saldo inicial", amount, date)
}

// insertRendimento insere um RENDIMENTO: conta no guardado, neutro ao disponível.
func insertRendimento(t *testing.T, db *sql.DB, id, caixinhaID string, amount int64, date string) {
	t.Helper()
	insertNeutral(t, db, id, caixinhaID, "sub-caixinha-rendimento", "Rendimento", amount, date)
}

func insertNeutral(t *testing.T, db *sql.DB, id, caixinhaID, sub, title string, amount int64, date string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO transactions
		(id,title,amount,type,subcategory_id,payment_method,status,competence_date,payment_date,caixinha_id,created_at,updated_at)
		VALUES (?,?,?,'transferencia',?,'outros','realizado',?,?,?,?,?)`,
		id, title, amount, sub, date, date, caixinhaID, now, now)
	if err != nil {
		t.Fatalf("insert neutral %s: %v", id, err)
	}
}

func insertCaixinhaRow(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO caixinhas (id,name,type,display_order,archived,created_at,updated_at)
		VALUES (?,?,'reserva',0,0,?,?)`, id, name, now, now); err != nil {
		t.Fatalf("insert caixinha %s: %v", id, err)
	}
}

func insertMovement(t *testing.T, db *sql.DB, id, caixinhaID, direction string, amount int64, date string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	sub := "sub-caixinha-aporte"
	if direction == "resgate" {
		sub = "sub-caixinha-resgate"
	}
	_, err := db.Exec(`INSERT INTO transactions
		(id,title,amount,type,subcategory_id,payment_method,status,competence_date,payment_date,caixinha_id,created_at,updated_at)
		VALUES (?,?,?,'transferencia',?,'outros','realizado',?,?,?,?,?)`,
		id, direction, amount, sub, date, date, caixinhaID, now, now)
	if err != nil {
		t.Fatalf("insert movement %s: %v", id, err)
	}
}

// ─── Summary: aporte reduz / resgate aumenta o disponível ─────────────────────

func TestGetSummary_CaixinhaAfetaDisponivel(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	// receita 1000 realizada em 05/07
	rec := mkTransaction("t-rec", "Salário", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-05", strPtr("2026-07-05"))
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create receita: %v", err)
	}
	insertMovement(t, db, "m-ap", "cx1", "aporte", 30000, "2026-07-10")
	insertMovement(t, db, "m-rg", "cx1", "resgate", 10000, "2026-07-20")

	from, to := "2026-07-01", "2026-07-31"
	s, err := repo.GetSummary(ctx, transaction.TransactionFilter{CompetenceDateFrom: &from, CompetenceDateTo: &to})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.TotalReceitas != 100000 || s.TotalDespesas != 0 || s.SaldoPeriodo != 100000 {
		t.Fatalf("fluxo não deveria mudar: %+v", s)
	}
	if s.MovimentacaoCaixinhas != -20000 {
		t.Fatalf("movimentacao esperada -20000, veio %d", s.MovimentacaoCaixinhas)
	}
	if s.SaldoFinal != 80000 {
		t.Fatalf("saldoFinal esperado 80000, veio %d", s.SaldoFinal)
	}
}

// ─── Saldo inicial: conta no guardado, NEUTRO ao disponível ───────────────────

func TestSaldoInicial_NeutroNoDisponivel(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	reader := transaction.NewCaixinhaReader(db)
	ctx := context.Background()

	// receita 1000 realizada
	rec := mkTransaction("t-rec", "Salário", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-05", strPtr("2026-07-05"))
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create receita: %v", err)
	}
	// saldo inicial de 500 (dinheiro que já tinha) — NÃO deve mexer no disponível
	insertOpening(t, db, "op1", "cx1", 50000, "2026-07-01")

	from, to := "2026-07-01", "2026-07-31"
	s, err := repo.GetSummary(ctx, transaction.TransactionFilter{CompetenceDateFrom: &from, CompetenceDateTo: &to})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// disponível = só a receita (a abertura não conta)
	if s.MovimentacaoCaixinhas != 0 {
		t.Fatalf("abertura não deveria mexer no disponível, mov=%d", s.MovimentacaoCaixinhas)
	}
	if s.SaldoFinal != 100000 {
		t.Fatalf("saldoFinal esperado 100000 (abertura neutra), veio %d", s.SaldoFinal)
	}
	// mas o guardado da caixinha INCLUI a abertura
	bal, err := reader.BalanceByCaixinha(ctx, "cx1")
	if err != nil || bal != 50000 {
		t.Fatalf("guardado esperado 50000 (inclui abertura), veio %d err=%v", bal, err)
	}
	// e OpeningMovementIDs encontra a abertura (p/ replace)
	ids, err := reader.OpeningMovementIDs(ctx, "cx1")
	if err != nil || len(ids) != 1 || ids[0] != "op1" {
		t.Fatalf("opening ids inesperado: %v err=%v", ids, err)
	}
}

func TestRendimento_NeutroNoDisponivelContaNoGuardado(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	reader := transaction.NewCaixinhaReader(db)
	ctx := context.Background()

	rec := mkTransaction("t-rec", "Salário", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-05", strPtr("2026-07-05"))
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create receita: %v", err)
	}
	insertOpening(t, db, "op1", "cx1", 50000, "2026-07-01")   // já tinha 500
	insertRendimento(t, db, "r1", "cx1", 300, "2026-07-31")   // rendeu 3

	from, to := "2026-07-01", "2026-07-31"
	s, err := repo.GetSummary(ctx, transaction.TransactionFilter{CompetenceDateFrom: &from, CompetenceDateTo: &to})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// rendimento não mexe no disponível
	if s.MovimentacaoCaixinhas != 0 || s.SaldoFinal != 100000 {
		t.Fatalf("rendimento deveria ser neutro: mov=%d final=%d", s.MovimentacaoCaixinhas, s.SaldoFinal)
	}
	// guardado inclui saldo inicial + rendimento
	bal, err := reader.BalanceByCaixinha(ctx, "cx1")
	if err != nil || bal != 50300 {
		t.Fatalf("guardado esperado 50300, veio %d err=%v", bal, err)
	}
	// definir saldo inicial NÃO deve enxergar o rendimento como opening
	ids, err := reader.OpeningMovementIDs(ctx, "cx1")
	if err != nil || len(ids) != 1 || ids[0] != "op1" {
		t.Fatalf("opening ids deveria ser só o saldo inicial, veio %v", ids)
	}
}

func TestSaldoInicial_CarryoverNeutro(t *testing.T) {
	db := newTestDB(t)
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	// abertura ANTES do período — não deve entrar no SaldoInicial/carryover do disponível
	insertOpening(t, db, "op1", "cx1", 50000, "2026-06-10")
	from, to := "2026-07-01", "2026-07-31"
	s, err := repo.GetSummary(ctx, transaction.TransactionFilter{CompetenceDateFrom: &from, CompetenceDateTo: &to})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.SaldoInicial != 0 || s.SaldoFinal != 0 {
		t.Fatalf("abertura não deveria entrar no disponível: inicial=%d final=%d", s.SaldoInicial, s.SaldoFinal)
	}
}

// ─── List esconde movimentos de caixinha por padrão ───────────────────────────

func TestList_HideCaixinhaByDefault(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	rec := mkTransaction("t-rec", "Salário", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-05", strPtr("2026-07-05"))
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create: %v", err)
	}
	insertMovement(t, db, "m-ap", "cx1", "aporte", 30000, "2026-07-10")

	// default: esconde caixinha
	list, err := repo.List(ctx, transaction.TransactionFilter{}, shared.DefaultPagination())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "t-rec" {
		t.Fatalf("esperava só a receita, veio %d itens", len(list))
	}

	// filtro por caixinha: mostra o extrato
	cx := "cx1"
	movs, err := repo.List(ctx, transaction.TransactionFilter{CaixinhaID: &cx}, shared.DefaultPagination())
	if err != nil {
		t.Fatalf("list caixinha: %v", err)
	}
	if len(movs) != 1 || movs[0].ID != "m-ap" {
		t.Fatalf("esperava só o movimento, veio %d itens", len(movs))
	}
}

// ─── Writer cria movimento neutro ─────────────────────────────────────────────

func TestCaixinhaWriter_RegisterAndDelete(t *testing.T) {
	db := newTestDB(t)
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	create := transaction.NewCreateTransaction(repo, fakeFacade{typ: "transferencia"}, fakeChecker{}, nil)
	del := transaction.NewDeleteTransaction(repo, nil)
	w := transaction.NewCaixinhaWriter(create, del)

	id, err := w.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: "cx1", Direction: "aporte", Amount: 50000, Date: "2026-07-10",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != transaction.TypeTransferencia || got.SubcategoryID != "sub-caixinha-aporte" {
		t.Fatalf("movimento inesperado: type=%s sub=%s", got.Type, got.SubcategoryID)
	}
	if got.CaixinhaID == nil || *got.CaixinhaID != "cx1" {
		t.Fatalf("caixinha_id não setado: %v", got.CaixinhaID)
	}
	if got.Status != transaction.StatusRealizado || got.PaymentDate == nil || *got.PaymentDate != "2026-07-10" {
		t.Fatalf("status/payment_date inesperados: %s %v", got.Status, got.PaymentDate)
	}

	if err := w.Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, id); err != transaction.ErrTransactionNotFound {
		t.Fatalf("esperava not found após delete, veio %v", err)
	}
}

// ─── Writer: saldo inicial e rendimento usam as subcategorias certas ──────────

func TestCaixinhaWriter_OpeningERendimento(t *testing.T) {
	db := newTestDB(t)
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()
	create := transaction.NewCreateTransaction(repo, fakeFacade{typ: "transferencia"}, fakeChecker{}, nil)
	w := transaction.NewCaixinhaWriter(create, transaction.NewDeleteTransaction(repo, nil))

	idOp, err := w.Register(ctx, shared.NewCaixinhaMovement{CaixinhaID: "cx1", Direction: "aporte", Amount: 50000, Date: "2026-07-01", Opening: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	op, _ := repo.Get(ctx, idOp)
	if op.SubcategoryID != "sub-caixinha-saldo-inicial" {
		t.Fatalf("opening deveria usar sub-caixinha-saldo-inicial, veio %s", op.SubcategoryID)
	}

	idR, err := w.Register(ctx, shared.NewCaixinhaMovement{CaixinhaID: "cx1", Direction: "aporte", Amount: 300, Date: "2026-07-31", Rendimento: true})
	if err != nil {
		t.Fatalf("rendimento: %v", err)
	}
	r, _ := repo.Get(ctx, idR)
	if r.SubcategoryID != "sub-caixinha-rendimento" {
		t.Fatalf("rendimento deveria usar sub-caixinha-rendimento, veio %s", r.SubcategoryID)
	}
}

// ─── Reader: saldos, extrato e disponível ─────────────────────────────────────

func TestCaixinhaReader(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertCaixinhaSeed(t, db)
	insertCaixinhaRow(t, db, "cx1", "Reserva")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	// receita para o disponível
	rec := mkTransaction("t-rec", "Salário", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-05", strPtr("2026-07-05"))
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create receita: %v", err)
	}
	insertMovement(t, db, "m1", "cx1", "aporte", 50000, "2026-07-10")
	insertMovement(t, db, "m2", "cx1", "aporte", 30000, "2026-07-12")
	insertMovement(t, db, "m3", "cx1", "resgate", 20000, "2026-07-20")

	reader := transaction.NewCaixinhaReader(db)

	bal, err := reader.BalanceByCaixinha(ctx, "cx1")
	if err != nil || bal != 60000 {
		t.Fatalf("balance esperado 60000, veio %d err=%v", bal, err)
	}

	all, err := reader.BalancesAll(ctx)
	if err != nil || all["cx1"] != 60000 {
		t.Fatalf("balancesAll esperado 60000, veio %v err=%v", all, err)
	}

	movs, total, err := reader.ListByCaixinha(ctx, "cx1", shared.DefaultPagination())
	if err != nil || total != 3 || len(movs) != 3 {
		t.Fatalf("extrato esperado 3, veio total=%d len=%d err=%v", total, len(movs), err)
	}
	// mais recente primeiro (m3 resgate 20/07)
	if movs[0].TransactionID != "m3" || movs[0].Direction != "resgate" || movs[0].Amount != 20000 {
		t.Fatalf("ordem/DTO do extrato inesperados: %+v", movs[0])
	}

	// disponível = receita 100000 + movimentacao (20000 - 80000) = 40000
	disp, err := reader.DisponivelAtual(ctx)
	if err != nil || disp != 40000 {
		t.Fatalf("disponível esperado 40000, veio %d err=%v", disp, err)
	}
}
