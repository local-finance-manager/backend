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
	_ PayInvoiceUseCase         = (*payInvoiceImpl)(nil)
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

// fakeWriter satisfaz InvoicePaymentWriter: marca/reverte as compras (mutando o fakeReader,
// para refletir o estado em passos seguintes) e registra a última chamada.
type fakeWriter struct {
	reader        *fakeReader
	lastMarkIDs   []string
	lastMarkDate  string
	lastRevertIDs []string
}

func (w *fakeWriter) MarkInvoicePaid(_ context.Context, ids []string, paymentDate string) error {
	w.lastMarkIDs, w.lastMarkDate = ids, paymentDate
	w.reader.setStatus(ids, statusRealizado, &paymentDate)
	return nil
}
func (w *fakeWriter) RevertInvoicePayment(_ context.Context, ids []string) error {
	w.lastRevertIDs = ids
	w.reader.setStatus(ids, statusPendente, nil)
	return nil
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

// setStatus muta as compras (por id) — usado pelo fakeWriter para refletir pagar/desfazer.
func (f *fakeReader) setStatus(ids []string, status string, payDate *string) {
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for card := range f.txns {
		for i := range f.txns[card] {
			if _, ok := set[f.txns[card][i].ID]; ok {
				f.txns[card][i].Status = status
				f.txns[card][i].PaymentDate = payDate
			}
		}
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func seedCard(repo *fakeCardRepo, id string, archived bool) {
	repo.data[id] = CreditCard{
		ID: id, Name: "Card " + id, Brand: BrandMastercard,
		CreditLimit: 500000, ClosingDay: 3, DueDay: 10, Archived: archived,
	}
}

// cardTxn é uma compra de cartão EM ABERTO (pendente) — estado natural até a fatura ser paga.
func cardTxn(id, competence string, amount int64) shared.CardTransaction {
	return shared.CardTransaction{
		ID: id, Amount: amount, CompetenceDate: competence, Status: statusPendente,
		CategoryID: "cat-1", CategoryName: "Cat 1", CategoryColor: "#fff", CreditCardID: "card-1",
	}
}

func pendingTxn(id, competence string, amount int64) shared.CardTransaction {
	return cardTxn(id, competence, amount)
}

// paidTxn é uma compra já paga (realizado) numa data de pagamento.
func paidTxn(id, competence string, amount int64, payDate string) shared.CardTransaction {
	t := cardTxn(id, competence, amount)
	t.Status = statusRealizado
	t.PaymentDate = &payDate
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

	uc := &getCreditCardImpl{repo: repo, reader: reader, now: fixedNow}
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
	uc := &getCreditCardImpl{repo: repo, reader: newFakeReader(), now: fixedNow}
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
	uc := &getCreditCardImpl{repo: newFakeCardRepo(), reader: newFakeReader(), now: fixedNow}
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
	uc := &listCreditCardsImpl{repo: repo, reader: newFakeReader(), now: fixedNow}

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

func payDeps(t *testing.T) (*fakeCardRepo, *fakeReader) {
	t.Helper()
	repo := newFakeCardRepo()
	seedCard(repo, "card-1", false)
	reader := newFakeReader()
	// compra em 2026-06-20 → ref 2026-07 (closing 03/07, due 10/07) → vencida em 20/07 (em aberto)
	// compra em 2026-07-20 → ref 2026-08 (aberta, em aberto)
	reader.txns["card-1"] = []shared.CardTransaction{
		cardTxn("past", "2026-06-20", 20000),
		cardTxn("open", "2026-07-20", 30000),
	}
	return repo, reader
}

func TestListInvoices(t *testing.T) {
	repo, reader := payDeps(t)
	uc := &listInvoicesImpl{repo: repo, reader: reader, now: fixedNow}
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
	repo, reader := payDeps(t)
	uc := &getInvoiceImpl{repo: repo, reader: reader, now: fixedNow}
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
	repo, reader := payDeps(t)
	uc := &getInvoiceImpl{repo: repo, reader: reader, now: fixedNow}
	if _, err := uc.Execute(context.Background(), "card-1", "2099-01", shared.Pagination{Page: 1, Limit: 10}); err != ErrInvoiceNotFound {
		t.Errorf("expected ErrInvoiceNotFound, got %v", err)
	}
}

func TestPayInvoice_FullOpenBalance(t *testing.T) {
	repo, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{
		pendingTxn("past", "2026-06-20", 20000),
		cardTxn("open", "2026-07-20", 30000),
	}
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	inv, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2026-07", PaymentDate: "2026-07-15"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if inv.Status != StatusPaga || inv.PaymentStatus != PaymentPaga || inv.OutstandingAmount != 0 {
		t.Errorf("fatura deveria ficar paga: %+v", inv)
	}
	// só a compra em aberto da fatura 2026-07 foi marcada (não a "open" de 2026-08).
	if len(writer.lastMarkIDs) != 1 || writer.lastMarkIDs[0] != "past" || writer.lastMarkDate != "2026-07-15" {
		t.Errorf("marcação inesperada: ids=%v date=%s", writer.lastMarkIDs, writer.lastMarkDate)
	}
	if len(inv.Payments) != 1 || inv.Payments[0].PaymentDate != "2026-07-15" || inv.Payments[0].Amount != 20000 {
		t.Errorf("pagamentos derivados: %+v", inv.Payments)
	}
}

func TestPayInvoice_PaysOnlyOpenBatches(t *testing.T) {
	repo, reader := payDeps(t)
	// fatura 2026-07: uma compra já paga (07-10) + uma nova em aberto.
	reader.txns["card-1"] = []shared.CardTransaction{
		paidTxn("a", "2026-06-18", 10000, "2026-07-10"),
		pendingTxn("b", "2026-06-25", 5000),
	}
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	inv, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2026-07", PaymentDate: "2026-07-18"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(writer.lastMarkIDs) != 1 || writer.lastMarkIDs[0] != "b" {
		t.Errorf("deveria pagar só as em aberto: %v", writer.lastMarkIDs)
	}
	if len(inv.Payments) != 2 || inv.PaymentStatus != PaymentPaga {
		t.Errorf("esperava 2 lotes e fatura paga: %+v", inv)
	}
}

func TestPayInvoice_OpenInvoiceAllowed(t *testing.T) {
	repo, reader := payDeps(t) // 2026-08 aberta, compra "open" em aberto
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	inv, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2026-08", PaymentDate: "2026-07-20"})
	if err != nil {
		t.Fatalf("pagar fatura aberta deveria ser permitido (antecipamento): %v", err)
	}
	if inv.PaymentStatus != PaymentPaga || len(writer.lastMarkIDs) != 1 || writer.lastMarkIDs[0] != "open" {
		t.Errorf("antecipado em fatura aberta: %+v ids=%v", inv, writer.lastMarkIDs)
	}
}

func TestPayInvoice_AlreadyPaid(t *testing.T) {
	repo, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{paidTxn("a", "2026-06-20", 20000, "2026-07-10")}
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	if _, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2026-07", PaymentDate: "2026-07-15"}); err != ErrInvoiceAlreadyPaid {
		t.Errorf("expected ErrInvoiceAlreadyPaid, got %v", err)
	}
}

func TestPayInvoice_Futura(t *testing.T) {
	repo, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{pendingTxn("fut", "2027-01-15", 9000)} // ref 2027-02 (futura)
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	if _, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2027-02", PaymentDate: "2026-07-15"}); err != ErrInvoiceFutura {
		t.Errorf("expected ErrInvoiceFutura, got %v", err)
	}
}

func TestPayInvoice_InvalidDateAndNotFound(t *testing.T) {
	repo, reader := payDeps(t)
	writer := &fakeWriter{reader: reader}
	uc := &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	if _, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2026-07", PaymentDate: "15/07/2026"}); err == nil {
		t.Error("data inválida deveria falhar")
	}
	if _, err := uc.Execute(context.Background(), PayInvoiceInput{CardID: "card-1", Reference: "2099-01", PaymentDate: "2026-07-15"}); err != ErrInvoiceNotFound {
		t.Errorf("expected ErrInvoiceNotFound, got %v", err)
	}
}

func TestUndoInvoicePayment_Success(t *testing.T) {
	repo, reader := payDeps(t)
	reader.txns["card-1"] = []shared.CardTransaction{paidTxn("past", "2026-06-20", 20000, "2026-07-10")}
	writer := &fakeWriter{reader: reader}
	uc := &undoInvoicePaymentImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	inv, err := uc.Execute(context.Background(), "card-1", "2026-07", "2026-07-10")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(writer.lastRevertIDs) != 1 || writer.lastRevertIDs[0] != "past" {
		t.Errorf("revert ids: %v", writer.lastRevertIDs)
	}
	if inv.PaymentStatus == PaymentPaga || inv.OutstandingAmount != 20000 {
		t.Errorf("após desfazer deveria reabrir: %+v", inv)
	}
}

func TestUndoInvoicePayment_NotFound(t *testing.T) {
	repo, reader := payDeps(t) // compras em aberto, sem pagamento na data
	writer := &fakeWriter{reader: reader}
	uc := &undoInvoicePaymentImpl{repo: repo, reader: reader, writer: writer, now: fixedNow}
	if _, err := uc.Execute(context.Background(), "card-1", "2026-07", "2026-07-10"); err != ErrPaymentNotFound {
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
