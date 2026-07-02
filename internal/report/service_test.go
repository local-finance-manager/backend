package report

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── DB + fakes ──────────────────────────────────────────────────────────────

func newReportDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, _ = db.Exec("PRAGMA foreign_keys = ON")
	stmts := []string{
		`CREATE TABLE report_monthly_closing (
			reference TEXT PRIMARY KEY, closed_at TEXT NOT NULL, month_last_day TEXT NOT NULL,
			hard_lock_at TEXT NOT NULL, total_receitas INTEGER NOT NULL, total_despesas INTEGER NOT NULL,
			total_transferencias INTEGER NOT NULL, saldo_periodo INTEGER NOT NULL, saldo_inicial INTEGER NOT NULL,
			saldo_final INTEGER NOT NULL, tx_count INTEGER NOT NULL, recalculated_at TEXT,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE report_monthly_snapshot (
			reference TEXT NOT NULL REFERENCES report_monthly_closing(reference) ON DELETE CASCADE,
			subcategory_id TEXT NOT NULL, category_id TEXT NOT NULL, type TEXT NOT NULL,
			total INTEGER NOT NULL, tx_count INTEGER NOT NULL, PRIMARY KEY (reference, subcategory_id))`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

type fakeRealized struct {
	aggs   map[string][]shared.SubcategoryAggregate
	totals map[string]shared.MonthlyTotals
}

func (f *fakeRealized) AggregateMonth(_ context.Context, ref string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return f.aggs[ref], f.totals[ref], nil
}

type fakePending struct{ totals shared.MonthlyTotals }

func (f *fakePending) AggregatePendingMonth(_ context.Context, _ string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return nil, f.totals, nil
}

type fakeTree struct{}

func (fakeTree) Tree(_ context.Context) ([]shared.CategoryNode, error) {
	return []shared.CategoryNode{{
		CategoryID: "cat-1", CategoryName: "Alimentação", CategoryColor: "#27AE60", Type: "despesa",
		Subcategories: []shared.SubcategoryNode{{ID: "s1", Name: "Mercado"}},
	}}, nil
}

type fakePayments struct{ m map[string]int64 }

func (f fakePayments) PaymentBreakdownMonth(_ context.Context, _ string) (map[string]int64, error) {
	return f.m, nil
}

// fakeCash implementa CashAggregator devolvendo valores fixos (independe do intervalo).
type fakeCash struct {
	aggs    []shared.SubcategoryAggregate
	totals  shared.MonthlyTotals
	pending shared.MonthlyTotals
	pay     map[string]int64
}

func (f *fakeCash) AggregateCashPeriod(_ context.Context, _, _ string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return f.aggs, f.totals, nil
}
func (f *fakeCash) AggregateCashPending(_ context.Context, _, _ string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return nil, f.pending, nil
}
func (f *fakeCash) PaymentBreakdownCash(_ context.Context, _, _ string) (map[string]int64, error) {
	return f.pay, nil
}

func newSvc(t *testing.T, today string, fr *fakeRealized) *Service {
	svc := NewService(Deps{
		Repo:     NewSQLiteRepository(newReportDB(t)),
		Realized: fr,
		Pending:  &fakePending{},
		Tree:     fakeTree{},
		Payments: fakePayments{m: map[string]int64{"cartao_credito": 45000}},
		Cash:     &fakeCash{pay: map[string]int64{}},
	})
	svc.now = func() time.Time { tt, _ := time.Parse("2006-01-02", today); return tt }
	return svc
}

func despesa(total int64) (map[string][]shared.SubcategoryAggregate, map[string]shared.MonthlyTotals) {
	return map[string][]shared.SubcategoryAggregate{
			"2026-06": {{SubcategoryID: "s1", CategoryID: "cat-1", Type: "despesa", Total: total, TxCount: 2}},
		},
		map[string]shared.MonthlyTotals{
			"2026-06": {Receitas: 800000, Despesas: total, SaldoPeriodo: 800000 - total, SaldoInicial: 1000000, SaldoFinal: 1000000 + (800000 - total), TxCount: 2},
		}
}

// ─── Testes ──────────────────────────────────────────────────────────────────

// ─── Regime de CAIXA (R8) ────────────────────────────────────────────────────

func newSvcCash(t *testing.T, today string, fc *fakeCash) *Service {
	svc := NewService(Deps{
		Repo:     NewSQLiteRepository(newReportDB(t)),
		Realized: &fakeRealized{},
		Pending:  &fakePending{},
		Tree:     fakeTree{},
		Payments: fakePayments{m: map[string]int64{}},
		Cash:     fc,
	})
	svc.now = func() time.Time { tt, _ := time.Parse("2006-01-02", today); return tt }
	return svc
}

func TestService_Monthly_Caixa(t *testing.T) {
	fc := &fakeCash{
		aggs:    []shared.SubcategoryAggregate{{SubcategoryID: "s1", CategoryID: "cat-1", Type: "despesa", Total: 90000, TxCount: 3}},
		totals:  shared.MonthlyTotals{Receitas: 500000, Despesas: 90000, SaldoPeriodo: 410000, SaldoInicial: 100000, SaldoFinal: 510000, TxCount: 3},
		pending: shared.MonthlyTotals{Despesas: 20000, Receitas: 0},
		pay:     map[string]int64{"cartao_credito": 30000},
	}
	svc := newSvcCash(t, "2026-07-05", fc)
	ctx := context.Background()

	// regime default (vazio) = caixa
	rep, err := svc.Monthly(ctx, "2026-06", "realizado", "")
	if err != nil {
		t.Fatalf("monthly caixa: %v", err)
	}
	if rep.Regime != RegimeCaixa {
		t.Fatalf("regime esperado caixa, veio %q", rep.Regime)
	}
	if rep.KPIs.TotalDespesas != 90000 || rep.KPIs.TotalReceitas != 500000 {
		t.Fatalf("KPIs de caixa inesperados: %+v", rep.KPIs)
	}
	if rep.KPIs.PercentNoCredito != 33 { // 30000/90000
		t.Fatalf("%% no crédito esperado 33, veio %d", rep.KPIs.PercentNoCredito)
	}

	// projetivo caixa usa os pendentes por competência
	rp, err := svc.Monthly(ctx, "2026-06", "projetivo", RegimeCaixa)
	if err != nil {
		t.Fatalf("monthly caixa projetivo: %v", err)
	}
	if rp.Mode != "projetivo" || rp.Projetado == nil || rp.Projetado.TotalDespesas != 20000 {
		t.Fatalf("projetivo caixa inesperado: %+v", rp.Projetado)
	}
}

func TestService_LongPeriod_Caixa(t *testing.T) {
	fc := &fakeCash{
		aggs:   []shared.SubcategoryAggregate{{SubcategoryID: "s1", CategoryID: "cat-1", Type: "despesa", Total: 60000, TxCount: 2}},
		totals: shared.MonthlyTotals{Receitas: 300000, Despesas: 60000, SaldoPeriodo: 240000, SaldoFinal: 240000, TxCount: 2},
		pay:    map[string]int64{},
	}
	svc := newSvcCash(t, "2027-01-05", fc)
	ctx := context.Background()

	q, err := svc.Quarterly(ctx, 2026, 2, RegimeCaixa)
	if err != nil {
		t.Fatalf("quarterly caixa: %v", err)
	}
	if q.Regime != RegimeCaixa {
		t.Fatalf("regime esperado caixa, veio %q", q.Regime)
	}
	// caixa não depende de fechamento: todos os meses incluídos, nenhum faltante
	if len(q.IncludedMonths) != 3 || len(q.MissingMonths) != 0 {
		t.Fatalf("caixa deveria incluir todos os meses: incl=%v miss=%v", q.IncludedMonths, q.MissingMonths)
	}
	if len(q.Monthly) != 3 {
		t.Fatalf("esperava 3 pontos mês a mês, veio %d", len(q.Monthly))
	}
	// anual e semestral em caixa também
	if a, err := svc.Annual(ctx, 2026, RegimeCaixa); err != nil || a.Regime != RegimeCaixa {
		t.Fatalf("annual caixa: %v %q", err, a.Regime)
	}
	if s, err := svc.Semiannual(ctx, 2026, 2, RegimeCaixa); err != nil || len(s.IncludedMonths) != 6 {
		t.Fatalf("semiannual caixa: %v incl=%d", err, len(s.IncludedMonths))
	}
}

type fakeCashErr struct{}

func (fakeCashErr) AggregateCashPeriod(_ context.Context, _, _ string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return nil, shared.MonthlyTotals{}, context.Canceled
}
func (fakeCashErr) AggregateCashPending(_ context.Context, _, _ string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, error) {
	return nil, shared.MonthlyTotals{}, context.Canceled
}
func (fakeCashErr) PaymentBreakdownCash(_ context.Context, _, _ string) (map[string]int64, error) {
	return nil, context.Canceled
}

func TestService_Caixa_ErrorPropagates(t *testing.T) {
	svc := NewService(Deps{
		Repo: NewSQLiteRepository(newReportDB(t)), Realized: &fakeRealized{}, Pending: &fakePending{},
		Tree: fakeTree{}, Payments: fakePayments{m: map[string]int64{}}, Cash: fakeCashErr{},
	})
	svc.now = func() time.Time { tt, _ := time.Parse("2006-01-02", "2026-07-05"); return tt }
	ctx := context.Background()
	if _, err := svc.Monthly(ctx, "2026-06", "realizado", RegimeCaixa); err == nil {
		t.Fatal("Monthly caixa deveria propagar erro do agregador")
	}
	if _, err := svc.Annual(ctx, 2026, RegimeCaixa); err == nil {
		t.Fatal("Annual caixa deveria propagar erro do agregador")
	}
}

func TestService_Close_RequiresMonthEnded(t *testing.T) {
	a, t2 := despesa(120000)
	svc := newSvc(t, "2026-06-15", &fakeRealized{aggs: a, totals: t2}) // mês ainda corre
	if _, err := svc.Close(context.Background(), "2026-06"); err != ErrMonthNotEnded {
		t.Fatalf("esperava ErrMonthNotEnded, got %v", err)
	}
}

func TestService_Close_And_Monthly(t *testing.T) {
	a, t2 := despesa(120000)
	svc := newSvc(t, "2026-07-05", &fakeRealized{aggs: a, totals: t2})
	ctx := context.Background()

	if _, err := svc.Close(ctx, "2026-06"); err != nil {
		t.Fatalf("close: %v", err)
	}
	// fechar de novo → erro
	if _, err := svc.Close(ctx, "2026-06"); err != ErrAlreadyClosed {
		t.Fatalf("esperava ErrAlreadyClosed, got %v", err)
	}

	rep, err := svc.Monthly(ctx, "2026-06", "realizado", RegimeCompetencia)
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if rep.Status != StateAdjustable {
		t.Errorf("status=%s want fechado_ajustavel", rep.Status)
	}
	if rep.KPIs.TotalDespesas != 120000 {
		t.Errorf("despesas=%d want 120000", rep.KPIs.TotalDespesas)
	}
	if len(rep.Analitico.Despesas) != 1 || rep.Analitico.Despesas[0].CategoryName != "Alimentação" {
		t.Errorf("analitico errado: %+v", rep.Analitico.Despesas)
	}
	if rep.KPIs.PercentNoCredito != pct(45000, 120000) {
		t.Errorf("percentNoCredito=%d", rep.KPIs.PercentNoCredito)
	}
}

func TestService_LockState(t *testing.T) {
	a, t2 := despesa(120000)
	svc := newSvc(t, "2026-07-05", &fakeRealized{aggs: a, totals: t2})
	ctx := context.Background()
	_, _ = svc.Close(ctx, "2026-06")

	if st, _ := svc.LockState(ctx, "2026-06-10"); st != StateAdjustable {
		t.Errorf("ajustável esperado, got %s", st)
	}
	if err := svc.EnsureEditable(ctx, "2026-06-10"); err != nil {
		t.Errorf("ajustável deveria permitir edição: %v", err)
	}
	// avança o tempo > hard lock (2026-06-30 + 90 = 2026-09-28)
	svc.now = func() time.Time { tt, _ := time.Parse("2006-01-02", "2026-10-01"); return tt }
	if st, _ := svc.LockState(ctx, "2026-06-10"); st != StateBlocked {
		t.Errorf("bloqueado esperado, got %s", st)
	}
	if err := svc.EnsureEditable(ctx, "2026-06-10"); err != ErrMonthBlocked {
		t.Errorf("bloqueado deveria rejeitar: %v", err)
	}
}

func TestService_Recalculate_Idempotent(t *testing.T) {
	a, t2 := despesa(120000)
	fr := &fakeRealized{aggs: a, totals: t2}
	svc := newSvc(t, "2026-07-05", fr)
	ctx := context.Background()
	_, _ = svc.Close(ctx, "2026-06")

	// muda a fonte e recalcula → snapshot reflete o novo valor
	a2, t3 := despesa(200000)
	fr.aggs, fr.totals = a2, t3
	if err := svc.Recalculate(ctx, "2026-06"); err != nil {
		t.Fatalf("recalc: %v", err)
	}
	rep, _ := svc.Monthly(ctx, "2026-06", "realizado", RegimeCompetencia)
	if rep.KPIs.TotalDespesas != 200000 {
		t.Errorf("após recalc despesas=%d want 200000", rep.KPIs.TotalDespesas)
	}
}

func TestService_Quarterly_OnlyClosedMonths(t *testing.T) {
	a, t2 := despesa(120000)
	svc := newSvc(t, "2026-12-31", &fakeRealized{aggs: a, totals: t2})
	ctx := context.Background()
	_, _ = svc.Close(ctx, "2026-06") // só junho fechado no Q2

	rep, err := svc.Quarterly(ctx, 2026, 2, RegimeCompetencia)
	if err != nil {
		t.Fatalf("quarterly: %v", err)
	}
	if len(rep.IncludedMonths) != 1 || rep.IncludedMonths[0] != "2026-06" {
		t.Errorf("included=%v want [2026-06]", rep.IncludedMonths)
	}
	if len(rep.MissingMonths) != 2 {
		t.Errorf("missing=%v want abril e maio", rep.MissingMonths)
	}
	if rep.KPIs.TotalDespesas != 120000 {
		t.Errorf("despesas do trimestre=%d want 120000 (só junho)", rep.KPIs.TotalDespesas)
	}
	if len(rep.Monthly) != 1 {
		t.Errorf("mês a mês deveria ter 1 ponto")
	}
}
