package transaction_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/transaction"
)

type fakeGuard struct {
	ensureErr error
	afterErr  error
}

func (g fakeGuard) EnsureEditable(_ context.Context, _ string) error { return g.ensureErr }
func (g fakeGuard) AfterChange(_ context.Context, _ ...string) error { return g.afterErr }

func seedTxn(t *testing.T, repo *transaction.SQLiteRepository, id, sub string, st transaction.TransactionStatus) {
	t.Helper()
	var pay *string
	if st == transaction.StatusRealizado {
		pay = strPtr("2026-01-15")
	}
	if err := repo.Create(context.Background(), mkTransaction(id, id, sub, 10000, transaction.TypeDespesa, transaction.MethodPix, st, "2026-01-15", pay)); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// O guard bloqueia mês fechado em todos os casos de uso de escrita.
func TestUseCases_GuardEnsureBlocks(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()
	seedTxn(t, repo, "t1", "sub-1", transaction.StatusPendente)

	blocked := fakeGuard{ensureErr: domainerr.NewConflict("mês fechado")}
	facade := fakeFacade{typ: "despesa"}
	checker := fakeChecker{}

	if _, err := transaction.NewCreateTransaction(repo, facade, checker, blocked).Execute(ctx, transaction.CreateTransactionInput{
		Title: "X", Amount: 1000, SubcategoryID: "sub-1", PaymentMethod: transaction.MethodPix, Status: transaction.StatusPendente, CompetenceDate: "2026-01-10",
	}); err == nil {
		t.Error("create deveria ser bloqueado pelo guard")
	}
	if _, err := transaction.NewUpdateTransaction(repo, facade, checker, blocked).Execute(ctx, transaction.UpdateTransactionInput{
		ID: "t1", Title: "X", Amount: 1000, SubcategoryID: "sub-1", PaymentMethod: transaction.MethodPix, Status: transaction.StatusPendente, CompetenceDate: "2026-01-15",
	}); err == nil {
		t.Error("update deveria ser bloqueado pelo guard")
	}
	if _, err := transaction.NewConfirmTransaction(repo, blocked).Execute(ctx, transaction.ConfirmTransactionInput{ID: "t1", PaymentDate: "2026-01-16"}); err == nil {
		t.Error("confirm deveria ser bloqueado pelo guard")
	}
	if _, err := transaction.NewCancelTransaction(repo, blocked).Execute(ctx, "t1"); err == nil {
		t.Error("cancel deveria ser bloqueado pelo guard")
	}
	if err := transaction.NewDeleteTransaction(repo, blocked).Execute(ctx, "t1"); err == nil {
		t.Error("delete deveria ser bloqueado pelo guard")
	}
}

// AfterChange com erro propaga após a escrita (recálculo de snapshot falhou).
func TestUseCases_GuardAfterChangePropagates(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()
	seedTxn(t, repo, "t1", "sub-1", transaction.StatusPendente)

	g := fakeGuard{afterErr: errors.New("recalc falhou")}
	if _, err := transaction.NewCancelTransaction(repo, g).Execute(ctx, "t1"); err == nil {
		t.Error("cancel deveria propagar erro de AfterChange")
	}
	seedTxn(t, repo, "t2", "sub-1", transaction.StatusPendente)
	if err := transaction.NewDeleteTransaction(repo, g).Execute(ctx, "t2"); err == nil {
		t.Error("delete deveria propagar erro de AfterChange")
	}
}

// Cancel: caminho feliz (idempotente) e transição inválida.
func TestCancelUseCase_HappyAndIdempotent(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	ctx := context.Background()
	seedTxn(t, repo, "t1", "sub-1", transaction.StatusPendente)

	uc := transaction.NewCancelTransaction(repo, nil)
	d, err := uc.Execute(ctx, "t1")
	if err != nil || d.Status != transaction.StatusCancelado {
		t.Fatalf("cancel: %v status=%v", err, d.Status)
	}
	// já cancelado → idempotente
	if d2, err := uc.Execute(ctx, "t1"); err != nil || d2.Status != transaction.StatusCancelado {
		t.Errorf("cancel idempotente: %v %v", err, d2.Status)
	}
	// inexistente → erro
	if _, err := uc.Execute(ctx, "nope"); err == nil {
		t.Error("cancel inexistente deveria falhar")
	}
}

// Handler CancelTransaction via HTTP (PATCH /cancel).
func TestTransactionRoutes_Cancel(t *testing.T) {
	db := newTestDB(t)
	insertTestSub(t, db, "cat-1", "Moradia", "despesa", "sub-1", "Aluguel")
	repo := transaction.NewSQLiteRepository(db)
	seedTxn(t, repo, "t1", "sub-1", transaction.StatusPendente)

	h := transaction.NewHandler(transaction.HandlerDeps{
		GetTransaction:     transaction.NewGetTransaction(repo),
		ListTransactions:   transaction.NewListTransactions(repo),
		CreateTransaction:  transaction.NewCreateTransaction(repo, fakeFacade{typ: "despesa"}, fakeChecker{}, nil),
		UpdateTransaction:  transaction.NewUpdateTransaction(repo, fakeFacade{typ: "despesa"}, fakeChecker{}, nil),
		ConfirmTransaction: transaction.NewConfirmTransaction(repo, nil),
		CancelTransaction:  transaction.NewCancelTransaction(repo, nil),
		DeleteTransaction:  transaction.NewDeleteTransaction(repo, nil),
	})
	r := chi.NewRouter()
	r.Route("/api/transactions", transaction.Routes(h))

	if code, body := httpDo(t, r, http.MethodPatch, "/api/transactions/t1/cancel", ""); code != http.StatusOK || body["status"] != "cancelado" {
		t.Errorf("cancel: %d %v", code, body)
	}
	if code, _ := httpDo(t, r, http.MethodPatch, "/api/transactions/nope/cancel", ""); code != http.StatusNotFound {
		t.Errorf("cancel inexistente: %d want 404", code)
	}
}
