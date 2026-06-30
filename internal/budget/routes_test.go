package budget_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/budget"
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
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE allocation_template (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE allocation_template_item (id TEXT PRIMARY KEY, template_id TEXT NOT NULL, name TEXT NOT NULL,
			kind TEXT NOT NULL, mode TEXT NOT NULL, percentage INTEGER, fixed_amount INTEGER,
			preset_subcategory_id TEXT, preset_payment_method TEXT, preset_description TEXT, display_order INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

type fakeIncome struct {
	total       int64
	allRealized bool
}

func (f *fakeIncome) MonthIncome(_ context.Context, _ string) (int64, bool, []shared.IncomeItem, error) {
	st := "realizado"
	if !f.allRealized {
		st = "pendente"
	}
	return f.total, f.allRealized, []shared.IncomeItem{{TransactionID: "r1", Title: "Salário", Amount: f.total, Status: st}}, nil
}

type fakeWriter struct {
	n       int
	deleted []string
}

func (f *fakeWriter) Create(_ context.Context, _ shared.NewTransaction) (string, error) {
	f.n++
	return "tx-" + string(rune('a'+f.n)), nil
}
func (f *fakeWriter) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func newRouter(db *sql.DB, income budget.IncomeReader, writer budget.TransactionWriter) http.Handler {
	svc := budget.NewService(budget.Deps{
		Repo: budget.NewSQLiteRepository(db), Income: income, Txns: writer, InvestSubcatID: "sub-trf-aporte",
	})
	r := chi.NewRouter()
	r.Route("/api/income", budget.Routes(budget.NewHandler(svc)))
	return r
}

func req(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	rr := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	rr.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, rr)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestBudgetRoutes_FullFlow(t *testing.T) {
	db := newDB(t)
	writer := &fakeWriter{}
	router := newRouter(db, &fakeIncome{total: 500000, allRealized: true}, writer)

	// plano vazio
	if code, _ := req(t, router, http.MethodGet, "/api/income/plan?reference=2026-06", ""); code != http.StatusOK {
		t.Fatalf("plan: %d", code)
	}

	// cria destino (com preset p/ materializar)
	code, body := req(t, router, http.MethodPost, "/api/income/destinations",
		`{"reference":"2026-06","name":"Aluguel","kind":"despesa","mode":"percentual","percentage":2500,"presetSubcategoryId":"sub-x","presetPaymentMethod":"pix"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, body)
	}
	id, _ := body["id"].(string)

	// validações
	if c, _ := req(t, router, http.MethodPost, "/api/income/destinations", `{`); c != http.StatusBadRequest {
		t.Errorf("bad json: %d", c)
	}
	if c, _ := req(t, router, http.MethodPost, "/api/income/destinations", `{"reference":"2026-06","name":"","kind":"despesa","mode":"percentual","percentage":1}`); c != http.StatusBadRequest {
		t.Errorf("nome vazio: %d", c)
	}
	// excesso de 100% (já tem 25%; +80% = 105%)
	if c, _ := req(t, router, http.MethodPost, "/api/income/destinations", `{"reference":"2026-06","name":"X","kind":"despesa","mode":"percentual","percentage":8000}`); c != http.StatusConflict {
		t.Errorf("over-allocation: %d want 409", c)
	}

	// update
	if c, _ := req(t, router, http.MethodPut, "/api/income/destinations/"+id,
		`{"reference":"2026-06","name":"Aluguel 2","kind":"despesa","mode":"percentual","percentage":3000,"presetSubcategoryId":"sub-x"}`); c != http.StatusNoContent {
		t.Errorf("update: %d", c)
	}

	// materializa
	code, mat := req(t, router, http.MethodPost, "/api/income/destinations/"+id+"/materialize", `{}`)
	if code != http.StatusOK || mat["status"] != "materializado" {
		t.Fatalf("materialize: %d %v", code, mat)
	}
	// dupla materialização → 409
	if c, _ := req(t, router, http.MethodPost, "/api/income/destinations/"+id+"/materialize", `{}`); c != http.StatusConflict {
		t.Errorf("dupla materialização: %d want 409", c)
	}
	// não pode editar/excluir materializado
	if c, _ := req(t, router, http.MethodDelete, "/api/income/destinations/"+id, ""); c != http.StatusConflict {
		t.Errorf("delete materializado: %d want 409", c)
	}
	// desfaz
	if c, _ := req(t, router, http.MethodDelete, "/api/income/destinations/"+id+"/materialize", ""); c != http.StatusNoContent {
		t.Errorf("undo: %d", c)
	}
	if len(writer.deleted) != 1 {
		t.Errorf("undo deveria excluir o lançamento")
	}
	// agora exclui o planejado
	if c, _ := req(t, router, http.MethodDelete, "/api/income/destinations/"+id, ""); c != http.StatusNoContent {
		t.Errorf("delete planejado: %d", c)
	}
}

func TestBudgetRoutes_MaterializeBlockedWhilePending(t *testing.T) {
	db := newDB(t)
	router := newRouter(db, &fakeIncome{total: 500000, allRealized: false}, &fakeWriter{})
	_, body := req(t, router, http.MethodPost, "/api/income/destinations",
		`{"reference":"2026-06","name":"Aluguel","kind":"despesa","mode":"percentual","percentage":2500,"presetSubcategoryId":"sub-x"}`)
	id, _ := body["id"].(string)
	if c, _ := req(t, router, http.MethodPost, "/api/income/destinations/"+id+"/materialize", `{}`); c != http.StatusConflict {
		t.Errorf("materialize com renda pendente: %d want 409", c)
	}
	// materialize-all também bloqueado
	if c, _ := req(t, router, http.MethodPost, "/api/income/plan/2026-06/materialize-all", ""); c != http.StatusConflict {
		t.Errorf("bulk com renda pendente: %d want 409", c)
	}
}

func TestBudgetRoutes_BulkTemplatesCopy(t *testing.T) {
	db := newDB(t)
	writer := &fakeWriter{}
	router := newRouter(db, &fakeIncome{total: 1000000, allRealized: true}, writer)

	// cria template
	code, tb := req(t, router, http.MethodPost, "/api/income/templates",
		`{"name":"50/30/20","items":[{"name":"Necessidades","kind":"despesa","mode":"percentual","percentage":5000,"presetSubcategoryId":"sub-n"},{"name":"Poupança","kind":"investimento","mode":"percentual","percentage":2000}]}`)
	if code != http.StatusCreated {
		t.Fatalf("create template: %d %v", code, tb)
	}
	tid, _ := tb["id"].(string)
	if c, _ := req(t, router, http.MethodGet, "/api/income/templates", ""); c != http.StatusOK {
		t.Errorf("list templates: %d", c)
	}
	// aplica template ao mês
	if c, _ := req(t, router, http.MethodPost, "/api/income/plan/2026-06/apply-template", `{"templateId":"`+tid+`"}`); c != http.StatusNoContent {
		t.Errorf("apply template: %d", c)
	}
	// materializa todos: investimento (default subcat) + necessidades (preset) materializam
	code, bulk := req(t, router, http.MethodPost, "/api/income/plan/2026-06/materialize-all", "")
	if code != http.StatusOK {
		t.Fatalf("bulk: %d %v", code, bulk)
	}
	mat, _ := bulk["materialized"].([]any)
	if len(mat) != 2 {
		t.Errorf("esperava 2 materializados (ambos com subcat), got %d", len(mat))
	}

	// copiar do mês anterior (2026-06 → 2026-07)
	if c, _ := req(t, router, http.MethodPost, "/api/income/plan/2026-07/copy-previous", ""); c != http.StatusNoContent {
		t.Errorf("copy previous: %d", c)
	}
	_, plan := req(t, router, http.MethodGet, "/api/income/plan?reference=2026-07", "")
	dests, _ := plan["destinations"].([]any)
	if len(dests) != 2 {
		t.Errorf("copy-previous deveria recriar 2 destinos, got %d", len(dests))
	}
}

func TestBudgetRoutes_ErrorPaths(t *testing.T) {
	db := newDB(t)
	router := newRouter(db, &fakeIncome{total: 500000, allRealized: true}, &fakeWriter{})
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"update missing", http.MethodPut, "/api/income/destinations/nope", `{"reference":"2026-06","name":"X","kind":"despesa","mode":"percentual","percentage":1}`, http.StatusNotFound},
		{"update bad json", http.MethodPut, "/api/income/destinations/x", `{`, http.StatusBadRequest},
		{"delete missing", http.MethodDelete, "/api/income/destinations/nope", "", http.StatusNotFound},
		{"materialize missing", http.MethodPost, "/api/income/destinations/nope/materialize", `{}`, http.StatusNotFound},
		{"undo not materialized", http.MethodDelete, "/api/income/destinations/nope/materialize", "", http.StatusNotFound},
		{"apply template missing", http.MethodPost, "/api/income/plan/2026-06/apply-template", `{"templateId":"nope"}`, http.StatusNotFound},
		{"apply template bad json", http.MethodPost, "/api/income/plan/2026-06/apply-template", `{`, http.StatusBadRequest},
		{"create template bad json", http.MethodPost, "/api/income/templates", `{`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if c, _ := req(t, router, tc.method, tc.path, tc.body); c != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, c, tc.want)
		}
	}
}

type errIncome struct{}

func (errIncome) MonthIncome(_ context.Context, _ string) (int64, bool, []shared.IncomeItem, error) {
	return 0, false, nil, errors.New("boom")
}

func TestBudgetRoutes_HandlerErrorPaths(t *testing.T) {
	db := newDB(t)

	// GetPlan sem reference → usa o mês atual (default) e responde 200.
	okRouter := newRouter(db, &fakeIncome{total: 100, allRealized: true}, &fakeWriter{})
	if c, _ := req(t, okRouter, http.MethodGet, "/api/income/plan", ""); c != http.StatusOK {
		t.Errorf("plan default reference: %d", c)
	}
	// copy-previous com referência inválida → 400.
	if c, _ := req(t, okRouter, http.MethodPost, "/api/income/plan/bad/copy-previous", ""); c != http.StatusBadRequest {
		t.Errorf("copy-previous ref inválida: %d want 400", c)
	}

	// erro de leitura da renda → 500 no GetPlan.
	errRouter := newRouter(db, errIncome{}, &fakeWriter{})
	if c, _ := req(t, errRouter, http.MethodGet, "/api/income/plan?reference=2026-06", ""); c != http.StatusInternalServerError {
		t.Errorf("plan income error: %d want 500", c)
	}

	// erro de repositório (DB fechado) → 500 no ListTemplates.
	closedDB := newDB(t)
	closedRouter := newRouter(closedDB, &fakeIncome{}, &fakeWriter{})
	closedDB.Close()
	if c, _ := req(t, closedRouter, http.MethodGet, "/api/income/templates", ""); c != http.StatusInternalServerError {
		t.Errorf("list templates DB error: %d want 500", c)
	}
}

func TestBudgetRepo_DBErrors(t *testing.T) {
	db := newDB(t)
	repo := budget.NewSQLiteRepository(db)
	ctx := context.Background()
	d := budget.Destination{ID: "d1", Reference: "2026-06", Name: "X", Kind: budget.KindDespesa, Mode: budget.ModePercentual}
	db.Close()

	if _, err := repo.ListDestinations(ctx, "2026-06"); err == nil {
		t.Error("ListDestinations deveria falhar")
	}
	if _, err := repo.GetDestination(ctx, "d1"); err == nil {
		t.Error("GetDestination deveria falhar")
	}
	if err := repo.CreateDestination(ctx, d); err == nil {
		t.Error("CreateDestination deveria falhar")
	}
	if err := repo.CreateDestinations(ctx, []budget.Destination{d}); err == nil {
		t.Error("CreateDestinations deveria falhar")
	}
	if err := repo.UpdateDestination(ctx, d); err == nil {
		t.Error("UpdateDestination deveria falhar")
	}
	if err := repo.DeleteDestination(ctx, "d1"); err == nil {
		t.Error("DeleteDestination deveria falhar")
	}
	if _, err := repo.SetMaterialized(ctx, "d1", "tx", 1, time.Now()); err == nil {
		t.Error("SetMaterialized deveria falhar")
	}
	if err := repo.ClearMaterialized(ctx, "d1"); err == nil {
		t.Error("ClearMaterialized deveria falhar")
	}
	if _, err := repo.ListTemplates(ctx); err == nil {
		t.Error("ListTemplates deveria falhar")
	}
	if _, err := repo.GetTemplate(ctx, "t1"); err == nil {
		t.Error("GetTemplate deveria falhar")
	}
	if err := repo.CreateTemplate(ctx, budget.Template{ID: "t1", Name: "T"}); err == nil {
		t.Error("CreateTemplate deveria falhar")
	}
}
