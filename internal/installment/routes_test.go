package installment_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/installment"
	"github.com/local-finance-manager/backend/internal/shared"
)

func newInstallmentRouter(db *sql.DB) http.Handler {
	svc := installment.NewService(installment.Deps{
		Repo:  installment.NewSQLiteRepository(db),
		Subs:  &fakeSubs{typ: "despesa"},
		Cards: &fakeCards{},
		Refs:  &fakeRefs{},
	})
	h := installment.NewHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/installments", installment.Routes(h))
	return r
}

func installmentRouterWith(db *sql.DB, subs installment.SubcategoryReader, cards installment.CreditCardChecker, refs installment.InvoiceReferenceResolver) http.Handler {
	svc := installment.NewService(installment.Deps{
		Repo: installment.NewSQLiteRepository(db), Subs: subs, Cards: cards, Refs: refs,
	})
	r := chi.NewRouter()
	r.Route("/api/installments", installment.Routes(installment.NewHandler(svc)))
	return r
}

func instReq(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestInstallmentRoutes_FullFlow(t *testing.T) {
	db := newTestDB(t) // semeia card-1, sub-1 (despesa)
	router := newInstallmentRouter(db)

	create := `{"credit_card_id":"card-1","subcategory_id":"sub-1","title":"Notebook","installments_count":3,"input_mode":"by_total","total_amount":10000,"purchase_date":"2026-06-22"}`

	// Preview (sem persistir)
	if code, resp := instReq(t, router, http.MethodPost, "/api/installments/preview", create); code != http.StatusOK || resp["installments"] == nil {
		t.Fatalf("preview: got %d body %v", code, resp)
	}
	// Preview — bad json
	if code, _ := instReq(t, router, http.MethodPost, "/api/installments/preview", `{`); code != http.StatusBadRequest {
		t.Errorf("preview bad json: got %d, want 400", code)
	}

	// Create
	code, body := instReq(t, router, http.MethodPost, "/api/installments", create)
	if code != http.StatusCreated {
		t.Fatalf("create: got %d body %v", code, body)
	}
	gid, _ := body["id"].(string)
	if gid == "" {
		t.Fatalf("create: no id %v", body)
	}
	// Create — validation error (installments_count < 2)
	bad := `{"credit_card_id":"card-1","subcategory_id":"sub-1","title":"x","installments_count":1,"input_mode":"by_total","total_amount":10000,"purchase_date":"2026-06-22"}`
	if code, _ := instReq(t, router, http.MethodPost, "/api/installments", bad); code != http.StatusBadRequest {
		t.Errorf("create invalid: got %d, want 400", code)
	}
	// Create — bad json
	if code, _ := instReq(t, router, http.MethodPost, "/api/installments", `{`); code != http.StatusBadRequest {
		t.Errorf("create bad json: got %d, want 400", code)
	}

	// List
	if code, resp := instReq(t, router, http.MethodGet, "/api/installments", ""); code != http.StatusOK || resp["data"] == nil {
		t.Errorf("list: got %d body %v", code, resp)
	}
	// List com filtro
	if code, _ := instReq(t, router, http.MethodGet, "/api/installments?credit_card_id=card-1&status=ativo", ""); code != http.StatusOK {
		t.Errorf("list filtered: got %d", code)
	}

	// Get + not found
	if code, _ := instReq(t, router, http.MethodGet, "/api/installments/"+gid, ""); code != http.StatusOK {
		t.Errorf("get: got %d", code)
	}
	if code, _ := instReq(t, router, http.MethodGet, "/api/installments/nope", ""); code != http.StatusNotFound {
		t.Errorf("get missing: got %d, want 404", code)
	}

	// UpdateSeries (título/subcategoria)
	if code, _ := instReq(t, router, http.MethodPut, "/api/installments/"+gid, `{"title":"Notebook Dell","subcategory_id":"sub-1"}`); code != http.StatusOK {
		t.Errorf("update series: got %d", code)
	}
	// UpdateSeries — campo imutável → 400
	if code, _ := instReq(t, router, http.MethodPut, "/api/installments/"+gid, `{"title":"x","subcategory_id":"sub-1","total_amount":999}`); code != http.StatusBadRequest {
		t.Errorf("update immutable: got %d, want 400", code)
	}
	// UpdateSeries — bad json
	if code, _ := instReq(t, router, http.MethodPut, "/api/installments/"+gid, `{`); code != http.StatusBadRequest {
		t.Errorf("update bad json: got %d, want 400", code)
	}

	// Cancel remaining
	if code, _ := instReq(t, router, http.MethodPatch, "/api/installments/"+gid+"/cancel-remaining", ""); code != http.StatusOK {
		t.Errorf("cancel-remaining: got %d", code)
	}

	// Delete + not found
	if code, _ := instReq(t, router, http.MethodDelete, "/api/installments/"+gid, ""); code != http.StatusNoContent {
		t.Errorf("delete: got %d, want 204", code)
	}
	if code, _ := instReq(t, router, http.MethodGet, "/api/installments/"+gid, ""); code != http.StatusNotFound {
		t.Errorf("get after delete: got %d, want 404", code)
	}
}

func TestInstallmentRoutes_ErrorPaths(t *testing.T) {
	db := newTestDB(t) // semeia card-1, sub-1 (despesa)
	create := `{"credit_card_id":"card-1","subcategory_id":"sub-1","title":"X","installments_count":3,"input_mode":"by_total","total_amount":10000,"purchase_date":"2026-06-22"}`

	// cartão não vinculável (arquivado) → 409
	r1 := installmentRouterWith(db, &fakeSubs{typ: "despesa"}, &fakeCards{err: domainerr.NewConflict("cartão arquivado")}, &fakeRefs{})
	if c, _ := instReq(t, r1, http.MethodPost, "/api/installments", create); c != http.StatusConflict {
		t.Errorf("card não vinculável: got %d, want 409", c)
	}
	if c, _ := instReq(t, r1, http.MethodPost, "/api/installments/preview", create); c != http.StatusConflict {
		t.Errorf("preview card não vinculável: got %d, want 409", c)
	}

	// subcategoria não-despesa → 409 (ErrOnlyExpensesInstallable)
	r2 := installmentRouterWith(db, &fakeSubs{typ: "receita"}, &fakeCards{}, &fakeRefs{})
	if c, _ := instReq(t, r2, http.MethodPost, "/api/installments", create); c != http.StatusConflict {
		t.Errorf("subcategoria não-despesa: got %d, want 409", c)
	}

	// not found em delete/cancel/update
	r := newInstallmentRouter(db)
	if c, _ := instReq(t, r, http.MethodDelete, "/api/installments/nope", ""); c != http.StatusNotFound {
		t.Errorf("delete missing: got %d, want 404", c)
	}
	if c, _ := instReq(t, r, http.MethodPatch, "/api/installments/nope/cancel-remaining", ""); c != http.StatusNotFound {
		t.Errorf("cancel missing: got %d, want 404", c)
	}
	if c, _ := instReq(t, r, http.MethodPut, "/api/installments/nope", `{"title":"X","subcategory_id":"sub-1"}`); c != http.StatusNotFound {
		t.Errorf("update missing: got %d, want 404", c)
	}
	// list com cada filtro de status (exercita statusHaving)
	for _, st := range []string{"ativo", "quitado", "cancelado"} {
		if c, _ := instReq(t, r, http.MethodGet, "/api/installments?status="+st, ""); c != http.StatusOK {
			t.Errorf("list status=%s: got %d", st, c)
		}
	}
}

func TestInstallmentRepo_DBErrors(t *testing.T) {
	db := newTestDB(t)
	repo := installment.NewSQLiteRepository(db)
	ctx := context.Background()
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "created_at", Order: "DESC"}
	db.Close()

	if err := repo.Create(ctx, installment.InstallmentGroup{ID: "g"}, nil); err == nil {
		t.Error("Create deveria falhar")
	}
	if _, _, err := repo.Get(ctx, "g"); err == nil {
		t.Error("Get deveria falhar")
	}
	if _, _, err := repo.List(ctx, installment.Filter{}, p); err == nil {
		t.Error("List deveria falhar")
	}
	if err := repo.UpdateSeries(ctx, "g", "t", nil, "s", "despesa"); err == nil {
		t.Error("UpdateSeries deveria falhar")
	}
	if _, err := repo.CancelRemaining(ctx, "g"); err == nil {
		t.Error("CancelRemaining deveria falhar")
	}
	if err := repo.Delete(ctx, "g"); err == nil {
		t.Error("Delete deveria falhar")
	}
}
