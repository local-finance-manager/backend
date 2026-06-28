package report

import (
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
)

func TestMonthLastDay(t *testing.T) {
	cases := map[string]string{
		"2026-01": "2026-01-31",
		"2026-02": "2026-02-28",
		"2024-02": "2024-02-29", // bissexto
		"2026-04": "2026-04-30",
		"2026-12": "2026-12-31",
	}
	for ref, want := range cases {
		got, err := MonthLastDay(ref)
		if err != nil || got != want {
			t.Errorf("MonthLastDay(%s)=%q,%v want %q", ref, got, err, want)
		}
	}
}

func TestHardLockDate(t *testing.T) {
	// 2026-01-31 + 90 dias = 2026-05-01
	got, err := HardLockDate("2026-01")
	if err != nil || got != "2026-05-01" {
		t.Errorf("HardLockDate(2026-01)=%q,%v want 2026-05-01", got, err)
	}
}

func TestMonthEnded(t *testing.T) {
	ended, _ := MonthEnded("2026-06", "2026-06-30")
	if ended {
		t.Error("último dia ainda não passou → não terminou")
	}
	ended, _ = MonthEnded("2026-06", "2026-07-01")
	if !ended {
		t.Error("dia seguinte ao último → terminou")
	}
}

func TestDeriveLockState(t *testing.T) {
	if DeriveLockState(false, "2026-05-01", "2026-06-01") != StateOpen {
		t.Error("sem fechamento → aberto")
	}
	if DeriveLockState(true, "2026-05-01", "2026-04-30") != StateAdjustable {
		t.Error("hoje <= hardLock → ajustável")
	}
	if DeriveLockState(true, "2026-05-01", "2026-05-02") != StateBlocked {
		t.Error("hoje > hardLock → bloqueado")
	}
}

func TestPeriodMonths(t *testing.T) {
	if got := MonthsInQuarter(2026, 2); got[0] != "2026-04" || got[2] != "2026-06" || len(got) != 3 {
		t.Errorf("Q2 errado: %v", got)
	}
	if got := MonthsInSemester(2026, 2); got[0] != "2026-07" || got[5] != "2026-12" {
		t.Errorf("S2 errado: %v", got)
	}
	if got := MonthsInYear(2026); len(got) != 12 || got[11] != "2026-12" {
		t.Errorf("ano errado: %v", got)
	}
}

func TestNavReferences(t *testing.T) {
	if p, _ := PrevReference("2026-01"); p != "2025-12" {
		t.Errorf("PrevReference virada de ano: %q", p)
	}
	if y, _ := SameMonthPrevYear("2026-06"); y != "2025-06" {
		t.Errorf("SameMonthPrevYear: %q", y)
	}
}

func TestBuildAnaliticoPercents(t *testing.T) {
	tree := []shared.CategoryNode{{
		CategoryID: "cat-1", CategoryName: "Alimentação", CategoryColor: "#27AE60", Type: "despesa",
		Subcategories: []shared.SubcategoryNode{{ID: "s1", Name: "Mercado"}, {ID: "s2", Name: "Delivery"}},
	}}
	lk := NewCategoryLookup(tree)
	aggs := []shared.SubcategoryAggregate{
		{SubcategoryID: "s1", CategoryID: "cat-1", Type: "despesa", Total: 90000, TxCount: 3},
		{SubcategoryID: "s2", CategoryID: "cat-1", Type: "despesa", Total: 30000, TxCount: 1},
	}
	a := BuildAnalitico(aggs, lk)
	if len(a.Despesas) != 1 || a.Despesas[0].Total != 120000 || a.Despesas[0].Percent != 100 {
		t.Fatalf("categoria errada: %+v", a.Despesas)
	}
	subs := a.Despesas[0].Subcategorias
	if subs[0].Name != "Mercado" || subs[0].Percent != 75 || subs[1].Percent != 25 {
		t.Errorf("percentuais de subcategoria errados: %+v", subs)
	}
}

func TestBuildKPIs(t *testing.T) {
	totals := shared.MonthlyTotals{Receitas: 800000, Despesas: 450000, SaldoPeriodo: 350000, TxCount: 50}
	aggs := []shared.SubcategoryAggregate{{Type: "despesa", Total: 450000, TxCount: 9}}
	k := BuildKPIs(totals, aggs, 38)
	if k.TaxaPoupanca != 43 { // (800000-450000)/800000 = 43%
		t.Errorf("taxaPoupanca=%d want 43", k.TaxaPoupanca)
	}
	if k.TicketMedio != 50000 { // 450000/9
		t.Errorf("ticketMedio=%d want 50000", k.TicketMedio)
	}
	if k.PercentNoCredito != 38 {
		t.Errorf("percentNoCredito=%d want 38", k.PercentNoCredito)
	}
}

func TestDeltaPct(t *testing.T) {
	if deltaPct(120, 100) != 20 {
		t.Error("delta +20%")
	}
	if deltaPct(80, 100) != -20 {
		t.Error("delta -20%")
	}
	if deltaPct(100, 0) != 0 {
		t.Error("base zero → 0")
	}
}
