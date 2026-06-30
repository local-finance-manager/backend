package budget

import (
	"context"
	"errors"
	"testing"
)

var errBoom = errors.New("boom")

// Erros de leitura da renda propagam em todos os fluxos que a consultam.
func TestService_IncomeReadErrors(t *testing.T) {
	svc := newSvc(t, &fakeIncome{err: errBoom}, &fakeWriter{})
	ctx := context.Background()
	if _, err := svc.GetPlan(ctx, "2026-06"); err == nil {
		t.Error("GetPlan deveria propagar erro de renda")
	}
	if _, err := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "X", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(10)}); err == nil {
		t.Error("CreateDestination deveria propagar erro de renda")
	}
	if _, err := svc.MaterializeAll(ctx, "2026-06"); err == nil {
		t.Error("MaterializeAll deveria propagar erro de renda")
	}
}

// Materializar individual de despesa SEM preset → ErrMissingPreset.
func TestService_MaterializeMissingPreset(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{})
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Mercado", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(3000)})
	if _, err := svc.Materialize(ctx, d.ID, MaterializeInput{}); err != ErrMissingPreset {
		t.Fatalf("esperava ErrMissingPreset, got %v", err)
	}
}

// Materializar com overrides cobre os ramos de buildTransaction.
func TestService_MaterializeWithOverrides(t *testing.T) {
	w := &fakeWriter{}
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, w)
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Lazer", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(1000)})
	desc := "ajustado"
	res, err := svc.Materialize(ctx, d.ID, MaterializeInput{
		SubcategoryID: strp("sub-y"), Amount: cents(9999), CompetenceDate: strp("2026-06-10"),
		PaymentDate: strp("2026-06-11"), Description: &desc, PaymentMethod: strp("pix"),
	})
	if err != nil {
		t.Fatalf("materialize override: %v", err)
	}
	if res.Amount != 9999 {
		t.Errorf("override amount=%d want 9999", res.Amount)
	}
	got := w.created[0]
	if got.SubcategoryID != "sub-y" || got.PaymentMethod != "pix" || got.CompetenceDate != "2026-06-10" ||
		got.PaymentDate == nil || *got.PaymentDate != "2026-06-11" || got.Description == nil || *got.Description != "ajustado" {
		t.Errorf("overrides não aplicados: %+v", got)
	}
}

// Falha na criação do lançamento propaga (sem materializar o destino).
func TestService_MaterializeCreateError(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{createErr: errBoom})
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500), PresetSubcategoryID: strp("sub-x")})
	if _, err := svc.Materialize(ctx, d.ID, MaterializeInput{}); err == nil {
		t.Error("erro na criação do lançamento deveria propagar")
	}
	plan, _ := svc.GetPlan(ctx, "2026-06")
	if plan.Destinations[0].Status != "planejado" {
		t.Error("destino não deveria materializar quando a criação falha")
	}
}

// MaterializeAll aborta se a criação de um lançamento falha.
func TestService_MaterializeAllCreateError(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{createErr: errBoom})
	ctx := context.Background()
	_, _ = svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Inv", Kind: KindInvestimento, Mode: ModePercentual, Percentage: pct(2000)})
	if _, err := svc.MaterializeAll(ctx, "2026-06"); err == nil {
		t.Error("bulk deveria abortar quando a criação falha")
	}
}

// CreateTemplate + ApplyTemplate + CopyPrevious caminhos de sucesso e erro.
func TestService_TemplatesAndCopy(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{})
	ctx := context.Background()

	tpl, err := svc.CreateTemplate(ctx, "Padrão", []TemplateItem{
		{Name: "Necessidades", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(5000)},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if err := svc.ApplyTemplate(ctx, "2026-06", tpl.ID); err != nil {
		t.Fatalf("apply template: %v", err)
	}
	if err := svc.ApplyTemplate(ctx, "2026-06", "nope"); err != ErrTemplateNotFound {
		t.Errorf("apply template inexistente: %v", err)
	}
	if err := svc.CopyPrevious(ctx, "2026-07"); err != nil { // copia de 2026-06
		t.Fatalf("copy previous: %v", err)
	}
	if err := svc.CopyPrevious(ctx, "bad"); err == nil {
		t.Error("copy previous com referência inválida deveria falhar")
	}
	ts, err := svc.ListTemplates(ctx)
	if err != nil || len(ts) != 1 {
		t.Errorf("list templates: %v len=%d", err, len(ts))
	}
}

// UpdateDestination de materializado é bloqueado.
func TestService_UpdateMaterializedBlocked(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{})
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500), PresetSubcategoryID: strp("sub-x")})
	_, _ = svc.Materialize(ctx, d.ID, MaterializeInput{})
	if _, err := svc.UpdateDestination(ctx, d.ID, DestinationInput{Reference: "2026-06", Name: "Z", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(1000)}); err != ErrAlreadyMaterialized {
		t.Errorf("editar materializado: %v want ErrAlreadyMaterialized", err)
	}
	if _, err := svc.UpdateDestination(ctx, "nope", DestinationInput{Reference: "2026-06", Name: "Z", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(1000)}); err != ErrDestinationNotFound {
		t.Errorf("editar inexistente: %v", err)
	}
}

// Após materializar, o plano reflete o "disponível" reduzido (branch de buildPlan).
func TestService_PlanAvailableAfterMaterialize(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 500000, allRealized: true}, &fakeWriter{})
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500), PresetSubcategoryID: strp("sub-x")})
	if _, err := svc.Materialize(ctx, d.ID, MaterializeInput{}); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	plan, _ := svc.GetPlan(ctx, "2026-06")
	if plan.AvailableAmount != 375000 { // 500000 - 125000
		t.Errorf("disponível=%d want 375000", plan.AvailableAmount)
	}
	if plan.Destinations[0].Status != "materializado" {
		t.Error("destino deveria estar materializado")
	}
}

func TestRoundDivZero(t *testing.T) {
	if roundDiv(10, 0) != 0 {
		t.Error("roundDiv por zero deveria ser 0")
	}
}
