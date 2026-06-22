package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Compile-time interface checks ────────────────────────────────────────────

var (
	_ GetTransactionUseCase     = (*getTransactionImpl)(nil)
	_ ListTransactionsUseCase   = (*listTransactionsImpl)(nil)
	_ CreateTransactionUseCase  = (*createTransactionImpl)(nil)
	_ UpdateTransactionUseCase  = (*updateTransactionImpl)(nil)
	_ ConfirmTransactionUseCase = (*confirmTransactionImpl)(nil)
	_ DeleteTransactionUseCase  = (*deleteTransactionImpl)(nil)
)

// ─── Fake repository ──────────────────────────────────────────────────────────

type fakeTransactionRepo struct {
	data     map[string]Transaction
	details  map[string]TransactionDetail
	forceErr error
}

func newFakeRepo() *fakeTransactionRepo {
	return &fakeTransactionRepo{
		data:    make(map[string]Transaction),
		details: make(map[string]TransactionDetail),
	}
}

func (f *fakeTransactionRepo) Get(_ context.Context, id string) (TransactionDetail, error) {
	if f.forceErr != nil {
		return TransactionDetail{}, f.forceErr
	}
	d, ok := f.details[id]
	if !ok {
		return TransactionDetail{}, ErrTransactionNotFound
	}
	return d, nil
}

func (f *fakeTransactionRepo) List(_ context.Context, _ TransactionFilter, _ shared.Pagination) ([]TransactionDetail, error) {
	if f.forceErr != nil {
		return nil, f.forceErr
	}
	out := []TransactionDetail{}
	for _, d := range f.details {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeTransactionRepo) GetSummary(_ context.Context, _ TransactionFilter) (Summary, error) {
	if f.forceErr != nil {
		return Summary{}, f.forceErr
	}
	return Summary{CountTotal: len(f.details)}, nil
}

func (f *fakeTransactionRepo) Create(_ context.Context, t Transaction) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[t.ID] = t
	f.details[t.ID] = TransactionDetail{Transaction: t}
	return nil
}

func (f *fakeTransactionRepo) Update(_ context.Context, t Transaction) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	if _, ok := f.data[t.ID]; !ok {
		return ErrTransactionNotFound
	}
	f.data[t.ID] = t
	f.details[t.ID] = TransactionDetail{Transaction: t}
	return nil
}

func (f *fakeTransactionRepo) Delete(_ context.Context, id string) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	delete(f.data, id)
	delete(f.details, id)
	return nil
}

// ─── Fake facade ──────────────────────────────────────────────────────────────

type fakeSubFacade struct {
	typ      string
	notFound bool
}

func (f *fakeSubFacade) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	if f.notFound {
		return "", ErrTransactionNotFound
	}
	return f.typ, nil
}

// fakeCardChecker é o stub do CreditCardChecker injetado em create/update.
type fakeCardChecker struct{ err error }

func (f *fakeCardChecker) CheckLinkable(_ context.Context, _ string) error { return f.err }

// okChecker é um CreditCardChecker que sempre aprova o vínculo.
func okChecker() CreditCardChecker { return &fakeCardChecker{} }

// ─── helpers ──────────────────────────────────────────────────────────────────

func seedDetail(repo *fakeTransactionRepo, id string, status TransactionStatus) {
	t := Transaction{
		ID:             id,
		Title:          "Test",
		Amount:         1000,
		Type:           TypeDespesa,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodPix,
		Status:         status,
		CompetenceDate: "2026-01-01",
	}
	repo.data[id] = t
	repo.details[id] = TransactionDetail{Transaction: t}
}

// ─── GetTransaction ───────────────────────────────────────────────────────────

func TestGetTransaction_Found(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-1", StatusPendente)
	uc := NewGetTransaction(repo)

	got, err := uc.Execute(context.Background(), "txn-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "txn-1" {
		t.Errorf("id: got %q, want txn-1", got.ID)
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	uc := NewGetTransaction(newFakeRepo())
	_, err := uc.Execute(context.Background(), "missing")
	if err == nil {
		t.Error("expected not-found error")
	}
}

// ─── ListTransactions ─────────────────────────────────────────────────────────

func TestListTransactions_ReturnsSummary(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-1", StatusPendente)
	seedDetail(repo, "txn-2", StatusRealizado)
	uc := NewListTransactions(repo)

	result, err := uc.Execute(context.Background(), ListTransactionsInput{
		Pagination: shared.Pagination{Page: 1, Limit: 10, OrderBy: "competence_date", Order: "ASC"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pagination.Total != 2 {
		t.Errorf("total: got %d, want 2", result.Pagination.Total)
	}
	if len(result.Data) != 2 {
		t.Errorf("data len: got %d, want 2", len(result.Data))
	}
}

func TestListTransactions_RepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.forceErr = errors.New("db error")
	uc := NewListTransactions(repo)

	_, err := uc.Execute(context.Background(), ListTransactionsInput{
		Pagination: shared.DefaultPagination(),
	})
	if err == nil {
		t.Error("expected error when repo fails")
	}
}

// ─── CreateTransaction ────────────────────────────────────────────────────────

func TestCreateTransaction_Success(t *testing.T) {
	repo := newFakeRepo()
	facade := &fakeSubFacade{typ: "despesa"}
	uc := NewCreateTransaction(repo, facade, okChecker())

	got, err := uc.Execute(context.Background(), CreateTransactionInput{
		Title:          "Aluguel",
		Amount:         150000,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodPix,
		Status:         StatusPendente,
		CompetenceDate: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.Type != TypeDespesa {
		t.Errorf("type derived: got %q, want despesa", got.Type)
	}
}

func TestCreateTransaction_ValidationError(t *testing.T) {
	repo := newFakeRepo()
	facade := &fakeSubFacade{typ: "despesa"}
	uc := NewCreateTransaction(repo, facade, okChecker())

	_, err := uc.Execute(context.Background(), CreateTransactionInput{
		Title: "", Amount: 0, SubcategoryID: "sub-1",
		PaymentMethod: MethodPix, Status: StatusPendente, CompetenceDate: "2026-01-01",
	})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestCreateTransaction_SubcategoryNotFound(t *testing.T) {
	repo := newFakeRepo()
	facade := &fakeSubFacade{notFound: true}
	uc := NewCreateTransaction(repo, facade, okChecker())

	_, err := uc.Execute(context.Background(), CreateTransactionInput{
		Title:          "Aluguel",
		Amount:         100,
		SubcategoryID:  "missing",
		PaymentMethod:  MethodPix,
		Status:         StatusPendente,
		CompetenceDate: "2026-01-01",
	})
	if err == nil {
		t.Error("expected error when subcategory not found")
	}
}

func TestCreateTransaction_CardCheckerError(t *testing.T) {
	repo := newFakeRepo()
	facade := &fakeSubFacade{typ: "despesa"}
	checker := &fakeCardChecker{err: errors.New("cartão arquivado")}
	uc := NewCreateTransaction(repo, facade, checker)

	cardID := "card-1"
	_, err := uc.Execute(context.Background(), CreateTransactionInput{
		Title:          "Compra",
		Amount:         100,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodCartaoCredito,
		Status:         StatusPendente,
		CompetenceDate: "2026-01-01",
		CreditCardID:   &cardID,
	})
	if err == nil {
		t.Error("expected error propagated from card checker")
	}
}

// ─── UpdateTransaction ────────────────────────────────────────────────────────

func TestUpdateTransaction_Success(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-upd", StatusPendente)
	facade := &fakeSubFacade{typ: "despesa"}
	uc := NewUpdateTransaction(repo, facade, okChecker())

	got, err := uc.Execute(context.Background(), UpdateTransactionInput{
		ID:             "txn-upd",
		Title:          "Atualizado",
		Amount:         2000,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodBoleto,
		Status:         StatusRealizado,
		CompetenceDate: "2026-01-01",
		PaymentDate:    func() *string { s := "2026-01-10"; return &s }(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Atualizado" {
		t.Errorf("title: got %q, want Atualizado", got.Title)
	}
	if got.Status != StatusRealizado {
		t.Errorf("status: got %q, want realizado", got.Status)
	}
}

func TestUpdateTransaction_InvalidTransition(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-can", StatusCancelado)
	facade := &fakeSubFacade{typ: "despesa"}
	uc := NewUpdateTransaction(repo, facade, okChecker())

	// cancelado → realizado is prohibited
	pd := "2026-01-10"
	_, err := uc.Execute(context.Background(), UpdateTransactionInput{
		ID:             "txn-can",
		Title:          "X",
		Amount:         100,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodPix,
		Status:         StatusRealizado,
		CompetenceDate: "2026-01-01",
		PaymentDate:    &pd,
	})
	if err == nil {
		t.Error("expected invalid transition error: cancelado → realizado")
	}
}

func TestUpdateTransaction_NotFound(t *testing.T) {
	facade := &fakeSubFacade{typ: "despesa"}
	uc := NewUpdateTransaction(newFakeRepo(), facade, okChecker())

	_, err := uc.Execute(context.Background(), UpdateTransactionInput{
		ID:             "missing",
		Title:          "X",
		Amount:         100,
		SubcategoryID:  "sub-1",
		PaymentMethod:  MethodPix,
		Status:         StatusPendente,
		CompetenceDate: "2026-01-01",
	})
	if err == nil {
		t.Error("expected not-found error")
	}
}

// ─── ConfirmTransaction ───────────────────────────────────────────────────────

func TestConfirmTransaction_Success(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-conf", StatusPendente)
	uc := NewConfirmTransaction(repo)

	got, err := uc.Execute(context.Background(), ConfirmTransactionInput{
		ID:          "txn-conf",
		PaymentDate: "2026-01-10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != StatusRealizado {
		t.Errorf("status: got %q, want realizado", got.Status)
	}
}

func TestConfirmTransaction_InvalidTransition(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-can", StatusCancelado)
	uc := NewConfirmTransaction(repo)

	_, err := uc.Execute(context.Background(), ConfirmTransactionInput{
		ID:          "txn-can",
		PaymentDate: "2026-01-10",
	})
	if err == nil {
		t.Error("expected invalid transition: cancelado → realizado")
	}
}

func TestConfirmTransaction_ValidationError(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-1", StatusPendente)
	uc := NewConfirmTransaction(repo)

	_, err := uc.Execute(context.Background(), ConfirmTransactionInput{
		ID:          "txn-1",
		PaymentDate: "",
	})
	if err == nil {
		t.Error("expected validation error for empty paymentDate")
	}
}

// ─── DeleteTransaction ────────────────────────────────────────────────────────

func TestDeleteTransaction_Success(t *testing.T) {
	repo := newFakeRepo()
	seedDetail(repo, "txn-del", StatusPendente)
	uc := NewDeleteTransaction(repo)

	if err := uc.Execute(context.Background(), "txn-del"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := repo.data["txn-del"]; ok {
		t.Error("expected transaction to be removed")
	}
}

func TestDeleteTransaction_NotFound(t *testing.T) {
	uc := NewDeleteTransaction(newFakeRepo())
	err := uc.Execute(context.Background(), "missing")
	if err == nil {
		t.Error("expected not-found error")
	}
}
