package creditcard

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Compile-time interface checks ──────────────────────────────────────────

var (
	_ CreateCreditCardUseCase   = (*createCreditCardImpl)(nil)
	_ GetCreditCardUseCase      = (*getCreditCardImpl)(nil)
	_ ListCreditCardsUseCase    = (*listCreditCardsImpl)(nil)
	_ UpdateCreditCardUseCase   = (*updateCreditCardImpl)(nil)
	_ DeleteCreditCardUseCase   = (*deleteCreditCardImpl)(nil)
	_ ArchiveCreditCardUseCase  = (*archiveCreditCardImpl)(nil)
	_ ListInvoicesUseCase       = (*listInvoicesImpl)(nil)
	_ GetInvoiceUseCase         = (*getInvoiceImpl)(nil)
	_ AddInvoicePaymentUseCase  = (*addInvoicePaymentImpl)(nil)
	_ UndoInvoicePaymentUseCase = (*undoInvoicePaymentImpl)(nil)
	_ MonthlyCardSummaryUseCase = (*monthlyCardSummaryImpl)(nil)
)

// today fixo para determinismo
func fixedNow() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeCardRepo struct {
	data     map[string]CreditCard
	forceErr error
}

func newFakeCardRepo() *fakeCardRepo { return &fakeCardRepo{data: map[string]CreditCard{}} }

func (f *fakeCardRepo) Create(_ context.Context, c CreditCard) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[c.ID] = c
	return nil
}
func (f *fakeCardRepo) Get(_ context.Context, id string) (CreditCard, error) {
	if f.forceErr != nil {
		return CreditCard{}, f.forceErr
	}
	c, ok := f.data[id]
	if !ok {
		return CreditCard{}, ErrCreditCardNotFound
	}
	return c, nil
}
func (f *fakeCardRepo) List(_ context.Context, archived bool, p shared.Pagination) ([]CreditCard, int, error) {
	if f.forceErr != nil {
		return nil, 0, f.forceErr
	}
	var all []CreditCard
	for _, c := range f.data {
		if c.Archived == archived {
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := len(all)
	start := p.Offset()
	if start > total {
		start = total
	}
	end := start + p.Limit
	if p.Limit <= 0 || end > total {
		end = total
	}
	return all[start:end], total, nil
}
func (f *fakeCardRepo) Update(_ context.Context, c CreditCard) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	if _, ok := f.data[c.ID]; !ok {
		return ErrCreditCardNotFound
	}
	f.data[c.ID] = c
	return nil
}
func (f *fakeCardRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.data[id]; !ok {
		return ErrCreditCardNotFound
	}
	delete(f.data, id)
	return nil
}
func (f *fakeCardRepo) SetArchived(_ context.Context, id string, archived bool) error {
	c, ok := f.data[id]
	if !ok {
		return ErrCreditCardNotFound
	}
	c.Archived = archived
	f.data[id] = c
	return nil
}

type fakePayRepo struct {
	data    map[string]map[string][]InvoicePayment // cardID → reference → ledger
	lastAdd *AtomicAddPaymentInput
	lastRem *AtomicRemovePaymentInput
}

func newFakePayRepo() *fakePayRepo {
	return &fakePayRepo{data: map[string]map[string][]InvoicePayment{}}
}

// seed adiciona um pagamento ao ledger (helper de teste).
func (f *fakePayRepo) seed(cardID string, p InvoicePayment) {
	if f.data[cardID] == nil {
		f.data[cardID] = map[string][]InvoicePayment{}
	}
	f.data[cardID][p.Reference] = append(f.data[cardID][p.Reference], p)
}

func (f *fakePayRepo) ListByCard(_ context.Context, cardID string) (map[string][]InvoicePayment, error) {
	out := map[string][]InvoicePayment{}
	for ref, ps := range f.data[cardID] {
		out[ref] = append([]InvoicePayment{}, ps...)
	}
	return out, nil
}
func (f *fakePayRepo) AddPaymentAtomic(_ context.Context, in AtomicAddPaymentInput) error {
	cp := in
	f.lastAdd = &cp
	f.seed(in.CardID, in.Entry)
	return nil
}
func (f *fakePayRepo) RemovePaymentAtomic(_ context.Context, in AtomicRemovePaymentInput) error {
	cp := in
	f.lastRem = &cp
	for card, byRef := range f.data {
		for ref, ps := range byRef {
			kept := make([]InvoicePayment, 0, len(ps))
			found := false
			for _, p := range ps {
				if p.ID == in.PaymentID {
					found = true
					continue
				}
				kept = append(kept, p)
			}
			if found {
				f.data[card][ref] = kept
				return nil
			}
		}
	}
	return ErrPaymentNotFound
}

// fakeSubReader satisfaz SubcategoryReader (deriva o tipo do lançamento de pagamento).
type fakeSubReader struct {
	typ string
	err error
}

func (f fakeSubReader) GetSubcategoryType(_ context.Context, _ string) (string, error) {
	return f.typ, f.err
}

type fakeReader struct {
	txns map[string][]shared.CardTransaction // cardID → txns
}

func newFakeReader() *fakeReader { return &fakeReader{txns: map[string][]shared.CardTransaction{}} }

func (f *fakeReader) ListByCard(_ context.Context, cardID, from, to string) ([]shared.CardTransaction, error) {
	var out []shared.CardTransaction
	for _, t := range f.txns[cardID] {
		if t.CompetenceDate >= from && t.CompetenceDate <= to {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeReader) HasTransactions(_ context.Context, cardID string) (bool, error) {
	return len(f.txns[cardID]) > 0, nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

func seedCard(repo *fakeCardRepo, id string, archived bool) {
	repo.data[id] = CreditCard{
		ID: id, Name: "Card " + id, Brand: BrandMastercard,
		CreditLimit: 500000, ClosingDay: 3, DueDay: 10, Archived: archived,
	}
}

func cardTxn(id, competence string, amount int64) shared.CardTransaction {
	return shared.CardTransaction{
		ID: id, Amount: amount, CompetenceDate: competence, Status: "realizado",
		CategoryID: "cat-1", CategoryName: "Cat 1", CategoryColor: "#fff", CreditCardID: "card-1",
	}
}

func pendingTxn(id, competence string, amount int64) shared.CardTransaction {
	t := cardTxn(id, competence, amount)
	t.Status = "pendente"
	return t
}

func strPtrCC(s string) *string { return &s }

// ─── Create ─────────────────────────────────────────────────────────────────

func TestCreate_Success_DefaultIcon(t *testing.T) {
	repo := newFakeCardRepo()
	uc := NewCreateCreditCard(repo)
	c, err := uc.Execute(context.Background(), CreateCreditCardInput{
		Name: "Nubank", Brand: BrandMastercard, CreditLimit: 500000, ClosingDay: 3, DueDay: 10,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.ID == "" {
		t.Error("expected generated id")
	}
	if c.Icon == nil || *c.Icon != DefaultIcon {
		t.Errorf("expected default icon, got %v", c.Icon)
	}
	if c.Archived {
		t.Error("expected archived=false on create")
	}
}

func TestCreate_ValidationError(t *testing.T) {
	uc := NewCreateCreditCard(newFakeCardRepo())
	_, err := uc.Execute(context.Background(), CreateCreditCardInput{Name: "", CreditLimit: 0})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := newFakeCardRepo()
	repo.forceErr = errors.New("db down")
	uc := NewCreateCreditCard(repo)
	_, err := uc.Execute(context.Background(), CreateCreditCardInput{
		Name: "X", Brand: BrandVisa, CreditLimit: 1000, ClosingDay: 1, DueDay: 2,
	})
	if err == nil {
		t.Error("expected repo error")
	}
}

// ─── Get / buildDetail ──────────────────────────────────────────────────────

func TestGet_BuildsIndicators(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	reader := newFakeReader()
	// compra em 2026-07-20 cai na fatura aberta (ref 2026-08, ciclo 04/07..03/08)
	reader.txns["card-1"] = []shared.CardTransaction{cardTxn("t1", "2026-07-20", 132000)}

	uc := &getCreditCardImpl{repo: repo, payRepo: newFakePayRepo(), reader: reader, now: fixedNow}
	d, err := uc.Execute(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if d.UsedLimit != 132000 {
		t.Errorf("usedLimit: got %d, want 132000", d.UsedLimit)
	}
	if d.AvailableLimit != 368000 {
		t.Errorf("availableLimit: got %d, want 368000", d.AvailableLimit)
	}
	if d.UtilizationPercent != 26 || d.UtilizationLevel != LevelSaudavel {
		t.Errorf("utilization: got %d/%s", d.UtilizationPercent, d.UtilizationLevel)
	}
	if d.BestPurchaseDay != 4 {
		t.Errorf("bestPurchaseDay: got %d, want 4", d.BestPurchaseDay)
	}
	if d.OpenInvoice == nil {
		t.Fatal("openInvoice should never be nil")
	}
	if d.OpenInvoice.Reference != "2026-08" || d.OpenInvoice.Total != 132000 || d.OpenInvoice.Status != StatusAberta {
		t.Errorf("openInvoice inesperada: %+v", d.OpenInvoice)
	}
}

func TestGet_EmptyCardHasZeroedOpenInvoice(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	uc := &getCreditCardImpl{repo: repo, payRepo: newFakePayRepo(), reader: newFakeReader(), now: fixedNow}
	d, err := uc.Execute(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if d.OpenInvoice == nil || d.OpenInvoice.Total != 0 || d.OpenInvoice.Status != StatusAberta {
		t.Errorf("expected zeroed open invoice, got %+v", d.OpenInvoice)
	}
	if d.UsedLimit != 0 || d.AvailableLimit != 500000 {
		t.Errorf("limits: got used=%d avail=%d", d.UsedLimit, d.AvailableLimit)
	}
}

func TestGet_NotFound(t *testing.T) {
	uc := &getCreditCardImpl{repo: newFakeCardRepo(), payRepo: newFakePayRepo(), reader: newFakeReader(), now: fixedNow}
	if _, err := uc.Execute(context.Background(), "ghost"); err == nil {
		t.Error("expected not-found")
	}
}

// ─── List ───────────────────────────────────────────────────────────────────

func TestList_ReturnsDetails(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	seedCard(repo, "card-2", false)
	seedCard(repo, "card-3", true)
	uc := &listCreditCardsImpl{repo: repo, payRepo: newFakePayRepo(), reader: newFakeReader(), now: fixedNow}

	res, err := uc.Execute(context.Background(), ListInput{
		Archived:   false,
		Pagination: shared.Pagination{Page: 1, Limit: 10, OrderBy: "name", Order: "ASC"},
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.Pagination.Total != 2 || len(res.Data) != 2 {
		t.Errorf("got total=%d len=%d, want 2/2", res.Pagination.Total, len(res.Data))
	}
	for _, d := range res.Data {
		if d.OpenInvoice == nil {
			t.Error("each card should carry an open invoice")
		}
	}
}

// ─── Update ─────────────────────────────────────────────────────────────────

func TestUpdate_Success(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	uc := NewUpdateCreditCard(repo)
	c, err := uc.Execute(context.Background(), UpdateCreditCardInput{
		ID: "card-1", Name: "Novo", Brand: BrandVisa, CreditLimit: 700000, ClosingDay: 5, DueDay: 12,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.Name != "Novo" || c.CreditLimit != 700000 {
		t.Errorf("update not reflected: %+v", c)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	uc := NewUpdateCreditCard(newFakeCardRepo())
	_, err := uc.Execute(context.Background(), UpdateCreditCardInput{
		ID: "ghost", Name: "X", Brand: BrandVisa, CreditLimit: 1000, ClosingDay: 1, DueDay: 2,
	})
	if err == nil {
		t.Error("expected not-found")
	}
}

func TestUpdate_ValidationError(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	uc := NewUpdateCreditCard(repo)
	_, err := uc.Execute(context.Background(), UpdateCreditCardInput{ID: "card-1", Name: "", CreditLimit: 0})
	if err == nil {
		t.Error("expected validation error")
	}
}

// ─── Delete ─────────────────────────────────────────────────────────────────

func TestDelete_Success(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	uc := NewDeleteCreditCard(repo, newFakeReader())
	if err := uc.Execute(context.Background(), "card-1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if _, ok := repo.data["card-1"]; ok {
		t.Error("expected card removed")
	}
}

func TestDelete_BlockedByTransactions(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	reader := newFakeReader()
	reader.txns["card-1"] = []shared.CardTransaction{cardTxn("t1", "2026-07-20", 1000)}
	uc := NewDeleteCreditCard(repo, reader)
	err := uc.Execute(context.Background(), "card-1")
	if err != ErrCardHasTransactions {
		t.Errorf("expected ErrCardHasTransactions, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	uc := NewDeleteCreditCard(newFakeCardRepo(), newFakeReader())
	if err := uc.Execute(context.Background(), "ghost"); err == nil {
		t.Error("expected not-found")
	}
}

// ─── Archive ────────────────────────────────────────────────────────────────

func TestArchive_And_Unarchive(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	uc := NewArchiveCreditCard(repo)
	if err := uc.Execute(context.Background(), "card-1", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !repo.data["card-1"].Archived {
		t.Error("expected archived")
	}
	if err := uc.Execute(context.Background(), "card-1", false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if repo.data["card-1"].Archived {
		t.Error("expected unarchived")
	}
}

func TestArchive_NotFound(t *testing.T) {
	uc := NewArchiveCreditCard(newFakeCardRepo())
	if err := uc.Execute(context.Background(), "ghost", true); err == nil {
		t.Error("expected not-found")
	}
}

// ─── Invoices ───────────────────────────────────────────────────────────────

func payDeps(t *testing.T) (*fakeCardRepo, *fakePayRepo, *fakeReader) {
	t.Helper()
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	reader := newFakeReader()
	// compra em 2026-06-20 → ref 2026-07 (closing 03/07, due 10/07) → vencida em 20/07
	// compra em 2026-07-20 → ref 2026-08 (aberta)
	reader.txns["card-1"] = []shared.CardTransaction{
		cardTxn("past", "2026-06-20", 20000),
		cardTxn("open", "2026-07-20", 30000),
	}
	return repo, newFakePayRepo(), reader
}

func TestListInvoices(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &listInvoicesImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	invs, err := uc.Execute(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// refs 2026-07 e 2026-08, ordenadas desc
	if len(invs) != 2 || invs[0].Reference != "2026-08" || invs[1].Reference != "2026-07" {
		t.Fatalf("faturas inesperadas: %+v", invs)
	}
	if invs[0].Status != StatusAberta || invs[1].Status != StatusVencida {
		t.Errorf("status: %s / %s", invs[0].Status, invs[1].Status)
	}
}

func TestGetInvoice_FoundAndPaginated(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &getInvoiceImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	det, err := uc.Execute(context.Background(), "card-1", "2026-07", shared.Pagination{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if det.Reference != "2026-07" || det.Total != 20000 {
		t.Errorf("invoice inesperada: %+v", det.Invoice)
	}
	if det.Pagination.Total != 1 || len(det.Data) != 1 {
		t.Errorf("pagination: total=%d len=%d", det.Pagination.Total, len(det.Data))
	}
}

func TestGetInvoice_NotFound(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &getInvoiceImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	if _, err := uc.Execute(context.Background(), "card-1", "2099-01", shared.Pagination{Page: 1, Limit: 10}); err != ErrInvoiceNotFound {
		t.Errorf("expected ErrInvoiceNotFound, got %v", err)
	}
}

func TestAddInvoicePayment_FullPayment(t *testing.T) {
	repo, pay, reader := payDeps(t)
	// "past" (ref 2026-07) começa pendente → deve ser realizada ao quitar.
	reader.txns["card-1"] = []shared.CardTransaction{
		pendingTxn("past", "2026-06-20", 20000),
		cardTxn("open", "2026-07-20", 30000),
	}
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	inv, err := uc.Execute(context.Background(), AddInvoicePaymentInput{
		CardID: "card-1", Reference: "2026-07", Amount: 20000, PaymentDate: "2026-07-15", SubcategoryID: "sub-trf-pgto",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if inv.Status != StatusPaga || inv.PaymentStatus != PaymentPaga || inv.OutstandingAmount != 0 {
		t.Errorf("fatura deveria ficar paga: %+v", inv)
	}
	if len(pay.data["card-1"]["2026-07"]) != 1 {
		t.Fatalf("pagamento não persistido no ledger")
	}
	// quitou → baixa em lote da compra pendente.
	if pay.lastAdd == nil || len(pay.lastAdd.RealizeIDs) != 1 || pay.lastAdd.RealizeIDs[0] != "past" {
		t.Errorf("RealizeIDs inesperados: %+v", pay.lastAdd)
	}
	p := pay.lastAdd.Payment
	if p.Type != "transferencia" || p.Amount != 20000 || p.PaymentMethod != "outros" {
		t.Errorf("payment txn inesperado: %+v", p)
	}
}

func TestAddInvoicePayment_Partial(t *testing.T) {
	repo, pay, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{pendingTxn("past", "2026-06-20", 20000)}
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	inv, err := uc.Execute(context.Background(), AddInvoicePaymentInput{
		CardID: "card-1", Reference: "2026-07", Amount: 8000, PaymentDate: "2026-07-15", SubcategoryID: "sub-trf-pgto",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if inv.PaymentStatus != PaymentParcial || inv.PaidAmount != 8000 || inv.OutstandingAmount != 12000 || inv.Status == StatusPaga {
		t.Errorf("parcial inesperado: %+v", inv)
	}
	// parcial não quita → sem baixa em lote.
	if pay.lastAdd == nil || len(pay.lastAdd.RealizeIDs) != 0 {
		t.Errorf("parcial não deveria realizar compras: %+v", pay.lastAdd)
	}
}

func TestAddInvoicePayment_OpenInvoiceAllowed(t *testing.T) {
	repo, pay, reader := payDeps(t) // ref 2026-08 aberta (compra "open" 30000)
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	inv, err := uc.Execute(context.Background(), AddInvoicePaymentInput{
		CardID: "card-1", Reference: "2026-08", Amount: 10000, PaymentDate: "2026-07-20", SubcategoryID: "sub-trf-pgto",
	})
	if err != nil {
		t.Fatalf("pagar fatura aberta deveria ser permitido (antecipamento): %v", err)
	}
	if inv.PaymentStatus != PaymentParcial || inv.PaidAmount != 10000 {
		t.Errorf("antecipado parcial em fatura aberta: %+v", inv)
	}
}

func TestAddInvoicePayment_ExceedsBalance(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	_, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2026-07", Amount: 25000, PaymentDate: "2026-07-15", SubcategoryID: "sub-trf-pgto"})
	if err != ErrPaymentExceedsBalance {
		t.Errorf("expected ErrPaymentExceedsBalance, got %v", err)
	}
}

func TestAddInvoicePayment_AlreadyPaid(t *testing.T) {
	repo, pay, reader := payDeps(t)
	pay.seed("card-1", InvoicePayment{ID: "p1", Reference: "2026-07", Amount: 20000, PaymentDate: "2026-07-10"})
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	_, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2026-07", Amount: 1000, PaymentDate: "2026-07-15", SubcategoryID: "sub-trf-pgto"})
	if err != ErrInvoiceAlreadyPaid {
		t.Errorf("expected ErrInvoiceAlreadyPaid, got %v", err)
	}
}

func TestAddInvoicePayment_InvalidAmountAndDateAndSubcat(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	if _, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2026-07", Amount: 0, PaymentDate: "2026-07-15", SubcategoryID: "s"}); err != ErrInvalidPaymentAmount {
		t.Errorf("valor 0 → ErrInvalidPaymentAmount, got %v", err)
	}
	if _, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2026-07", Amount: 100, PaymentDate: "15/07/2026", SubcategoryID: "s"}); err == nil {
		t.Error("data inválida deveria falhar")
	}
	if _, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2026-07", Amount: 100, PaymentDate: "2026-07-15"}); err == nil {
		t.Error("subcategoria ausente deveria falhar")
	}
}

func TestAddInvoicePayment_NotFound(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &addInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, subs: fakeSubReader{typ: "transferencia"}, now: fixedNow}
	_, err := uc.Execute(context.Background(), AddInvoicePaymentInput{CardID: "card-1", Reference: "2099-01", Amount: 100, PaymentDate: "2026-07-15", SubcategoryID: "sub-trf-pgto"})
	if err != ErrInvoiceNotFound {
		t.Errorf("expected ErrInvoiceNotFound, got %v", err)
	}
}

func TestUndoInvoicePayment_Success(t *testing.T) {
	repo, pay, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{cardTxn("past", "2026-06-20", 20000)} // realizado (quitada)
	pay.seed("card-1", InvoicePayment{ID: "p1", Reference: "2026-07", Amount: 20000, PaymentDate: "2026-07-10", TransactionID: strPtrCC("pay-txn-1")})
	uc := &undoInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	inv, err := uc.Execute(context.Background(), "card-1", "2026-07", "p1")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if inv.PaymentStatus == PaymentPaga {
		t.Error("não deveria continuar paga após desfazer")
	}
	if len(pay.data["card-1"]["2026-07"]) != 0 {
		t.Error("pagamento deveria ter sido removido")
	}
	if pay.lastRem == nil || pay.lastRem.PaymentTxnID != "pay-txn-1" {
		t.Errorf("undo não passou o PaymentTxnID: %+v", pay.lastRem)
	}
	// estava quitada e deixou de estar → reverte a compra realizada.
	if len(pay.lastRem.RevertIDs) != 1 || pay.lastRem.RevertIDs[0] != "past" {
		t.Errorf("RevertIDs inesperados: %+v", pay.lastRem)
	}
}

func TestUndoInvoicePayment_PartialNoRevert(t *testing.T) {
	repo, pay, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{pendingTxn("past", "2026-06-20", 20000)}
	pay.seed("card-1", InvoicePayment{ID: "p1", Reference: "2026-07", Amount: 8000, PaymentDate: "2026-07-10", TransactionID: strPtrCC("pay-txn-1")})
	uc := &undoInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	if _, err := uc.Execute(context.Background(), "card-1", "2026-07", "p1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// não estava quitada → não reabre compras.
	if pay.lastRem == nil || len(pay.lastRem.RevertIDs) != 0 {
		t.Errorf("parcial não deveria reverter compras: %+v", pay.lastRem)
	}
}

func TestUndoInvoicePayment_NotFound(t *testing.T) {
	repo, pay, reader := payDeps(t)
	uc := &undoInvoicePaymentImpl{repo: repo, payRepo: pay, reader: reader, now: fixedNow}
	if _, err := uc.Execute(context.Background(), "card-1", "2026-07", "nope"); err != ErrPaymentNotFound {
		t.Errorf("expected ErrPaymentNotFound, got %v", err)
	}
}

// ─── Monthly summary ────────────────────────────────────────────────────────

func TestMonthlySummary_Success(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	reader := newFakeReader()
	reader.txns["card-1"] = []shared.CardTransaction{
		cardTxn("a", "2026-06-05", 100000),
		cardTxn("b", "2026-06-25", 50000),
		cardTxn("c", "2026-07-01", 999999), // fora do mês 6
	}
	uc := NewMonthlyCardSummary(repo, reader)
	s, err := uc.Execute(context.Background(), "card-1", 2026, 6)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.Total != 150000 || s.Count != 2 || s.AverageTicket != 75000 {
		t.Errorf("resumo inesperado: %+v", s)
	}
}

func TestMonthlySummary_CardNotFound(t *testing.T) {
	uc := NewMonthlyCardSummary(newFakeCardRepo(), newFakeReader())
	if _, err := uc.Execute(context.Background(), "ghost", 2026, 6); err == nil {
		t.Error("expected not-found")
	}
}
