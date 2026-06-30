package transaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

func TestReportAggregator_Month(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-exp", "Despesas", "despesa", "sub-exp", "Aluguel")
	insertTestSub(t, db, "cat-inc", "Receitas", "receita", "sub-inc", "Salário")
	insertAdjustmentSub(t, db, "cat-trf", "sub-adj", "Saldo Inicial")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	mk := func(id, sub string, amt int64, typ transaction.TransactionType, pm transaction.PaymentMethod, st transaction.TransactionStatus, comp string) {
		if err := repo.Create(ctx, mkTransaction(id, id, sub, amt, typ, pm, st, comp, strPtr(comp))); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	mk("p1", "sub-inc", 100000, transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2025-12-10") // carryover
	mk("adj", "sub-adj", 50000, transaction.TypeTransferencia, transaction.MethodOutros, transaction.StatusRealizado, "2026-01-05")
	mk("d1", "sub-exp", 30000, transaction.TypeDespesa, transaction.MethodPix, transaction.StatusRealizado, "2026-01-15")
	mk("r1", "sub-inc", 200000, transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-01-20")
	// pendente
	if err := repo.Create(ctx, mkTransaction("d2", "d2", "sub-exp", 40000, transaction.TypeDespesa, transaction.MethodBoleto, transaction.StatusPendente, "2026-01-25", nil)); err != nil {
		t.Fatalf("seed d2: %v", err)
	}

	agg := transaction.NewReportAggregator(db)

	_, totals, err := agg.AggregateMonth(ctx, "2026-01")
	if err != nil {
		t.Fatalf("AggregateMonth: %v", err)
	}
	if totals.Receitas != 200000 || totals.Despesas != 30000 || totals.SaldoPeriodo != 170000 {
		t.Errorf("totais realizados: %+v", totals)
	}
	// saldoInicial = carryover (100000) + ajuste (50000) = 150000; saldoFinal = 320000
	if totals.SaldoInicial != 150000 || totals.SaldoFinal != 320000 {
		t.Errorf("saldo acumulado: inicial=%d final=%d", totals.SaldoInicial, totals.SaldoFinal)
	}

	_, ptot, err := agg.AggregatePendingMonth(ctx, "2026-01")
	if err != nil || ptot.Despesas != 40000 {
		t.Errorf("pendentes: %v %+v", err, ptot)
	}

	pb, err := agg.PaymentBreakdownMonth(ctx, "2026-01")
	if err != nil || pb["pix"] != 30000 {
		t.Errorf("breakdown por forma de pagamento: %v %+v", err, pb)
	}

	// referências inválidas → erro
	if _, _, err := agg.AggregateMonth(ctx, "bad"); err == nil {
		t.Error("AggregateMonth ref inválida deveria falhar")
	}
	if _, _, err := agg.AggregatePendingMonth(ctx, "bad"); err == nil {
		t.Error("AggregatePendingMonth ref inválida deveria falhar")
	}
	if _, err := agg.PaymentBreakdownMonth(ctx, "bad"); err == nil {
		t.Error("PaymentBreakdownMonth ref inválida deveria falhar")
	}
}

func TestIncomeReader_MonthIncome(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-inc", "Receitas", "receita", "sub-inc", "Salário")
	insertAdjustmentSub(t, db, "cat-trf", "sub-adj", "Saldo Inicial")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	// realizada + pendente + ajuste de saldo (este excluído)
	if err := repo.Create(ctx, mkTransaction("r1", "Salário", "sub-inc", 200000, transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-01-05", strPtr("2026-01-05"))); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, mkTransaction("r2", "Extra", "sub-inc", 100000, transaction.TypeReceita, transaction.MethodPix, transaction.StatusPendente, "2026-01-20", nil)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, mkTransaction("adj", "Ajuste", "sub-adj", 999999, transaction.TypeTransferencia, transaction.MethodOutros, transaction.StatusRealizado, "2026-01-10", strPtr("2026-01-10"))); err != nil {
		t.Fatal(err)
	}

	reader := transaction.NewIncomeReader(db)
	total, allRealized, items, err := reader.MonthIncome(ctx, "2026-01")
	if err != nil {
		t.Fatalf("MonthIncome: %v", err)
	}
	if total != 300000 || allRealized || len(items) != 2 {
		t.Errorf("renda: total=%d allRealized=%v itens=%d (ajuste deve ser excluído)", total, allRealized, len(items))
	}

	// confirma a pendente → allRealized true
	if _, err := db.Exec("UPDATE transactions SET status='realizado' WHERE id='r2'"); err != nil {
		t.Fatal(err)
	}
	total, allRealized, _, _ = reader.MonthIncome(ctx, "2026-01")
	if total != 300000 || !allRealized {
		t.Errorf("após confirmar: total=%d allRealized=%v", total, allRealized)
	}

	if _, _, _, err := reader.MonthIncome(ctx, "bad"); err == nil {
		t.Error("ref inválida deveria falhar")
	}
}

// ── BudgetWriter com use cases fake ──────────────────────────────────────────

type fakeCreateUC struct {
	in  transaction.CreateTransactionInput
	err error
}

func (f *fakeCreateUC) Execute(_ context.Context, in transaction.CreateTransactionInput) (transaction.TransactionDetail, error) {
	f.in = in
	if f.err != nil {
		return transaction.TransactionDetail{}, f.err
	}
	return transaction.TransactionDetail{Transaction: transaction.Transaction{ID: "tx-1"}}, nil
}

type fakeDeleteUC struct {
	id  string
	err error
}

func (f *fakeDeleteUC) Execute(_ context.Context, id string) error {
	f.id = id
	return f.err
}

func TestBudgetWriter(t *testing.T) {
	ctx := context.Background()

	fc := &fakeCreateUC{}
	fd := &fakeDeleteUC{}
	w := transaction.NewBudgetWriter(fc, fd)

	// sem forma de pagamento → default "outros"
	id, err := w.Create(ctx, shared.NewTransaction{Title: "Aluguel", Amount: 125000, SubcategoryID: "sub-x", Status: "realizado", CompetenceDate: "2026-06-30"})
	if err != nil || id != "tx-1" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	if fc.in.PaymentMethod != transaction.MethodOutros {
		t.Errorf("default payment method=%q want outros", fc.in.PaymentMethod)
	}

	// com forma de pagamento explícita
	_, _ = w.Create(ctx, shared.NewTransaction{Title: "Pix", Amount: 1000, SubcategoryID: "s", Status: "realizado", CompetenceDate: "2026-06-30", PaymentMethod: "pix"})
	if fc.in.PaymentMethod != transaction.MethodPix {
		t.Errorf("payment method=%q want pix", fc.in.PaymentMethod)
	}

	// erro na criação propaga
	fc.err = errors.New("boom")
	if _, err := w.Create(ctx, shared.NewTransaction{Title: "X", Amount: 1, SubcategoryID: "s", Status: "realizado", CompetenceDate: "2026-06-30"}); err == nil {
		t.Error("erro de create deveria propagar")
	}

	// delete delega ao use case
	if err := w.Delete(ctx, "tx-9"); err != nil || fd.id != "tx-9" {
		t.Errorf("delete: err=%v id=%q", err, fd.id)
	}
}
