package transaction_test

import (
	"context"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

// Cenário do usuário: salário com competência 30/jun mas pago em 01/jul deve pertencer
// a JULHO (regime de caixa) na lista, no resumo e na base de renda da Receitas.
func TestCashRegime_ReceitaPagaNoMesSeguinte(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-r", "Renda", "receita", "sub-r", "Salário")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	if err := repo.Create(ctx, mkTransaction("sal", "Salário", "sub-r", 500000,
		transaction.TypeReceita, transaction.MethodPix, transaction.StatusRealizado,
		"2026-06-30", strPtr("2026-07-01"))); err != nil {
		t.Fatalf("create: %v", err)
	}

	jun := transaction.TransactionFilter{CompetenceDateFrom: strPtr("2026-06-01"), CompetenceDateTo: strPtr("2026-06-30")}
	jul := transaction.TransactionFilter{CompetenceDateFrom: strPtr("2026-07-01"), CompetenceDateTo: strPtr("2026-07-31")}

	// Lista: aparece em JULHO, não em junho
	listJul, _ := repo.List(ctx, jul, shared.DefaultPagination())
	if len(listJul) != 1 || listJul[0].ID != "sal" {
		t.Fatalf("lista de julho deveria conter o salário (pago em jul), veio %d", len(listJul))
	}
	listJun, _ := repo.List(ctx, jun, shared.DefaultPagination())
	if len(listJun) != 0 {
		t.Fatalf("lista de junho NÃO deveria conter o salário, veio %d", len(listJun))
	}

	// Resumo: conta em julho
	sJul, _ := repo.GetSummary(ctx, jul)
	if sJul.TotalReceitas != 500000 {
		t.Fatalf("resumo de julho: receitas esperado 500000, veio %d", sJul.TotalReceitas)
	}
	sJun, _ := repo.GetSummary(ctx, jun)
	if sJun.TotalReceitas != 0 {
		t.Fatalf("resumo de junho: receitas esperado 0, veio %d", sJun.TotalReceitas)
	}

	// Base de renda da Receitas: conta em julho
	inc := transaction.NewIncomeReader(db)
	totJul, _, _, _ := inc.MonthIncome(ctx, "2026-07")
	if totJul != 500000 {
		t.Fatalf("renda de julho esperada 500000, veio %d", totJul)
	}
	totJun, _, _, _ := inc.MonthIncome(ctx, "2026-06")
	if totJun != 0 {
		t.Fatalf("renda de junho esperada 0, veio %d", totJun)
	}
}
