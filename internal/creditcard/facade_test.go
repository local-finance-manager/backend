package creditcard

import (
	"context"
	"testing"
)

func TestCreditCardChecker_CheckLinkable(t *testing.T) {
	repo := newFakeCardRepo()
	seedCard(repo, "active", false)
	seedCard(repo, "archived", true)
	checker := NewCreditCardChecker(repo)
	ctx := context.Background()

	if err := checker.CheckLinkable(ctx, "active"); err != nil {
		t.Errorf("cartão ativo deveria ser linkável, got %v", err)
	}
	if err := checker.CheckLinkable(ctx, "archived"); err != ErrCannotLinkArchivedCard {
		t.Errorf("cartão arquivado: got %v, want ErrCannotLinkArchivedCard", err)
	}
	if err := checker.CheckLinkable(ctx, "ghost"); err != ErrCreditCardNotFound {
		t.Errorf("cartão inexistente: got %v, want ErrCreditCardNotFound", err)
	}
}
