package installment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/local-finance-manager/backend/internal/installment"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeRepo struct {
	groups   map[string]installment.InstallmentGroup
	parcelas map[string][]installment.Installment
	created  bool
	forceErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{groups: map[string]installment.InstallmentGroup{}, parcelas: map[string][]installment.Installment{}}
}

func (f *fakeRepo) Create(_ context.Context, g installment.InstallmentGroup, parcelas []installment.Parcela) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.created = true
	f.groups[g.ID] = g
	ins := make([]installment.Installment, len(parcelas))
	for i, p := range parcelas {
		ins[i] = installment.Installment{TransactionID: p.ID, Number: p.Number, Amount: p.Amount, CompetenceDate: p.CompetenceDate, Status: "pendente"}
	}
	f.parcelas[g.ID] = ins
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (installment.InstallmentGroup, []installment.Installment, error) {
	g, ok := f.groups[id]
	if !ok {
		return installment.InstallmentGroup{}, nil, installment.ErrInstallmentGroupNotFound
	}
	return g, f.parcelas[id], nil
}

func (f *fakeRepo) List(_ context.Context, _ installment.Filter, p shared.Pagination) ([]installment.GroupSummary, int, error) {
	if f.forceErr != nil {
		return nil, 0, f.forceErr
	}
	out := []installment.GroupSummary{}
	for id, g := range f.groups {
		out = append(out, installment.GroupSummary{Group: g, RemainingCount: len(f.parcelas[id]), Status: installment.GroupActive})
	}
	return out, len(out), nil
}

func (f *fakeRepo) UpdateSeries(_ context.Context, id, title string, description *string, subcategoryID, _ string) error {
	g, ok := f.groups[id]
	if !ok {
		return installment.ErrInstallmentGroupNotFound
	}
	g.Title = title
	g.Description = description
	g.SubcategoryID = subcategoryID
	f.groups[id] = g
	return nil
}

func (f *fakeRepo) CancelRemaining(_ context.Context, id string) (int, error) {
	n := 0
	for i, p := range f.parcelas[id] {
		if p.Status == "pendente" {
			f.parcelas[id][i].Status = "cancelado"
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.groups[id]; !ok {
		return installment.ErrInstallmentGroupNotFound
	}
	delete(f.groups, id)
	delete(f.parcelas, id)
	return nil
}

type fakeSubs struct {
	typ string
	err error
}

func (f *fakeSubs) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	return f.typ, f.err
}

type fakeCards struct{ err error }

func (f *fakeCards) CheckLinkable(_ context.Context, _ string) error { return f.err }

type fakeRefs struct{ err error }

func (f *fakeRefs) ReferencesFor(_ context.Context, _ string, dates []string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]string, len(dates))
	for i, d := range dates {
		out[i] = d[:7] // YYYY-MM (aproximação suficiente p/ teste)
	}
	return out, nil
}

func newSvc(repo installment.Repository, subs installment.SubcategoryReader, cards installment.CreditCardChecker, refs installment.InvoiceReferenceResolver) *installment.Service {
	return installment.NewService(installment.Deps{Repo: repo, Subs: subs, Cards: cards, Refs: refs})
}

func validInput() installment.CreateInput {
	return installment.CreateInput{
		CreditCardID: "card-1", SubcategoryID: "sub-1", Title: "Notebook",
		InstallmentsCount: 10, InputMode: installment.ByTotal, TotalAmount: 500000,
		PurchaseDate: "2026-06-22",
	}
}

func despesaSvc(repo installment.Repository) *installment.Service {
	return newSvc(repo, &fakeSubs{typ: "despesa"}, &fakeCards{}, &fakeRefs{})
}

// ─── Preview ────────────────────────────────────────────────────────────────

func TestService_Preview_NoPersist(t *testing.T) {
	repo := newFakeRepo()
	plan, err := despesaSvc(repo).Preview(context.Background(), validInput())
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if repo.created {
		t.Error("preview NÃO deveria persistir")
	}
	if plan.TotalAmount != 500000 || len(plan.Installments) != 10 {
		t.Errorf("plano inesperado: %+v", plan)
	}
	if plan.Installments[0].Reference != "2026-06" {
		t.Errorf("reference[0]: got %q", plan.Installments[0].Reference)
	}
}

func TestService_Preview_Interest(t *testing.T) {
	repo := newFakeRepo()
	in := validInput()
	in.InputMode = installment.ByInstallment
	in.TotalAmount = 0
	in.InstallmentAmount = 55000
	in.PrincipalAmount = ptr(int64(500000))
	plan, err := despesaSvc(repo).Preview(context.Background(), in)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if plan.TotalAmount != 550000 || plan.InterestAmount != 50000 {
		t.Errorf("juros: total=%d interest=%d, want 550000/50000", plan.TotalAmount, plan.InterestAmount)
	}
}

// ─── Create ─────────────────────────────────────────────────────────────────

func TestService_Create_Success(t *testing.T) {
	repo := newFakeRepo()
	d, err := despesaSvc(repo).Create(context.Background(), validInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !repo.created || d.ID == "" || len(d.Installments) != 10 {
		t.Errorf("create inesperado: %+v", d)
	}
	if d.Status != installment.GroupActive || d.RemainingCount != 10 || d.RemainingAmount != 500000 {
		t.Errorf("indicadores: %+v", d)
	}
	if d.FirstReference != "2026-06" {
		t.Errorf("first_reference: got %q", d.FirstReference)
	}
}

func TestService_Create_ValidationError(t *testing.T) {
	in := validInput()
	in.InstallmentsCount = 1
	if _, err := despesaSvc(newFakeRepo()).Create(context.Background(), in); err == nil {
		t.Error("esperava erro de validação")
	}
}

func TestService_Create_NotExpense(t *testing.T) {
	svc := newSvc(newFakeRepo(), &fakeSubs{typ: "receita"}, &fakeCards{}, &fakeRefs{})
	_, err := svc.Create(context.Background(), validInput())
	if err != installment.ErrOnlyExpensesInstallable {
		t.Errorf("got %v, want ErrOnlyExpensesInstallable", err)
	}
}

func TestService_Create_CardNotLinkable(t *testing.T) {
	svc := newSvc(newFakeRepo(), &fakeSubs{typ: "despesa"}, &fakeCards{err: errors.New("cartão arquivado")}, &fakeRefs{})
	if _, err := svc.Create(context.Background(), validInput()); err == nil {
		t.Error("esperava erro do cartão propagado")
	}
}

func TestService_Create_RepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.forceErr = errors.New("db down")
	if _, err := despesaSvc(repo).Create(context.Background(), validInput()); err == nil {
		t.Error("esperava erro do repo")
	}
}

// ─── Get / Update / Cancel / Delete ─────────────────────────────────────────

func TestService_Get_DerivesStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := despesaSvc(repo)
	d, _ := svc.Create(context.Background(), validInput())
	// paga 1, cancela nada → ativo
	repo.parcelas[d.ID][0].Status = "realizado"

	got, err := svc.Get(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PaidCount != 1 || got.RemainingCount != 9 || got.Status != installment.GroupActive {
		t.Errorf("indicadores: %+v", got)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	if _, err := despesaSvc(newFakeRepo()).Get(context.Background(), "ghost"); err != installment.ErrInstallmentGroupNotFound {
		t.Errorf("got %v, want ErrInstallmentGroupNotFound", err)
	}
}

func TestService_UpdateSeries(t *testing.T) {
	repo := newFakeRepo()
	svc := despesaSvc(repo)
	d, _ := svc.Create(context.Background(), validInput())

	got, err := svc.UpdateSeries(context.Background(), installment.UpdateSeriesInput{
		ID: d.ID, Title: "Notebook Novo", SubcategoryID: "sub-1",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Title != "Notebook Novo" {
		t.Errorf("title: got %q", got.Title)
	}
}

func TestService_UpdateSeries_NotExpense(t *testing.T) {
	repo := newFakeRepo()
	despesaSvc(repo).Create(context.Background(), validInput())
	var id string
	for k := range repo.groups {
		id = k
	}
	svc := newSvc(repo, &fakeSubs{typ: "transferencia"}, &fakeCards{}, &fakeRefs{})
	_, err := svc.UpdateSeries(context.Background(), installment.UpdateSeriesInput{ID: id, Title: "X", SubcategoryID: "sub-2"})
	if err != installment.ErrOnlyExpensesInstallable {
		t.Errorf("got %v, want ErrOnlyExpensesInstallable", err)
	}
}

func TestService_CancelRemaining(t *testing.T) {
	repo := newFakeRepo()
	svc := despesaSvc(repo)
	d, _ := svc.Create(context.Background(), validInput())
	repo.parcelas[d.ID][0].Status = "realizado" // paga 1

	got, err := svc.CancelRemaining(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.RemainingCount != 0 || got.Status != installment.GroupCancelled {
		t.Errorf("após cancelar: remaining=%d status=%s", got.RemainingCount, got.Status)
	}
	if got.PaidCount != 1 {
		t.Errorf("parcela paga deveria permanecer: paid=%d", got.PaidCount)
	}
}

func TestService_CancelRemaining_NotFound(t *testing.T) {
	if _, err := despesaSvc(newFakeRepo()).CancelRemaining(context.Background(), "ghost"); err != installment.ErrInstallmentGroupNotFound {
		t.Errorf("got %v, want not found", err)
	}
}

func TestService_Delete(t *testing.T) {
	repo := newFakeRepo()
	svc := despesaSvc(repo)
	d, _ := svc.Create(context.Background(), validInput())
	if err := svc.Delete(context.Background(), d.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := repo.groups[d.ID]; ok {
		t.Error("grupo deveria ter sido removido")
	}
}

func TestService_List(t *testing.T) {
	repo := newFakeRepo()
	svc := despesaSvc(repo)
	svc.Create(context.Background(), validInput())
	svc.Create(context.Background(), validInput())

	res, err := svc.List(context.Background(), installment.Filter{}, shared.DefaultPagination())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Pagination.Total != 2 || len(res.Data) != 2 {
		t.Errorf("got total=%d len=%d, want 2/2", res.Pagination.Total, len(res.Data))
	}
}
