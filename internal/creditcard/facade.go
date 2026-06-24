package creditcard

import "context"

// CreditCardChecker satisfaz transaction.CreditCardChecker via structural typing (D7):
// o módulo transaction declara a interface; este struct a implementa e é injetado no
// main.go. Retorna apenas `error` (domainerr), sem acoplamento de tipo entre os módulos.
type CreditCardChecker struct {
	repo CreditCardRepository
}

// NewCreditCardChecker cria um CreditCardChecker.
func NewCreditCardChecker(repo CreditCardRepository) *CreditCardChecker {
	return &CreditCardChecker{repo: repo}
}

// CheckLinkable retorna nil se o cartão existe e está ativo; erro de domínio caso
// contrário (não encontrado ou arquivado).
func (c *CreditCardChecker) CheckLinkable(ctx context.Context, cardID string) error {
	card, err := c.repo.Get(ctx, cardID)
	if err != nil {
		return err // ErrCreditCardNotFound (domainerr) vindo do repo
	}
	if card.Archived {
		return ErrCannotLinkArchivedCard
	}
	return nil
}

// InvoiceReferenceFacade satisfaz installment.InvoiceReferenceResolver via structural
// typing: resolve a reference (YYYY-MM) da fatura de cada competência reusando o ciclo
// do cartão. Busca o cartão 1× e mapeia InvoiceReferenceFor sobre as datas.
type InvoiceReferenceFacade struct {
	repo CreditCardRepository
}

// NewInvoiceReferenceFacade cria um InvoiceReferenceFacade.
func NewInvoiceReferenceFacade(repo CreditCardRepository) *InvoiceReferenceFacade {
	return &InvoiceReferenceFacade{repo: repo}
}

// ReferencesFor resolve a reference de fatura de cada data (YYYY-MM-DD) para o cartão.
func (f *InvoiceReferenceFacade) ReferencesFor(ctx context.Context, cardID string, dates []string) ([]string, error) {
	card, err := f.repo.Get(ctx, cardID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(dates))
	for i, d := range dates {
		ref, err := InvoiceReferenceFor(d, card.ClosingDay)
		if err != nil {
			return nil, err
		}
		out[i] = ref
	}
	return out, nil
}
