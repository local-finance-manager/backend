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
