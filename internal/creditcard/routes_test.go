package creditcard_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/creditcard"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Fakes mínimos dos ports cross-module ───────────────────────────────────

type ccFakeReader struct{ txns []shared.CardTransaction }

func (f ccFakeReader) ListByCard(_ context.Context, cardID, from, to string) ([]shared.CardTransaction, error) {
	out := []shared.CardTransaction{}
	for _, t := range f.txns {
		if t.CreditCardID == cardID && t.CompetenceDate >= from && t.CompetenceDate <= to {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f ccFakeReader) HasTransactions(_ context.Context, cardID string) (bool, error) {
	for _, t := range f.txns {
		if t.CreditCardID == cardID {
			return true, nil
		}
	}
	return false, nil
}

type ccFakeSubs struct{}

func (ccFakeSubs) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	return "transferencia", nil
}

func newCCRouter(db *sql.DB, reader creditcard.CardTransactionReader) http.Handler {
	ccRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	h := creditcard.NewHandler(creditcard.HandlerDeps{
		Create:       creditcard.NewCreateCreditCard(ccRepo),
		Get:          creditcard.NewGetCreditCard(ccRepo, payRepo, reader),
		List:         creditcard.NewListCreditCards(ccRepo, payRepo, reader),
		Update:       creditcard.NewUpdateCreditCard(ccRepo),
		Delete:       creditcard.NewDeleteCreditCard(ccRepo, reader),
		Archive:      creditcard.NewArchiveCreditCard(ccRepo),
		ListInvoices: creditcard.NewListInvoices(ccRepo, payRepo, reader),
		GetInvoice:   creditcard.NewGetInvoice(ccRepo, payRepo, reader),
		PayInvoice:   creditcard.NewPayInvoice(ccRepo, payRepo, reader, ccFakeSubs{}),
		UndoPayment:  creditcard.NewUndoInvoicePayment(ccRepo, payRepo, reader),
		MonthSummary: creditcard.NewMonthlyCardSummary(ccRepo, reader),
	})
	r := chi.NewRouter()
	r.Route("/api/credit-cards", creditcard.Routes(h))
	return r
}

func ccReq(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
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

func TestCreditCardRoutes_FullFlow(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	ccRepo := creditcard.NewSQLiteCreditCardRepository(db)
	ccRepo.Create(ctx, mkCard("c1", "Nubank", false))

	// duas compras pendentes no ciclo 2026-02 (competência 2026-01-15, closing 3 → vencida)
	insertCompra(t, db, "compra-1", "pendente", 20000)
	insertCompra(t, db, "compra-2", "pendente", 10000)
	// ajusta competência das compras para o ciclo passado
	db.Exec("UPDATE transactions SET competence_date='2026-01-15' WHERE id IN ('compra-1','compra-2')")

	reader := ccFakeReader{txns: []shared.CardTransaction{
		{ID: "compra-1", Amount: 20000, CompetenceDate: "2026-01-15", Status: "pendente", CreditCardID: "c1",
			CategoryID: "cat", CategoryName: "Cat", CategoryColor: "#fff", SubcategoryID: "sub", SubcategoryName: "Sub"},
		{ID: "compra-2", Amount: 10000, CompetenceDate: "2026-01-15", Status: "pendente", CreditCardID: "c1",
			CategoryID: "cat", CategoryName: "Cat", CategoryColor: "#fff", SubcategoryID: "sub", SubcategoryName: "Sub"},
	}}
	router := newCCRouter(db, reader)

	// List
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards", ""); code != http.StatusOK {
		t.Errorf("list: got %d", code)
	}
	// Create + validations
	code, body := ccReq(t, router, http.MethodPost, "/api/credit-cards",
		`{"name":"Inter","brand":"mastercard","credit_limit":100000,"closing_day":5,"due_day":15}`)
	if code != http.StatusCreated {
		t.Fatalf("create: got %d body %v", code, body)
	}
	newID, _ := body["id"].(string)
	if code, _ := ccReq(t, router, http.MethodPost, "/api/credit-cards", `{"name":"","brand":"x","credit_limit":0,"closing_day":0,"due_day":0}`); code != http.StatusBadRequest {
		t.Errorf("create invalid: got %d, want 400", code)
	}
	if code, _ := ccReq(t, router, http.MethodPost, "/api/credit-cards", `{`); code != http.StatusBadRequest {
		t.Errorf("create bad json: got %d, want 400", code)
	}

	// Get + not found
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1", ""); code != http.StatusOK {
		t.Errorf("get: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/nope", ""); code != http.StatusNotFound {
		t.Errorf("get missing: got %d, want 404", code)
	}

	// Update
	if code, _ := ccReq(t, router, http.MethodPut, "/api/credit-cards/c1",
		`{"name":"Nubank Roxinho","brand":"mastercard","credit_limit":600000,"closing_day":3,"due_day":10}`); code != http.StatusOK {
		t.Errorf("update: got %d", code)
	}

	// Archive + unarchive
	if code, _ := ccReq(t, router, http.MethodPatch, "/api/credit-cards/c1/archive", ""); code != http.StatusNoContent {
		t.Errorf("archive: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodPatch, "/api/credit-cards/c1/unarchive", ""); code != http.StatusNoContent {
		t.Errorf("unarchive: got %d", code)
	}

	// Summary + validation
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/summary?year=2026&month=1", ""); code != http.StatusOK {
		t.Errorf("summary: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/summary?year=2026", ""); code != http.StatusBadRequest {
		t.Errorf("summary missing month: got %d, want 400", code)
	}
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/summary?month=1", ""); code != http.StatusBadRequest {
		t.Errorf("summary missing year: got %d, want 400", code)
	}

	// Invoices list + get
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/invoices", ""); code != http.StatusOK {
		t.Errorf("invoices: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/invoices/2026-02", ""); code != http.StatusOK {
		t.Errorf("get invoice: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodGet, "/api/credit-cards/c1/invoices/2099-01", ""); code != http.StatusNotFound {
		t.Errorf("get invoice missing: got %d, want 404", code)
	}

	// Pay + undo
	if code, resp := ccReq(t, router, http.MethodPatch, "/api/credit-cards/c1/invoices/2026-02/pay",
		`{"payment_date":"2026-06-20","subcategory_id":"sub-trf-pgto"}`); code != http.StatusOK || resp["status"] != "paga" {
		t.Errorf("pay: got %d body %v", code, resp)
	}
	if code, _ := ccReq(t, router, http.MethodDelete, "/api/credit-cards/c1/invoices/2026-02/pay", ""); code != http.StatusOK {
		t.Errorf("undo: got %d", code)
	}

	// Delete: c1 tem lançamentos → 409; o cartão recém-criado (sem lançamentos) → 204
	if code, _ := ccReq(t, router, http.MethodDelete, "/api/credit-cards/c1", ""); code != http.StatusConflict {
		t.Errorf("delete card with txns: got %d, want 409", code)
	}
	if code, _ := ccReq(t, router, http.MethodDelete, "/api/credit-cards/"+newID, ""); code != http.StatusNoContent {
		t.Errorf("delete empty card: got %d, want 204", code)
	}
}

func TestCreditCardRoutes_ErrorPaths(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	creditcard.NewSQLiteCreditCardRepository(db).Create(ctx, mkCard("c1", "Nubank", false))
	router := newCCRouter(db, ccFakeReader{})

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"create bad json", http.MethodPost, "/api/credit-cards", `{`, http.StatusBadRequest},
		{"update bad json", http.MethodPut, "/api/credit-cards/c1", `{`, http.StatusBadRequest},
		{"update missing", http.MethodPut, "/api/credit-cards/nope", `{"name":"X","brand":"visa","credit_limit":1,"closing_day":1,"due_day":2}`, http.StatusNotFound},
		{"get missing", http.MethodGet, "/api/credit-cards/nope", "", http.StatusNotFound},
		{"archive missing", http.MethodPatch, "/api/credit-cards/nope/archive", "", http.StatusNotFound},
		{"unarchive missing", http.MethodPatch, "/api/credit-cards/nope/unarchive", "", http.StatusNotFound},
		{"summary missing card", http.MethodGet, "/api/credit-cards/nope/summary?year=2026&month=1", "", http.StatusNotFound},
		{"invoices missing card", http.MethodGet, "/api/credit-cards/nope/invoices", "", http.StatusNotFound},
		{"get invoice missing card", http.MethodGet, "/api/credit-cards/nope/invoices/2026-01", "", http.StatusNotFound},
		{"pay bad json", http.MethodPatch, "/api/credit-cards/c1/invoices/2026-02/pay", `{`, http.StatusBadRequest},
		{"pay missing subcategory", http.MethodPatch, "/api/credit-cards/c1/invoices/2026-02/pay", `{"payment_date":"2026-06-20"}`, http.StatusBadRequest},
		{"pay nonexistent invoice", http.MethodPatch, "/api/credit-cards/c1/invoices/2099-01/pay", `{"payment_date":"2026-06-20","subcategory_id":"s"}`, http.StatusNotFound},
		{"undo without payment", http.MethodDelete, "/api/credit-cards/c1/invoices/2099-01/pay", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		if code, _ := ccReq(t, router, tc.method, tc.path, tc.body); code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, code, tc.want)
		}
	}

	// Archive/unarchive de cartão existente (sucesso) — cobre os ramos felizes.
	if code, _ := ccReq(t, router, http.MethodPatch, "/api/credit-cards/c1/archive", ""); code != http.StatusNoContent {
		t.Errorf("archive c1: got %d", code)
	}
	if code, _ := ccReq(t, router, http.MethodPatch, "/api/credit-cards/c1/unarchive", ""); code != http.StatusNoContent {
		t.Errorf("unarchive c1: got %d", code)
	}
}

// TestCreditCardRepo_DBErrors cobre os ramos de erro de I/O (DB fechado).
func TestCreditCardRepo_DBErrors(t *testing.T) {
	db := newTestDB(t)
	ccRepo := creditcard.NewSQLiteCreditCardRepository(db)
	payRepo := creditcard.NewSQLiteInvoicePaymentRepository(db)
	ctx := context.Background()
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "created_at", Order: "DESC"}
	db.Close()

	if err := ccRepo.Create(ctx, mkCard("c", "C", false)); err == nil {
		t.Error("Create deveria falhar")
	}
	if _, err := ccRepo.Get(ctx, "c"); err == nil {
		t.Error("Get deveria falhar")
	}
	if _, _, err := ccRepo.List(ctx, false, p); err == nil {
		t.Error("List deveria falhar")
	}
	if err := ccRepo.Update(ctx, mkCard("c", "C", false)); err == nil {
		t.Error("Update deveria falhar")
	}
	if err := ccRepo.SetArchived(ctx, "c", true); err == nil {
		t.Error("SetArchived deveria falhar")
	}
	if err := ccRepo.Delete(ctx, "c"); err == nil {
		t.Error("Delete deveria falhar")
	}
	if _, err := payRepo.Get(ctx, "c", "2026-01"); err == nil {
		t.Error("payment Get deveria falhar")
	}
	if _, err := payRepo.ListByCard(ctx, "c"); err == nil {
		t.Error("payment ListByCard deveria falhar")
	}
	if err := payRepo.PayInvoiceAtomic(ctx, creditcard.AtomicPayInput{CardID: "c", Reference: "2026-01", Payment: mkPaymentTxn("p")}); err == nil {
		t.Error("PayInvoiceAtomic deveria falhar")
	}
	if err := payRepo.UndoPaymentAtomic(ctx, creditcard.AtomicUndoInput{CardID: "c", Reference: "2026-01"}); err == nil {
		t.Error("UndoPaymentAtomic deveria falhar")
	}
}

// TestInvoiceReferenceFacade cobre o facade produtor consumido pelo installment.
func TestInvoiceReferenceFacade(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	ccRepo := creditcard.NewSQLiteCreditCardRepository(db)
	ccRepo.Create(ctx, mkCard("c1", "Nubank", false))

	facade := creditcard.NewInvoiceReferenceFacade(ccRepo)
	refs, err := facade.ReferencesFor(ctx, "c1", []string{"2026-01-15", "2026-02-20"})
	if err != nil {
		t.Fatalf("ReferencesFor: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs: got %d, want 2", len(refs))
	}
	// cartão inexistente → erro
	if _, err := facade.ReferencesFor(ctx, "nope", []string{"2026-01-15"}); err == nil {
		t.Error("esperava erro para cartão inexistente")
	}
}
