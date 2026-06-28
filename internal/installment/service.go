package installment

import (
	"context"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Deps são as dependências do serviço (injetadas no main.go).
type Deps struct {
	Repo  Repository
	Subs  SubcategoryReader
	Cards CreditCardChecker
	Refs  InvoiceReferenceResolver
}

// Service orquestra os casos de uso de compras parceladas.
type Service struct {
	repo  Repository
	subs  SubcategoryReader
	cards CreditCardChecker
	refs  InvoiceReferenceResolver
	now   func() time.Time
}

// NewService cria o serviço.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, subs: d.Subs, cards: d.Cards, refs: d.Refs, now: time.Now}
}

// Preview calcula o cronograma SEM persistir (RF-PARC-03).
func (s *Service) Preview(ctx context.Context, in CreateInput) (Plan, error) {
	total, planned, interest, err := s.buildPlan(ctx, in)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		TotalAmount:       total,
		InstallmentsCount: in.InstallmentsCount,
		InterestAmount:    interest,
		Installments:      planned,
	}, nil
}

// Create gera o grupo + N parcelas atomicamente (RF-PARC-01/05).
func (s *Service) Create(ctx context.Context, in CreateInput) (GroupDetail, error) {
	total, planned, interest, err := s.buildPlan(ctx, in)
	if err != nil {
		return GroupDetail{}, err
	}

	now := s.now().UTC()
	g := InstallmentGroup{
		ID:                uuid.New().String(),
		CreditCardID:      in.CreditCardID,
		SubcategoryID:     in.SubcategoryID,
		Title:             in.Title,
		Description:       in.Description,
		TotalAmount:       total,
		PrincipalAmount:   in.PrincipalAmount,
		InstallmentsCount: in.InstallmentsCount,
		PurchaseDate:      in.PurchaseDate,
		FirstReference:    planned[0].Reference,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	parcelas := make([]Parcela, len(planned))
	installments := make([]Installment, len(planned))
	for i, pl := range planned {
		id := uuid.New().String()
		parcelas[i] = Parcela{ID: id, Number: pl.Number, Amount: pl.Amount, CompetenceDate: pl.CompetenceDate}
		installments[i] = Installment{
			TransactionID: id, Number: pl.Number, Amount: pl.Amount,
			CompetenceDate: pl.CompetenceDate, Reference: pl.Reference, Status: parcelaPending,
		}
	}

	if err := s.repo.Create(ctx, g, parcelas); err != nil {
		return GroupDetail{}, domainerr.NewInternal("erro ao criar compra parcelada")
	}

	return GroupDetail{
		InstallmentGroup: g,
		InterestAmount:   interest,
		PaidCount:        0,
		RemainingCount:   len(planned),
		RemainingAmount:  total,
		Status:           GroupActive,
		Installments:     installments,
	}, nil
}

// List devolve os grupos paginados (RF-PARC-06).
func (s *Service) List(ctx context.Context, f Filter, p shared.Pagination) (shared.PagedResult[GroupSummary], error) {
	groups, total, err := s.repo.List(ctx, f, p)
	if err != nil {
		return shared.PagedResult[GroupSummary]{}, domainerr.NewInternal("erro ao listar compras parceladas")
	}
	return shared.NewPagedResult(groups, total, p), nil
}

// Get devolve o detalhe do grupo com as parcelas e indicadores derivados.
func (s *Service) Get(ctx context.Context, id string) (GroupDetail, error) {
	g, parcelas, err := s.repo.Get(ctx, id)
	if err != nil {
		return GroupDetail{}, err
	}
	return s.composeDetail(ctx, g, parcelas)
}

// UpdateSeries edita os metadados da série (RF-PARC-07).
func (s *Service) UpdateSeries(ctx context.Context, in UpdateSeriesInput) (GroupDetail, error) {
	if err := ValidateUpdateSeries(in); err != nil {
		return GroupDetail{}, err
	}
	typ, err := s.subs.GetSubcategoryType(ctx, in.SubcategoryID)
	if err != nil {
		return GroupDetail{}, err
	}
	if typ != expenseType {
		return GroupDetail{}, ErrOnlyExpensesInstallable
	}
	if err := s.repo.UpdateSeries(ctx, in.ID, in.Title, in.Description, in.SubcategoryID, typ); err != nil {
		return GroupDetail{}, err
	}
	return s.Get(ctx, in.ID)
}

// CancelRemaining cancela as parcelas pendentes, preservando as pagas (RF-PARC-08).
func (s *Service) CancelRemaining(ctx context.Context, id string) (GroupDetail, error) {
	if _, _, err := s.repo.Get(ctx, id); err != nil {
		return GroupDetail{}, err
	}
	if _, err := s.repo.CancelRemaining(ctx, id); err != nil {
		return GroupDetail{}, domainerr.NewInternal("erro ao cancelar parcelas")
	}
	return s.Get(ctx, id)
}

// Delete remove a compra inteira e suas parcelas (cascade — RF-PARC-09), MAS só quando
// nenhuma parcela já foi paga (realizado). Se houver parcela paga, excluir apagaria
// histórico financeiro real → bloqueia e orienta a cancelar as restantes (as pagas
// são preservadas). "Cancelar restantes" só mexe nas pendentes (abertas).
func (s *Service) Delete(ctx context.Context, id string) error {
	_, parcelas, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	for _, p := range parcelas {
		if p.Status == parcelaRealized {
			return ErrInstallmentHasPaidParcelas
		}
	}
	return s.repo.Delete(ctx, id)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// buildPlan valida e calcula o plano de parcelas (compartilhado por Preview e Create).
func (s *Service) buildPlan(ctx context.Context, in CreateInput) (int64, []PlannedInstallment, int64, error) {
	if err := ValidateCreate(in); err != nil {
		return 0, nil, 0, err
	}
	if err := s.cards.CheckLinkable(ctx, in.CreditCardID); err != nil {
		return 0, nil, 0, err // cartão inexistente/arquivado (erro de domínio do creditcard)
	}
	typ, err := s.subs.GetSubcategoryType(ctx, in.SubcategoryID)
	if err != nil {
		return 0, nil, 0, err
	}
	if typ != expenseType {
		return 0, nil, 0, ErrOnlyExpensesInstallable
	}

	total, amounts, err := ResolvePlan(in.InputMode, in.TotalAmount, in.InstallmentAmount, in.InstallmentsCount)
	if err != nil {
		return 0, nil, 0, err
	}
	dates, err := CompetenceSchedule(in.PurchaseDate, in.InstallmentsCount)
	if err != nil {
		return 0, nil, 0, err
	}
	references, err := s.refs.ReferencesFor(ctx, in.CreditCardID, dates)
	if err != nil {
		return 0, nil, 0, err
	}

	planned := make([]PlannedInstallment, in.InstallmentsCount)
	for k := 0; k < in.InstallmentsCount; k++ {
		planned[k] = PlannedInstallment{
			Number: k + 1, Amount: amounts[k], CompetenceDate: dates[k], Reference: references[k],
		}
	}
	return total, planned, Interest(total, in.PrincipalAmount), nil
}

// composeDetail monta o GroupDetail a partir do grupo + parcelas lidas, resolvendo
// as references e derivando contagens/saldo/status.
func (s *Service) composeDetail(ctx context.Context, g InstallmentGroup, parcelas []Installment) (GroupDetail, error) {
	dates := make([]string, len(parcelas))
	for i, p := range parcelas {
		dates[i] = p.CompetenceDate
	}
	references, err := s.refs.ReferencesFor(ctx, g.CreditCardID, dates)
	if err != nil {
		return GroupDetail{}, err
	}

	var counts StatusCounts
	var remaining int64
	out := make([]Installment, len(parcelas))
	for i, p := range parcelas {
		p.Reference = references[i]
		out[i] = p
		switch p.Status {
		case parcelaPending:
			counts.Pending++
			remaining += p.Amount
		case parcelaRealized:
			counts.Realized++
		case parcelaCancelled:
			counts.Cancelled++
		}
	}

	return GroupDetail{
		InstallmentGroup: g,
		InterestAmount:   Interest(g.TotalAmount, g.PrincipalAmount),
		PaidCount:        counts.Realized,
		RemainingCount:   counts.Pending,
		RemainingAmount:  remaining,
		Status:           DeriveGroupStatus(counts),
		Installments:     out,
	}, nil
}
