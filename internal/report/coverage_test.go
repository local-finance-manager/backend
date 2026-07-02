package report

import (
	"context"
	"net/http"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
)

// fecha vários meses com o mesmo total e exercita comparativos/períodos longos.
func multiMonth(total int64, refs ...string) (*fakeRealized, map[string]shared.MonthlyTotals) {
	aggs := map[string][]shared.SubcategoryAggregate{}
	totals := map[string]shared.MonthlyTotals{}
	for _, r := range refs {
		aggs[r] = []shared.SubcategoryAggregate{{SubcategoryID: "s1", CategoryID: "cat-1", Type: "despesa", Total: total, TxCount: 2}}
		totals[r] = shared.MonthlyTotals{Receitas: 800000, Despesas: total, SaldoPeriodo: 800000 - total, SaldoInicial: 1000000, SaldoFinal: 1000000 + (800000 - total), TxCount: 2}
	}
	return &fakeRealized{aggs: aggs, totals: totals}, totals
}

func TestService_ClosedMonthsComparativeAndLong(t *testing.T) {
	fr, _ := multiMonth(120000, "2026-04", "2026-05", "2026-06")
	svc := newSvc(t, "2026-07-05", fr)
	ctx := context.Background()
	for _, r := range []string{"2026-04", "2026-05", "2026-06"} {
		if _, err := svc.Close(ctx, r); err != nil {
			t.Fatalf("close %s: %v", r, err)
		}
	}

	// mensal de mês fechado → lê snapshot; comparativo com maio (fechado, não-parcial)
	rep, err := svc.Monthly(ctx, "2026-06", "realizado", RegimeCompetencia)
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if rep.Status != StateAdjustable {
		t.Errorf("status=%s", rep.Status)
	}
	if rep.Comparativos.PeriodoAnterior == nil || rep.Comparativos.PeriodoAnterior.Partial {
		t.Errorf("comparativo com mês fechado não deveria ser parcial: %+v", rep.Comparativos.PeriodoAnterior)
	}

	// trimestral Q2 com os 3 meses fechados
	q, err := svc.Quarterly(ctx, 2026, 2, RegimeCompetencia)
	if err != nil {
		t.Fatalf("quarterly: %v", err)
	}
	if len(q.IncludedMonths) != 3 || len(q.Monthly) != 3 {
		t.Errorf("trimestre deveria incluir 3 meses fechados: %+v", q.IncludedMonths)
	}
	if q.KPIs.TotalDespesas != 360000 {
		t.Errorf("despesas do trimestre=%d want 360000", q.KPIs.TotalDespesas)
	}

	// anual e semestral exercitam os caminhos longos
	if _, err := svc.Annual(ctx, 2026, RegimeCompetencia); err != nil {
		t.Fatalf("annual: %v", err)
	}
	if _, err := svc.Semiannual(ctx, 2026, 1, RegimeCompetencia); err != nil {
		t.Fatalf("semiannual: %v", err)
	}

	// ListClosings
	cs, err := svc.ListClosings(ctx)
	if err != nil || len(cs) != 3 {
		t.Errorf("list closings: %v len=%d", err, len(cs))
	}
}

func TestService_AfterChangeAndLock(t *testing.T) {
	fr, _ := multiMonth(120000, "2026-06")
	svc := newSvc(t, "2026-07-05", fr)
	ctx := context.Background()
	_, _ = svc.Close(ctx, "2026-06")

	// AfterChange num mês fechado-ajustável recalcula; mês aberto e data vazia são ignorados.
	if err := svc.AfterChange(ctx, "2026-06-10", "2026-08-01", ""); err != nil {
		t.Fatalf("after change: %v", err)
	}
	if err := svc.EnsureEditable(ctx, "2026-06-10"); err != nil {
		t.Errorf("ajustável deveria permitir: %v", err)
	}
	// data inválida em AfterChange é ignorada (não quebra).
	if err := svc.AfterChange(ctx, "data-ruim"); err != nil {
		t.Errorf("data inválida deveria ser ignorada: %v", err)
	}
}

func TestBuildInsights(t *testing.T) {
	analitico := Analitico{Despesas: []CatAnalitico{
		{CategoryID: "cat-1", CategoryName: "Alimentação", Total: 50000, Percent: 50},
		{CategoryID: "cat-2", CategoryName: "Lazer", Total: 20000, Percent: 20},
	}}
	kpis := KPIs{TotalReceitas: 100000, TaxaPoupanca: 30}
	prevByCat := map[string]int64{"cat-1": 30000, "cat-2": 40000} // A subiu 66%, B caiu 50%
	ins := BuildInsights(analitico, kpis, &Comparison{}, prevByCat)
	if len(ins) == 0 {
		t.Fatal("esperava insights")
	}
	joined := ""
	for _, s := range ins {
		joined += s + "\n"
	}
	if !contains(joined, "subiu") || !contains(joined, "caiu") || !contains(joined, "poupança") {
		t.Errorf("insights incompletos:\n%s", joined)
	}

	// taxa de poupança negativa → alerta
	neg := BuildInsights(Analitico{}, KPIs{TotalReceitas: 100000, TaxaPoupanca: -10}, nil, nil)
	if len(neg) == 0 {
		t.Error("esperava alerta de poupança negativa")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

type errTree struct{}

func (errTree) Tree(_ context.Context) ([]shared.CategoryNode, error) { return nil, context.Canceled }

func TestService_ErrorPaths(t *testing.T) {
	fr, _ := multiMonth(120000, "2026-06")

	// service com DB fechado → erros de repositório nos handlers e leituras.
	db := newReportDB(t)
	repo := NewSQLiteRepository(db)
	svc := NewService(Deps{Repo: repo, Realized: fr, Pending: &fakePending{}, Tree: fakeTree{}, Payments: fakePayments{m: map[string]int64{}}, Cash: &fakeCash{pay: map[string]int64{}}})
	db.Close()
	router := newRouter(svc)

	// regime=competencia força o caminho de snapshot (repositório) → 500 com DB fechado.
	for _, p := range []string{
		"/api/reports/monthly?reference=2026-06&regime=competencia",
		"/api/reports/annual?year=2026&regime=competencia",
		"/api/reports/closings",
	} {
		if c, _ := do(t, router, http.MethodGet, p, ""); c != http.StatusInternalServerError {
			t.Errorf("GET %s com DB fechado: %d want 500", p, c)
		}
	}
	if c, _ := do(t, router, http.MethodPost, "/api/reports/closings/2026-06/recalculate", ""); c != http.StatusInternalServerError {
		t.Errorf("recalculate DB fechado: %d want 500", c)
	}

	// tree com erro → lookup falha no Monthly.
	svc2 := newSvc(t, "2026-07-05", fr)
	svc2.tree = errTree{}
	if _, err := svc2.Monthly(context.Background(), "2026-06", "realizado", RegimeCompetencia); err == nil {
		t.Error("Monthly deveria falhar com tree em erro")
	}

	// Q1 cobre o ramo de "trimestre anterior = Q4 do ano anterior".
	svc3 := newSvc(t, "2026-07-05", fr)
	if _, err := svc3.Quarterly(context.Background(), 2026, 1, RegimeCompetencia); err != nil {
		t.Errorf("quarterly Q1: %v", err)
	}
}

func TestDateHelpersInvalid(t *testing.T) {
	if _, err := ReferenceOf("bad"); err == nil {
		t.Error("ReferenceOf inválida")
	}
	if _, err := HardLockDate("bad"); err == nil {
		t.Error("HardLockDate inválida")
	}
	if _, err := MonthEnded("bad", "2026-01-01"); err == nil {
		t.Error("MonthEnded inválida")
	}
	if _, err := PrevReference("bad"); err == nil {
		t.Error("PrevReference inválida")
	}
	if _, err := SameMonthPrevYear("bad"); err == nil {
		t.Error("SameMonthPrevYear inválida")
	}
	if _, err := MonthLastDay("bad"); err == nil {
		t.Error("MonthLastDay inválida")
	}
	if itoa(-5) != "-5" || itoa(0) != "0" {
		t.Error("itoa")
	}
}
