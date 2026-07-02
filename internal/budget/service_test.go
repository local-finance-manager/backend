package budget

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/shared"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE allocation_destination (
			id TEXT PRIMARY KEY, reference TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL,
			mode TEXT NOT NULL, percentage INTEGER, fixed_amount INTEGER, preset_subcategory_id TEXT,
			preset_payment_method TEXT, preset_description TEXT, display_order INTEGER NOT NULL DEFAULT 0,
			materialized_transaction_id TEXT, materialized_amount INTEGER, materialized_at TEXT,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL, caixinha_id TEXT)`,
		`CREATE TABLE allocation_template (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE allocation_template_item (id TEXT PRIMARY KEY, template_id TEXT NOT NULL, name TEXT NOT NULL,
			kind TEXT NOT NULL, mode TEXT NOT NULL, percentage INTEGER, fixed_amount INTEGER,
			preset_subcategory_id TEXT, preset_payment_method TEXT, preset_description TEXT, display_order INTEGER NOT NULL DEFAULT 0, caixinha_id TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

// ── fakes dos ports ─────────────────────────────────────────────────────────

type fakeIncome struct {
	total       int64
	allRealized bool
	err         error
}

func (f *fakeIncome) MonthIncome(_ context.Context, _ string) (int64, bool, []shared.IncomeItem, error) {
	if f.err != nil {
		return 0, false, nil, f.err
	}
	items := []shared.IncomeItem{{TransactionID: "r1", Title: "Salário", Amount: f.total, Status: "realizado"}}
	if !f.allRealized {
		items[0].Status = "pendente"
	}
	return f.total, f.allRealized, items, nil
}

type fakeWriter struct {
	created   []shared.NewTransaction
	deleted   []string
	nextID    int
	createErr error
}

func (f *fakeWriter) Create(_ context.Context, in shared.NewTransaction) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, in)
	f.nextID++
	return "tx-" + string(rune('0'+f.nextID)), nil
}
func (f *fakeWriter) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func newSvc(t *testing.T, income *fakeIncome, writer *fakeWriter) *Service {
	return NewService(Deps{
		Repo:           NewSQLiteRepository(newDB(t)),
		Income:         income,
		Txns:           writer,
		InvestSubcatID: "sub-trf-aporte",
	})
}

type fakeAporter struct {
	caixinhaID string
	amount     int64
	calls      int
}

func (f *fakeAporter) RegisterAporte(_ context.Context, caixinhaID string, amount int64, _ string, _ *string) (string, error) {
	f.calls++
	f.caixinhaID = caixinhaID
	f.amount = amount
	return "tx-aporte", nil
}

// ── testes ──────────────────────────────────────────────────────────────────

func TestService_CreateAndPlan(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 500000, allRealized: false}, &fakeWriter{})
	ctx := context.Background()

	_, err := svc.CreateDestination(ctx, DestinationInput{
		Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	plan, err := svc.GetPlan(ctx, "2026-06")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Income.Total != 500000 || plan.CanMaterialize {
		t.Errorf("renda pendente → canMaterialize false; got total=%d can=%v", plan.Income.Total, plan.CanMaterialize)
	}
	if len(plan.Destinations) != 1 || plan.Destinations[0].ComputedAmount != 125000 {
		t.Errorf("destino computado errado: %+v", plan.Destinations)
	}
}

func TestService_CreateBlocksOverAllocation(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 100000, allRealized: true}, &fakeWriter{})
	ctx := context.Background()
	_, _ = svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "A", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(9000)})
	_, err := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "B", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2000)})
	if err != ErrOverAllocated {
		t.Fatalf("esperava ErrOverAllocated, got %v", err)
	}
}

func TestService_MaterializeCaixinha(t *testing.T) {
	writer := &fakeWriter{}
	aporter := &fakeAporter{}
	svc := NewService(Deps{
		Repo: NewSQLiteRepository(newDB(t)), Income: &fakeIncome{total: 500000, allRealized: true},
		Txns: writer, Caixinha: aporter, InvestSubcatID: "sub-trf-aporte",
	})
	ctx := context.Background()
	d, err := svc.CreateDestination(ctx, DestinationInput{
		Reference: "2026-06", Name: "Investir", Kind: KindInvestimento, Mode: ModePercentual,
		Percentage: pct(2000), CaixinhaID: strp("cx1"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := svc.Materialize(ctx, d.ID, MaterializeInput{})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if aporter.calls != 1 || aporter.caixinhaID != "cx1" || aporter.amount != 100000 {
		t.Fatalf("deveria aportar 100000 em cx1: calls=%d cx=%s amt=%d", aporter.calls, aporter.caixinhaID, aporter.amount)
	}
	if len(writer.created) != 0 {
		t.Fatalf("destino de caixinha NÃO deveria criar lançamento normal: %+v", writer.created)
	}
	if res.TransactionID != "tx-aporte" {
		t.Fatalf("txID do aporte inesperado: %s", res.TransactionID)
	}
	// o plano expõe o caixinhaId do destino
	plan, _ := svc.GetPlan(ctx, "2026-06")
	if plan.Destinations[0].CaixinhaID == nil || *plan.Destinations[0].CaixinhaID != "cx1" {
		t.Fatalf("plan deveria expor caixinhaId: %+v", plan.Destinations[0])
	}
}

func TestService_MaterializeBlockedWhilePending(t *testing.T) {
	svc := newSvc(t, &fakeIncome{total: 500000, allRealized: false}, &fakeWriter{})
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500), PresetSubcategoryID: strp("sub-x")})
	if _, err := svc.Materialize(ctx, d.ID, MaterializeInput{}); err != ErrIncomePending {
		t.Fatalf("esperava ErrIncomePending, got %v", err)
	}
}

func TestService_MaterializeAndUndo(t *testing.T) {
	writer := &fakeWriter{}
	svc := newSvc(t, &fakeIncome{total: 500000, allRealized: true}, writer)
	ctx := context.Background()
	d, _ := svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Aluguel", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(2500), PresetSubcategoryID: strp("sub-x")})

	res, err := svc.Materialize(ctx, d.ID, MaterializeInput{})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if res.Amount != 125000 || res.TransactionID == "" {
		t.Errorf("resultado materialize: %+v", res)
	}
	if len(writer.created) != 1 || writer.created[0].Status != "realizado" || writer.created[0].Title != "Aluguel" {
		t.Errorf("lançamento criado errado: %+v", writer.created)
	}

	// não pode materializar de novo
	if _, err := svc.Materialize(ctx, d.ID, MaterializeInput{}); err != ErrAlreadyMaterialized {
		t.Errorf("dupla materialização deveria falhar, got %v", err)
	}

	// desfazer exclui o lançamento e volta a planejado
	if err := svc.Undo(ctx, d.ID); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if len(writer.deleted) != 1 {
		t.Errorf("undo deveria excluir o lançamento")
	}
	plan, _ := svc.GetPlan(ctx, "2026-06")
	if plan.Destinations[0].Status != "planejado" {
		t.Errorf("destino deveria voltar a planejado")
	}
}

func TestService_MaterializeAll_SkipsWithoutPreset(t *testing.T) {
	writer := &fakeWriter{}
	svc := newSvc(t, &fakeIncome{total: 1000000, allRealized: true}, writer)
	ctx := context.Background()
	// despesa SEM preset → pulada; investimento sem preset → usa default → materializa
	_, _ = svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Mercado", Kind: KindDespesa, Mode: ModePercentual, Percentage: pct(3000)})
	_, _ = svc.CreateDestination(ctx, DestinationInput{Reference: "2026-06", Name: "Investir", Kind: KindInvestimento, Mode: ModePercentual, Percentage: pct(2000)})

	res, err := svc.MaterializeAll(ctx, "2026-06")
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if len(res.Materialized) != 1 || len(res.Skipped) != 1 {
		t.Fatalf("esperava 1 materializado (investimento) e 1 pulado (despesa s/ preset): %+v", res)
	}
	if writer.created[0].SubcategoryID != "sub-trf-aporte" {
		t.Errorf("investimento sem preset deveria usar a subcat default, got %q", writer.created[0].SubcategoryID)
	}
}

func strp(s string) *string { return &s }
