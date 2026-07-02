package transaction_test

import (
	"context"
	"testing"

	"github.com/local-finance-manager/backend/internal/transaction"
)

func TestCashAggregator(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	insertTestSub(t, db, "cat-d", "Casa", "despesa", "sub-d", "Mercado")
	repo := transaction.NewSQLiteRepository(db)
	ca := transaction.NewCashAggregator(db)
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	// carryover: receita paga em junho
	must(repo.Create(ctx, mkTransaction("t0", "Salário jun", "sub-r", 50000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-06-01", strPtr("2026-06-05"))))
	// julho (por data de pagamento): receita + 2 despesas realizadas
	must(repo.Create(ctx, mkTransaction("t1", "Salário jul", "sub-r", 100000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado, "2026-07-01", strPtr("2026-07-05"))))
	// despesa com competência em JUNHO mas paga em JULHO → cai no caixa de julho
	must(repo.Create(ctx, mkTransaction("t2", "Mercado", "sub-d", 40000,
		transaction.TypeDespesa, transaction.MethodPix, transaction.StatusRealizado, "2026-06-20", strPtr("2026-07-10"))))
	must(repo.Create(ctx, mkTransaction("t3", "Mercado cartão", "sub-d", 25000,
		transaction.TypeDespesa, transaction.MethodCartaoCredito, transaction.StatusRealizado, "2026-07-02", strPtr("2026-07-15"))))
	// pendente (projetivo): por competência
	must(repo.Create(ctx, mkTransaction("t4", "Conta a pagar", "sub-d", 30000,
		transaction.TypeDespesa, transaction.MethodBoleto, transaction.StatusPendente, "2026-07-20", nil)))

	from, to := "2026-07-01", "2026-07-31"

	aggs, totals, err := ca.AggregateCashPeriod(ctx, from, to)
	if err != nil {
		t.Fatalf("cash period: %v", err)
	}
	if totals.Receitas != 100000 || totals.Despesas != 65000 || totals.SaldoPeriodo != 35000 {
		t.Fatalf("totais de caixa inesperados: %+v", totals)
	}
	if totals.SaldoInicial != 50000 || totals.SaldoFinal != 85000 {
		t.Fatalf("saldo acumulado de caixa inesperado: inicial=%d final=%d", totals.SaldoInicial, totals.SaldoFinal)
	}
	if len(aggs) != 2 { // receita(sub-r) + despesa(sub-d agrupada)
		t.Fatalf("esperava 2 agregados, veio %d: %+v", len(aggs), aggs)
	}

	_, pend, err := ca.AggregateCashPending(ctx, from, to)
	if err != nil {
		t.Fatalf("cash pending: %v", err)
	}
	if pend.Despesas != 30000 {
		t.Fatalf("pendente por competência esperado 30000, veio %d", pend.Despesas)
	}

	pay, err := ca.PaymentBreakdownCash(ctx, from, to)
	if err != nil {
		t.Fatalf("payment breakdown: %v", err)
	}
	if pay["pix"] != 40000 || pay["cartao_credito"] != 25000 {
		t.Fatalf("breakdown por forma de pagamento inesperado: %+v", pay)
	}
}

func TestCashAggregator_Errors(t *testing.T) {
	ca := transaction.NewCashAggregator(closedDB(t))
	ctx := context.Background()
	if _, _, err := ca.AggregateCashPeriod(ctx, "2026-07-01", "2026-07-31"); err == nil {
		t.Fatal("esperava erro com DB fechado")
	}
	if _, _, err := ca.AggregateCashPending(ctx, "2026-07-01", "2026-07-31"); err == nil {
		t.Fatal("esperava erro com DB fechado")
	}
	if _, err := ca.PaymentBreakdownCash(ctx, "2026-07-01", "2026-07-31"); err == nil {
		t.Fatal("esperava erro com DB fechado")
	}
}
