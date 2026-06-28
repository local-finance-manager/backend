package transaction_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

// fakeFacadeErr faz GetSubcategoryType falhar (subcategoria inexistente).
type fakeFacadeErr struct{ err error }

func (f fakeFacadeErr) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	return "", f.err
}

// newTransactionRouterWith permite injetar facade/checker customizados (ramos de erro).
func newTransactionRouterWith(db *sql.DB, facade transaction.SubcategoryFacade, checker transaction.CreditCardChecker) http.Handler {
	repo := transaction.NewSQLiteRepository(db)
	h := transaction.NewHandler(transaction.HandlerDeps{
		GetTransaction:     transaction.NewGetTransaction(repo),
		ListTransactions:   transaction.NewListTransactions(repo),
		CreateTransaction:  transaction.NewCreateTransaction(repo, facade, checker, nil),
		UpdateTransaction:  transaction.NewUpdateTransaction(repo, facade, checker, nil),
		ConfirmTransaction: transaction.NewConfirmTransaction(repo, nil),
		DeleteTransaction:  transaction.NewDeleteTransaction(repo, nil),
	})
	r := chi.NewRouter()
	r.Route("/api/transactions", transaction.Routes(h))
	return r
}

// ─── Fakes dos ports cross-module ───────────────────────────────────────────

type fakeFacade struct{ typ string }

func (f fakeFacade) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	return f.typ, nil
}

type fakeChecker struct{ err error }

func (f fakeChecker) CheckLinkable(_ context.Context, _ string) error { return f.err }

// newTransactionRouter monta o stack HTTP real sobre :memory: (handler+usecase+sqlite+rotas).
func newTransactionRouter(db *sql.DB) http.Handler {
	repo := transaction.NewSQLiteRepository(db)
	facade := fakeFacade{typ: "despesa"}
	checker := fakeChecker{}
	h := transaction.NewHandler(transaction.HandlerDeps{
		GetTransaction:     transaction.NewGetTransaction(repo),
		ListTransactions:   transaction.NewListTransactions(repo),
		CreateTransaction:  transaction.NewCreateTransaction(repo, facade, checker, nil),
		UpdateTransaction:  transaction.NewUpdateTransaction(repo, facade, checker, nil),
		ConfirmTransaction: transaction.NewConfirmTransaction(repo, nil),
		DeleteTransaction:  transaction.NewDeleteTransaction(repo, nil),
	})
	r := chi.NewRouter()
	r.Route("/api/transactions", transaction.Routes(h))
	return r
}

func httpDo(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
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

func TestTransactionRoutes_ErrorPaths(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	router := newTransactionRouter(db)

	// Lista com TODOS os filtros → exercita parseTransactionFilter por completo.
	q := "/api/transactions?type=despesa&status=pendente&payment_method=pix&subcategory_id=sub-1" +
		"&category_id=cat-1&account_id=a1&competence_date_from=2026-01-01&competence_date_to=2026-12-31" +
		"&payment_date_from=2026-01-01&payment_date_to=2026-12-31&search=alug&credit_card_id=x" +
		"&installment_group_id=g1&order_by=amount&order=asc&page=1&limit=10"
	if code, _ := httpDo(t, router, http.MethodGet, q, ""); code != http.StatusOK {
		t.Errorf("list all filters: got %d", code)
	}

	upd := `{"title":"x","amount":1000,"subcategory_id":"sub-1","payment_method":"pix","status":"pendente","competence_date":"2026-06-01"}`
	if code, _ := httpDo(t, router, http.MethodPut, "/api/transactions/nope", upd); code != http.StatusNotFound {
		t.Errorf("update missing: got %d, want 404", code)
	}
	if code, _ := httpDo(t, router, http.MethodPut, "/api/transactions/x", `{`); code != http.StatusBadRequest {
		t.Errorf("update bad json: got %d, want 400", code)
	}
	if code, _ := httpDo(t, router, http.MethodPatch, "/api/transactions/nope/confirm", `{"payment_date":"2026-06-05"}`); code != http.StatusNotFound {
		t.Errorf("confirm missing: got %d, want 404", code)
	}
	if code, _ := httpDo(t, router, http.MethodPatch, "/api/transactions/x/confirm", `{`); code != http.StatusBadRequest {
		t.Errorf("confirm bad json: got %d, want 400", code)
	}
}

// TestCardReader cobre o adapter produtor (transaction.CardReader) consumido pelo creditcard.
func TestCardReader(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	card := "card-1"
	c1 := mkTransaction("c1", "Compra A", "sub-1", 5000, transaction.TypeDespesa,
		transaction.MethodCartaoCredito, transaction.StatusPendente, "2026-06-10", nil)
	c1.CreditCardID = &card
	c2 := mkTransaction("c2", "Compra B", "sub-1", 3000, transaction.TypeDespesa,
		transaction.MethodCartaoCredito, transaction.StatusPendente, "2026-07-10", nil)
	c2.CreditCardID = &card
	if err := repo.Create(ctx, c1); err != nil {
		t.Fatalf("create c1: %v", err)
	}
	if err := repo.Create(ctx, c2); err != nil {
		t.Fatalf("create c2: %v", err)
	}

	reader := transaction.NewCardReader(db)

	// HasTransactions
	has, err := reader.HasTransactions(ctx, card)
	if err != nil || !has {
		t.Fatalf("HasTransactions: has=%v err=%v", has, err)
	}
	if has, _ := reader.HasTransactions(ctx, "outro"); has {
		t.Error("HasTransactions de cartão inexistente deveria ser false")
	}

	// ListByCard com janela de competência (só junho)
	junho, err := reader.ListByCard(ctx, card, "2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatalf("ListByCard: %v", err)
	}
	if len(junho) != 1 || junho[0].ID != "c1" || junho[0].Amount != 5000 {
		t.Errorf("ListByCard junho inesperado: %+v", junho)
	}
	// janela ampla pega as duas
	todas, _ := reader.ListByCard(ctx, card, "0001-01-01", "9999-12-31")
	if len(todas) != 2 {
		t.Errorf("ListByCard amplo: got %d, want 2", len(todas))
	}
}

func TestTransactionRoutes_FullCRUD(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	router := newTransactionRouter(db)

	// Create
	body := `{"title":"Aluguel","amount":150000,"subcategory_id":"sub-1","payment_method":"pix","status":"pendente","competence_date":"2026-06-01"}`
	code, resp := httpDo(t, router, http.MethodPost, "/api/transactions", body)
	if code != http.StatusCreated {
		t.Fatalf("create: got %d body %v", code, resp)
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create: no id %v", resp)
	}

	// Create — validation error (amount <= 0)
	if code, _ := httpDo(t, router, http.MethodPost, "/api/transactions",
		`{"title":"x","amount":0,"subcategory_id":"sub-1","payment_method":"pix","status":"pendente","competence_date":"2026-06-01"}`); code != http.StatusBadRequest {
		t.Errorf("create invalid: got %d, want 400", code)
	}
	// Create — bad JSON
	if code, _ := httpDo(t, router, http.MethodPost, "/api/transactions", `{`); code != http.StatusBadRequest {
		t.Errorf("create bad json: got %d, want 400", code)
	}

	// List (with summary) + a filter
	if code, resp := httpDo(t, router, http.MethodGet, "/api/transactions?type=despesa&status=pendente", ""); code != http.StatusOK || resp["summary"] == nil {
		t.Errorf("list: got %d body %v", code, resp)
	}

	// Get
	if code, _ := httpDo(t, router, http.MethodGet, "/api/transactions/"+id, ""); code != http.StatusOK {
		t.Errorf("get: got %d", code)
	}
	// Get — not found
	if code, _ := httpDo(t, router, http.MethodGet, "/api/transactions/nope", ""); code != http.StatusNotFound {
		t.Errorf("get missing: got %d, want 404", code)
	}

	// Update
	upd := `{"title":"Aluguel Jun","amount":160000,"subcategory_id":"sub-1","payment_method":"pix","status":"pendente","competence_date":"2026-06-01"}`
	if code, _ := httpDo(t, router, http.MethodPut, "/api/transactions/"+id, upd); code != http.StatusOK {
		t.Errorf("update: got %d", code)
	}

	// Confirm (pendente → realizado)
	if code, _ := httpDo(t, router, http.MethodPatch, "/api/transactions/"+id+"/confirm", `{"payment_date":"2026-06-05"}`); code != http.StatusOK {
		t.Errorf("confirm: got %d", code)
	}

	// Delete
	if code, _ := httpDo(t, router, http.MethodDelete, "/api/transactions/"+id, ""); code != http.StatusNoContent {
		t.Errorf("delete: got %d, want 204", code)
	}
	// Delete — not found
	if code, _ := httpDo(t, router, http.MethodDelete, "/api/transactions/nope", ""); code != http.StatusNotFound {
		t.Errorf("delete missing: got %d, want 404", code)
	}
}

// TestTransactionRoutes_CreateErrors cobre os ramos de erro do CreateTransaction:
// checker de cartão falho e facade (subcategoria) falho.
func TestTransactionRoutes_CreateErrors(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")

	// CheckLinkable falha (cartão arquivado) ao criar lançamento de cartão
	r1 := newTransactionRouterWith(db, fakeFacade{typ: "despesa"}, fakeChecker{err: domainerr.NewConflict("cartão arquivado")})
	cardBody := `{"title":"Compra","amount":5000,"subcategory_id":"sub-1","payment_method":"cartao_credito","status":"pendente","competence_date":"2026-06-01","credit_card_id":"card-1"}`
	if code, _ := httpDo(t, r1, http.MethodPost, "/api/transactions", cardBody); code != http.StatusConflict {
		t.Errorf("create card checker error: got %d, want 409", code)
	}

	// GetSubcategoryType falha (subcategoria inexistente)
	r2 := newTransactionRouterWith(db, fakeFacadeErr{err: domainerr.NewNotFound("subcategoria não encontrada")}, fakeChecker{})
	body := `{"title":"X","amount":1000,"subcategory_id":"sub-1","payment_method":"pix","status":"pendente","competence_date":"2026-06-01"}`
	if code, _ := httpDo(t, r2, http.MethodPost, "/api/transactions", body); code != http.StatusNotFound {
		t.Errorf("create facade error: got %d, want 404", code)
	}
}

// TestTransactionRepo_FullFieldsRoundTrip cobre os ramos nullable do scanDetail
// (description, payment_date, account_id, credit_card_id) e do scanCardTxn (installment_*).
func TestTransactionRepo_FullFieldsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()

	// repo.Create persiste description/payment_date/account_id/credit_card_id (scanDetail)
	txn := mkTransaction("full", "Compra", "sub-1", 5000, transaction.TypeDespesa,
		transaction.MethodCartaoCredito, transaction.StatusRealizado, "2026-06-10", strPtr("2026-06-15"))
	desc := "descrição completa"
	card := "card-1"
	txn.Description = &desc
	txn.AccountID = strPtr("acc-1")
	txn.CreditCardID = &card
	if err := repo.Create(ctx, txn); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(ctx, "full")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description == nil || got.PaymentDate == nil || got.AccountID == nil || got.CreditCardID == nil {
		t.Errorf("campos nullable do scanDetail não retornados: %+v", got)
	}

	// As colunas de parcelamento são escritas pelo módulo installment (Opção A); aqui
	// inserimos uma parcela direto para cobrir os ramos nullable do scanCardTxn.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO transactions
		(id,title,amount,type,subcategory_id,payment_method,status,competence_date,payment_date,
		 credit_card_id,installment_group_id,installment_number,installment_total,created_at,updated_at)
		VALUES ('parc','Parcela 1/3',3334,'despesa','sub-1','cartao_credito','pendente','2026-06-12','2026-06-12',
		 'card-1','grp-1',1,3,?,?)`, now, now)
	if err != nil {
		t.Fatalf("insert parcela: %v", err)
	}
	cards, err := transaction.NewCardReader(db).ListByCard(ctx, "card-1", "0001-01-01", "9999-12-31")
	if err != nil {
		t.Fatalf("ListByCard: %v", err)
	}
	var parc *shared.CardTransaction
	for i := range cards {
		if cards[i].ID == "parc" {
			parc = &cards[i]
		}
	}
	if parc == nil || parc.InstallmentGroupID == nil || parc.InstallmentNumber == nil ||
		parc.InstallmentTotal == nil || parc.PaymentDate == nil {
		t.Errorf("scanCardTxn não preencheu os campos de parcelamento: %+v", parc)
	}
}

func TestTransactionRepo_DBErrors(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	reader := transaction.NewCardReader(db)
	ctx := context.Background()
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "DESC"}
	txn := mkTransaction("x", "X", "sub-1", 1000, transaction.TypeDespesa,
		transaction.MethodPix, transaction.StatusPendente, "2026-06-01", nil)
	db.Close()

	if err := repo.Create(ctx, txn); err == nil {
		t.Error("Create deveria falhar")
	}
	if _, err := repo.Get(ctx, "x"); err == nil {
		t.Error("Get deveria falhar")
	}
	if _, err := repo.List(ctx, transaction.TransactionFilter{}, p); err == nil {
		t.Error("List deveria falhar")
	}
	if err := repo.Update(ctx, txn); err == nil {
		t.Error("Update deveria falhar")
	}
	if err := repo.Delete(ctx, "x"); err == nil {
		t.Error("Delete deveria falhar")
	}
	if _, err := repo.GetSummary(ctx, transaction.TransactionFilter{}); err == nil {
		t.Error("GetSummary deveria falhar")
	}
	if _, err := reader.ListByCard(ctx, "card-1", "a", "b"); err == nil {
		t.Error("ListByCard deveria falhar")
	}
	if _, err := reader.HasTransactions(ctx, "card-1"); err == nil {
		t.Error("HasTransactions deveria falhar")
	}
}
