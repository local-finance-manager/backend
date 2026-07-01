package creditcard

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Intervalo largo para "todos os lançamentos do cartão" (datas YYYY-MM-DD ordenáveis).
const (
	wideFrom = "0001-01-01"
	wideTo   = "9999-12-31"
)

func todayFrom(now func() time.Time) string {
	return now().UTC().Format("2006-01-02")
}

// ─── createCreditCardImpl ───────────────────────────────────────────────────

type createCreditCardImpl struct{ repo CreditCardRepository }

func NewCreateCreditCard(repo CreditCardRepository) CreateCreditCardUseCase {
	return &createCreditCardImpl{repo: repo}
}

func (uc *createCreditCardImpl) Execute(ctx context.Context, in CreateCreditCardInput) (CreditCard, error) {
	if err := ValidateCreate(in); err != nil {
		return CreditCard{}, err
	}
	icon := in.Icon
	if icon == nil || *icon == "" {
		def := DefaultIcon
		icon = &def
	}
	now := time.Now().UTC()
	c := CreditCard{
		ID:             uuid.New().String(),
		Name:           in.Name,
		Brand:          in.Brand,
		LastFourDigits: in.LastFourDigits,
		Issuer:         in.Issuer,
		CreditLimit:    in.CreditLimit,
		ClosingDay:     in.ClosingDay,
		DueDay:         in.DueDay,
		Color:          in.Color,
		Icon:           icon,
		Archived:       false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := uc.repo.Create(ctx, c); err != nil {
		return CreditCard{}, domainerr.NewInternal("erro ao criar cartão de crédito")
	}
	return c, nil
}

// ─── getCreditCardImpl ──────────────────────────────────────────────────────

type getCreditCardImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	now    func() time.Time
}

func NewGetCreditCard(repo CreditCardRepository, reader CardTransactionReader) GetCreditCardUseCase {
	return &getCreditCardImpl{repo: repo, reader: reader, now: time.Now}
}

func (uc *getCreditCardImpl) Execute(ctx context.Context, id string) (CreditCardDetail, error) {
	card, err := uc.repo.Get(ctx, id)
	if err != nil {
		return CreditCardDetail{}, err
	}
	return buildDetail(ctx, card, uc.reader, todayFrom(uc.now))
}

// ─── listCreditCardsImpl ────────────────────────────────────────────────────

type listCreditCardsImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	now    func() time.Time
}

func NewListCreditCards(repo CreditCardRepository, reader CardTransactionReader) ListCreditCardsUseCase {
	return &listCreditCardsImpl{repo: repo, reader: reader, now: time.Now}
}

func (uc *listCreditCardsImpl) Execute(ctx context.Context, in ListInput) (ListCreditCardsResult, error) {
	cards, total, err := uc.repo.List(ctx, in.Archived, in.Pagination)
	if err != nil {
		return ListCreditCardsResult{}, domainerr.NewInternal("erro ao listar cartões")
	}
	today := todayFrom(uc.now)
	data := make([]CreditCardDetail, 0, len(cards))
	for _, c := range cards {
		d, err := buildDetail(ctx, c, uc.reader, today)
		if err != nil {
			return ListCreditCardsResult{}, err
		}
		data = append(data, d)
	}

	p := in.Pagination
	totalPages := 1
	if p.Limit > 0 && total > 0 {
		totalPages = (total + p.Limit - 1) / p.Limit
	}
	return ListCreditCardsResult{
		Data: data,
		Pagination: shared.PagedMeta{
			Page: p.Page, Limit: p.Limit, Total: total,
			TotalPages: totalPages, Sort: p.OrderBy, SortDir: p.Order,
		},
	}, nil
}

// ─── updateCreditCardImpl ───────────────────────────────────────────────────

type updateCreditCardImpl struct{ repo CreditCardRepository }

func NewUpdateCreditCard(repo CreditCardRepository) UpdateCreditCardUseCase {
	return &updateCreditCardImpl{repo: repo}
}

func (uc *updateCreditCardImpl) Execute(ctx context.Context, in UpdateCreditCardInput) (CreditCard, error) {
	if err := ValidateUpdate(in); err != nil {
		return CreditCard{}, err
	}
	current, err := uc.repo.Get(ctx, in.ID)
	if err != nil {
		return CreditCard{}, err
	}
	icon := in.Icon
	if icon == nil || *icon == "" {
		def := DefaultIcon
		icon = &def
	}
	current.Name = in.Name
	current.Brand = in.Brand
	current.LastFourDigits = in.LastFourDigits
	current.Issuer = in.Issuer
	current.CreditLimit = in.CreditLimit
	current.ClosingDay = in.ClosingDay
	current.DueDay = in.DueDay
	current.Color = in.Color
	current.Icon = icon
	current.UpdatedAt = time.Now().UTC()
	if err := uc.repo.Update(ctx, current); err != nil {
		return CreditCard{}, domainerr.NewInternal("erro ao atualizar cartão")
	}
	return current, nil
}

// ─── deleteCreditCardImpl ───────────────────────────────────────────────────

type deleteCreditCardImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
}

func NewDeleteCreditCard(repo CreditCardRepository, reader CardTransactionReader) DeleteCreditCardUseCase {
	return &deleteCreditCardImpl{repo: repo, reader: reader}
}

func (uc *deleteCreditCardImpl) Execute(ctx context.Context, id string) error {
	if _, err := uc.repo.Get(ctx, id); err != nil {
		return err
	}
	has, err := uc.reader.HasTransactions(ctx, id)
	if err != nil {
		return domainerr.NewInternal("erro ao verificar lançamentos do cartão")
	}
	if has {
		return ErrCardHasTransactions
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}
	return nil
}

// ─── archiveCreditCardImpl ──────────────────────────────────────────────────

type archiveCreditCardImpl struct{ repo CreditCardRepository }

func NewArchiveCreditCard(repo CreditCardRepository) ArchiveCreditCardUseCase {
	return &archiveCreditCardImpl{repo: repo}
}

func (uc *archiveCreditCardImpl) Execute(ctx context.Context, id string, archived bool) error {
	return uc.repo.SetArchived(ctx, id, archived)
}

// ─── listInvoicesImpl ───────────────────────────────────────────────────────

type listInvoicesImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	now    func() time.Time
}

func NewListInvoices(repo CreditCardRepository, reader CardTransactionReader) ListInvoicesUseCase {
	return &listInvoicesImpl{repo: repo, reader: reader, now: time.Now}
}

func (uc *listInvoicesImpl) Execute(ctx context.Context, cardID string) ([]Invoice, error) {
	card, err := uc.repo.Get(ctx, cardID)
	if err != nil {
		return nil, err
	}
	today := todayFrom(uc.now)
	buckets, err := gatherBuckets(ctx, card, uc.reader, today)
	if err != nil {
		return nil, err
	}
	return buildInvoices(card, buckets, today)
}

// ─── getInvoiceImpl ─────────────────────────────────────────────────────────

type getInvoiceImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	now    func() time.Time
}

func NewGetInvoice(repo CreditCardRepository, reader CardTransactionReader) GetInvoiceUseCase {
	return &getInvoiceImpl{repo: repo, reader: reader, now: time.Now}
}

func (uc *getInvoiceImpl) Execute(ctx context.Context, cardID, reference string, p shared.Pagination) (InvoiceDetail, error) {
	card, err := uc.repo.Get(ctx, cardID)
	if err != nil {
		return InvoiceDetail{}, err
	}
	today := todayFrom(uc.now)
	buckets, err := gatherBuckets(ctx, card, uc.reader, today)
	if err != nil {
		return InvoiceDetail{}, err
	}
	cycleTxns, ok := buckets[reference]
	if !ok {
		return InvoiceDetail{}, ErrInvoiceNotFound
	}
	inv, err := BuildInvoice(reference, cycleTxns, card, today)
	if err != nil {
		return InvoiceDetail{}, err
	}
	page, meta := paginate(cycleTxns, p)
	return InvoiceDetail{Invoice: inv, Data: page, Pagination: meta}, nil
}

// ─── addInvoicePaymentImpl ──────────────────────────────────────────────────

type payInvoiceImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	writer InvoicePaymentWriter
	now    func() time.Time
}

func NewPayInvoice(repo CreditCardRepository, reader CardTransactionReader, writer InvoicePaymentWriter) PayInvoiceUseCase {
	return &payInvoiceImpl{repo: repo, reader: reader, writer: writer, now: time.Now}
}

// Execute paga a fatura: marca as compras EM ABERTO dela como realizado na data informada
// (Opção 1 — sem lançamento sintético, sem valor; paga o saldo aberto do momento). Pagar
// de novo depois (com novas compras) quita só as que estiverem em aberto naquele momento.
func (uc *payInvoiceImpl) Execute(ctx context.Context, in PayInvoiceInput) (Invoice, error) {
	if !isValidDate(in.PaymentDate) {
		return Invoice{}, domainerr.NewBadRequest("data de pagamento inválida: use YYYY-MM-DD", domainerr.WithDisplayable())
	}
	card, err := uc.repo.Get(ctx, in.CardID)
	if err != nil {
		return Invoice{}, err
	}
	today := todayFrom(uc.now)
	buckets, err := gatherBuckets(ctx, card, uc.reader, today)
	if err != nil {
		return Invoice{}, err
	}
	cycleTxns, ok := buckets[in.Reference]
	if !ok {
		return Invoice{}, ErrInvoiceNotFound
	}
	inv, err := BuildInvoice(in.Reference, cycleTxns, card, today)
	if err != nil {
		return Invoice{}, err
	}
	if inv.Status == StatusFutura {
		return Invoice{}, ErrInvoiceFutura // ainda não abriu
	}
	if inv.OutstandingAmount == 0 {
		return Invoice{}, ErrInvoiceAlreadyPaid // nada em aberto
	}

	// compras em aberto a marcar como pagas na data informada.
	var ids []string
	for _, t := range cycleTxns {
		if t.Status == statusPendente {
			ids = append(ids, t.ID)
		}
	}
	if err := uc.writer.MarkInvoicePaid(ctx, ids, in.PaymentDate); err != nil {
		return Invoice{}, domainerr.NewInternal("erro ao registrar pagamento da fatura")
	}
	return BuildInvoice(in.Reference, markRealized(cycleTxns, ids, in.PaymentDate), card, today)
}

// markRealized devolve uma cópia do ciclo com as compras realizadas (para a projeção de
// resposta refletir o estado pós-pagamento sem reler do banco).
func markRealized(txns []shared.CardTransaction, realizedIDs []string, payDate string) []shared.CardTransaction {
	realized := make(map[string]struct{}, len(realizedIDs))
	for _, id := range realizedIDs {
		realized[id] = struct{}{}
	}
	out := make([]shared.CardTransaction, len(txns))
	copy(out, txns)
	for i := range out {
		if _, ok := realized[out[i].ID]; ok {
			out[i].Status = statusRealizado
			pd := payDate
			out[i].PaymentDate = &pd
		}
	}
	return out
}

// ─── undoInvoicePaymentImpl ─────────────────────────────────────────────────

type undoInvoicePaymentImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
	writer InvoicePaymentWriter
	now    func() time.Time
}

func NewUndoInvoicePayment(repo CreditCardRepository, reader CardTransactionReader, writer InvoicePaymentWriter) UndoInvoicePaymentUseCase {
	return &undoInvoicePaymentImpl{repo: repo, reader: reader, writer: writer, now: time.Now}
}

// Execute desfaz o pagamento de uma data: volta para pendente as compras da fatura que
// foram pagas naquela data (payment_date). ErrPaymentNotFound se não houver nenhuma.
func (uc *undoInvoicePaymentImpl) Execute(ctx context.Context, cardID, reference, paymentDate string) (Invoice, error) {
	card, err := uc.repo.Get(ctx, cardID)
	if err != nil {
		return Invoice{}, err
	}
	today := todayFrom(uc.now)
	buckets, err := gatherBuckets(ctx, card, uc.reader, today)
	if err != nil {
		return Invoice{}, err
	}
	cycleTxns := buckets[reference]

	// compras pagas exatamente nessa data → voltam para pendente.
	var revertIDs []string
	for _, t := range cycleTxns {
		if t.Status == statusRealizado && t.PaymentDate != nil && *t.PaymentDate == paymentDate {
			revertIDs = append(revertIDs, t.ID)
		}
	}
	if len(revertIDs) == 0 {
		return Invoice{}, ErrPaymentNotFound
	}

	if err := uc.writer.RevertInvoicePayment(ctx, revertIDs); err != nil {
		return Invoice{}, domainerr.NewInternal("erro ao desfazer pagamento da fatura")
	}
	return BuildInvoice(reference, markPending(cycleTxns, revertIDs), card, today)
}

// markPending devolve uma cópia do ciclo com as compras revertidas para pendente.
func markPending(txns []shared.CardTransaction, revertIDs []string) []shared.CardTransaction {
	reverted := make(map[string]struct{}, len(revertIDs))
	for _, id := range revertIDs {
		reverted[id] = struct{}{}
	}
	out := make([]shared.CardTransaction, len(txns))
	copy(out, txns)
	for i := range out {
		if _, ok := reverted[out[i].ID]; ok {
			out[i].Status = statusPendente
			out[i].PaymentDate = nil
		}
	}
	return out
}

// ─── monthlyCardSummaryImpl ─────────────────────────────────────────────────

type monthlyCardSummaryImpl struct {
	repo   CreditCardRepository
	reader CardTransactionReader
}

func NewMonthlyCardSummary(repo CreditCardRepository, reader CardTransactionReader) MonthlyCardSummaryUseCase {
	return &monthlyCardSummaryImpl{repo: repo, reader: reader}
}

func (uc *monthlyCardSummaryImpl) Execute(ctx context.Context, cardID string, year, month int) (MonthlyCardSummary, error) {
	if _, err := uc.repo.Get(ctx, cardID); err != nil {
		return MonthlyCardSummary{}, err
	}
	from := fmt.Sprintf("%04d-%02d-01", year, month)
	to := fmt.Sprintf("%04d-%02d-%02d", year, month, daysInMonth(year, time.Month(month)))
	txns, err := uc.reader.ListByCard(ctx, cardID, from, to)
	if err != nil {
		return MonthlyCardSummary{}, domainerr.NewInternal("erro ao calcular resumo mensal")
	}
	return BuildMonthlySummary(cardID, year, month, txns), nil
}

// ─── Helpers de orquestração ────────────────────────────────────────────────

// gatherBuckets carrega todos os lançamentos e pagamentos do cartão, bucketiza por
// reference e garante que a reference do ciclo aberto exista (fatura aberta zerada).
func gatherBuckets(ctx context.Context, card CreditCard, reader CardTransactionReader, today string) (map[string][]shared.CardTransaction, error) {
	txns, err := reader.ListByCard(ctx, card.ID, wideFrom, wideTo)
	if err != nil {
		return nil, domainerr.NewInternal("erro ao ler lançamentos do cartão")
	}
	buckets, err := bucketByReference(txns, card.ClosingDay)
	if err != nil {
		return nil, err
	}
	openRef, err := InvoiceReferenceFor(today, card.ClosingDay)
	if err != nil {
		return nil, err
	}
	if _, ok := buckets[openRef]; !ok {
		buckets[openRef] = nil // fatura aberta zerada (UC-CC-01)
	}
	return buckets, nil
}

// buildDetail compõe o cartão com seus indicadores derivados.
func buildDetail(ctx context.Context, card CreditCard, reader CardTransactionReader, today string) (CreditCardDetail, error) {
	buckets, err := gatherBuckets(ctx, card, reader, today)
	if err != nil {
		return CreditCardDetail{}, err
	}
	used, err := UsedLimit(buckets, card, today)
	if err != nil {
		return CreditCardDetail{}, err
	}
	available := card.CreditLimit - used
	if available < 0 {
		available = 0
	}
	percent := UtilizationPercent(used, card.CreditLimit)

	openRef, err := InvoiceReferenceFor(today, card.ClosingDay)
	if err != nil {
		return CreditCardDetail{}, err
	}
	openInv, err := BuildInvoice(openRef, buckets[openRef], card, today)
	if err != nil {
		return CreditCardDetail{}, err
	}

	return CreditCardDetail{
		CreditCard:         card,
		BestPurchaseDay:    BestPurchaseDay(card.ClosingDay),
		UsedLimit:          used,
		AvailableLimit:     available,
		UtilizationPercent: percent,
		UtilizationLevel:   ClassifyUtilization(percent),
		OpenInvoice:        &openInv,
	}, nil
}

// buildInvoices monta todas as faturas a partir dos buckets, ordenadas por reference desc.
func buildInvoices(card CreditCard, buckets map[string][]shared.CardTransaction, today string) ([]Invoice, error) {
	refs := make([]string, 0, len(buckets))
	for ref := range buckets {
		refs = append(refs, ref)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(refs)))

	out := make([]Invoice, 0, len(refs))
	for _, ref := range refs {
		inv, err := BuildInvoice(ref, buckets[ref], card, today)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

// paginate fatia em memória os lançamentos do ciclo (D6).
func paginate(txns []shared.CardTransaction, p shared.Pagination) ([]shared.CardTransaction, shared.PagedMeta) {
	total := len(txns)
	totalPages := 1
	if p.Limit > 0 && total > 0 {
		totalPages = (total + p.Limit - 1) / p.Limit
	}
	start := p.Offset()
	if start > total {
		start = total
	}
	end := start + p.Limit
	if p.Limit <= 0 || end > total {
		end = total
	}
	page := append([]shared.CardTransaction{}, txns[start:end]...)
	return page, shared.PagedMeta{
		Page: p.Page, Limit: p.Limit, Total: total,
		TotalPages: totalPages, Sort: p.OrderBy, SortDir: p.Order,
	}
}

// isValidDate valida uma data YYYY-MM-DD.
func isValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
