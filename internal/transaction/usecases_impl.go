package transaction

import (
	"context"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── getTransactionImpl ───────────────────────────────────────────────────────

type getTransactionImpl struct{ repo TransactionRepository }

func NewGetTransaction(repo TransactionRepository) GetTransactionUseCase {
	return &getTransactionImpl{repo: repo}
}

func (uc *getTransactionImpl) Execute(ctx context.Context, id string) (TransactionDetail, error) {
	return uc.repo.Get(ctx, id)
}

// ─── listTransactionsImpl ─────────────────────────────────────────────────────

type listTransactionsImpl struct{ repo TransactionRepository }

func NewListTransactions(repo TransactionRepository) ListTransactionsUseCase {
	return &listTransactionsImpl{repo: repo}
}

func (uc *listTransactionsImpl) Execute(ctx context.Context, in ListTransactionsInput) (ListTransactionsResult, error) {
	data, err := uc.repo.List(ctx, in.Filter, in.Pagination)
	if err != nil {
		return ListTransactionsResult{}, domainerr.NewInternal("erro ao listar lançamentos")
	}
	summary, err := uc.repo.GetSummary(ctx, in.Filter)
	if err != nil {
		return ListTransactionsResult{}, domainerr.NewInternal("erro ao calcular resumo financeiro")
	}

	p := in.Pagination
	totalPages := 1
	if p.Limit > 0 && summary.CountTotal > 0 {
		totalPages = (summary.CountTotal + p.Limit - 1) / p.Limit
	}

	return ListTransactionsResult{
		Data:    data,
		Summary: summary,
		Pagination: shared.PagedMeta{
			Page:       p.Page,
			Limit:      p.Limit,
			Total:      summary.CountTotal,
			TotalPages: totalPages,
			Sort:       p.OrderBy,
			SortDir:    p.Order,
		},
	}, nil
}

// ─── createTransactionImpl ────────────────────────────────────────────────────

type createTransactionImpl struct {
	repo        TransactionRepository
	facade      SubcategoryFacade
	cardChecker CreditCardChecker
}

func NewCreateTransaction(repo TransactionRepository, facade SubcategoryFacade, cardChecker CreditCardChecker) CreateTransactionUseCase {
	return &createTransactionImpl{repo: repo, facade: facade, cardChecker: cardChecker}
}

func (uc *createTransactionImpl) Execute(ctx context.Context, in CreateTransactionInput) (TransactionDetail, error) {
	if err := ValidateCreate(in); err != nil {
		return TransactionDetail{}, err
	}

	if in.CreditCardID != nil {
		if err := uc.cardChecker.CheckLinkable(ctx, *in.CreditCardID); err != nil {
			return TransactionDetail{}, err
		}
	}

	typeStr, err := uc.facade.GetSubcategoryType(ctx, in.SubcategoryID)
	if err != nil {
		return TransactionDetail{}, err
	}

	now := time.Now().UTC()
	t := Transaction{
		ID:                   uuid.New().String(),
		Title:                in.Title,
		Description:          in.Description,
		Amount:               in.Amount,
		Type:                 TransactionType(typeStr),
		SubcategoryID:        in.SubcategoryID,
		PaymentMethod:        in.PaymentMethod,
		Status:               in.Status,
		CompetenceDate:       in.CompetenceDate,
		PaymentDate:          in.PaymentDate,
		AccountID:            in.AccountID,
		DestinationAccountID: in.DestinationAccountID,
		CreditCardID:         in.CreditCardID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := uc.repo.Create(ctx, t); err != nil {
		return TransactionDetail{}, domainerr.NewInternal("erro ao criar lançamento")
	}
	return uc.repo.Get(ctx, t.ID)
}

// ─── updateTransactionImpl ────────────────────────────────────────────────────

type updateTransactionImpl struct {
	repo        TransactionRepository
	facade      SubcategoryFacade
	cardChecker CreditCardChecker
}

func NewUpdateTransaction(repo TransactionRepository, facade SubcategoryFacade, cardChecker CreditCardChecker) UpdateTransactionUseCase {
	return &updateTransactionImpl{repo: repo, facade: facade, cardChecker: cardChecker}
}

func (uc *updateTransactionImpl) Execute(ctx context.Context, in UpdateTransactionInput) (TransactionDetail, error) {
	if err := ValidateUpdate(in); err != nil {
		return TransactionDetail{}, err
	}

	current, err := uc.repo.Get(ctx, in.ID)
	if err != nil {
		return TransactionDetail{}, err
	}

	// Status transitions are enforced unless the incoming status equals current.
	if in.Status != current.Status && !CanTransitionTo(current.Status, in.Status) {
		return TransactionDetail{}, ErrInvalidTransition(current.Status, in.Status)
	}

	if in.CreditCardID != nil {
		if err := uc.cardChecker.CheckLinkable(ctx, *in.CreditCardID); err != nil {
			return TransactionDetail{}, err
		}
	}

	// Type is derived from the subcategory; re-derive only when subcategory changed.
	newType := current.Type
	if in.SubcategoryID != current.SubcategoryID {
		typeStr, err := uc.facade.GetSubcategoryType(ctx, in.SubcategoryID)
		if err != nil {
			return TransactionDetail{}, err
		}
		newType = TransactionType(typeStr)
	}

	t := Transaction{
		ID:                   in.ID,
		Title:                in.Title,
		Description:          in.Description,
		Amount:               in.Amount,
		Type:                 newType,
		SubcategoryID:        in.SubcategoryID,
		PaymentMethod:        in.PaymentMethod,
		Status:               in.Status,
		CompetenceDate:       in.CompetenceDate,
		PaymentDate:          in.PaymentDate,
		AccountID:            in.AccountID,
		DestinationAccountID: in.DestinationAccountID,
		CreditCardID:         in.CreditCardID,
		CreatedAt:            current.CreatedAt,
		UpdatedAt:            time.Now().UTC(),
	}

	if err := uc.repo.Update(ctx, t); err != nil {
		return TransactionDetail{}, domainerr.NewInternal("erro ao atualizar lançamento")
	}
	return uc.repo.Get(ctx, in.ID)
}

// ─── confirmTransactionImpl ───────────────────────────────────────────────────

type confirmTransactionImpl struct{ repo TransactionRepository }

func NewConfirmTransaction(repo TransactionRepository) ConfirmTransactionUseCase {
	return &confirmTransactionImpl{repo: repo}
}

func (uc *confirmTransactionImpl) Execute(ctx context.Context, in ConfirmTransactionInput) (TransactionDetail, error) {
	if err := ValidateConfirm(in); err != nil {
		return TransactionDetail{}, err
	}

	current, err := uc.repo.Get(ctx, in.ID)
	if err != nil {
		return TransactionDetail{}, err
	}

	if !CanTransitionTo(current.Status, StatusRealizado) {
		return TransactionDetail{}, ErrInvalidTransition(current.Status, StatusRealizado)
	}

	t := current.Transaction
	t.Status = StatusRealizado
	t.PaymentDate = &in.PaymentDate
	t.UpdatedAt = time.Now().UTC()

	if err := uc.repo.Update(ctx, t); err != nil {
		return TransactionDetail{}, domainerr.NewInternal("erro ao confirmar lançamento")
	}
	return uc.repo.Get(ctx, in.ID)
}

// ─── cancelTransactionImpl ────────────────────────────────────────────────────

type cancelTransactionImpl struct{ repo TransactionRepository }

func NewCancelTransaction(repo TransactionRepository) CancelTransactionUseCase {
	return &cancelTransactionImpl{repo: repo}
}

func (uc *cancelTransactionImpl) Execute(ctx context.Context, id string) (TransactionDetail, error) {
	current, err := uc.repo.Get(ctx, id)
	if err != nil {
		return TransactionDetail{}, err
	}
	if current.Status == StatusCancelado {
		return current, nil // já cancelado: idempotente
	}
	if !CanTransitionTo(current.Status, StatusCancelado) {
		return TransactionDetail{}, ErrInvalidTransition(current.Status, StatusCancelado)
	}

	t := current.Transaction
	t.Status = StatusCancelado
	t.PaymentDate = nil // cancelado nunca tem data de pagamento
	t.UpdatedAt = time.Now().UTC()

	if err := uc.repo.Update(ctx, t); err != nil {
		return TransactionDetail{}, domainerr.NewInternal("erro ao cancelar lançamento")
	}
	return uc.repo.Get(ctx, id)
}

// ─── deleteTransactionImpl ────────────────────────────────────────────────────

type deleteTransactionImpl struct{ repo TransactionRepository }

func NewDeleteTransaction(repo TransactionRepository) DeleteTransactionUseCase {
	return &deleteTransactionImpl{repo: repo}
}

func (uc *deleteTransactionImpl) Execute(ctx context.Context, id string) error {
	if _, err := uc.repo.Get(ctx, id); err != nil {
		return err
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return domainerr.NewInternal("erro ao excluir lançamento")
	}
	return nil
}
