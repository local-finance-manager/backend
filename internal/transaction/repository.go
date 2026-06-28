package transaction

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// TransactionRepository defines persistence operations for Transaction aggregates.
// Defined at the consumer site (use cases) following ISP.
type TransactionRepository interface {
	Get(ctx context.Context, id string) (TransactionDetail, error)
	List(ctx context.Context, f TransactionFilter, p shared.Pagination) ([]TransactionDetail, error)
	GetSummary(ctx context.Context, f TransactionFilter) (Summary, error)
	Create(ctx context.Context, t Transaction) error
	Update(ctx context.Context, t Transaction) error
	Delete(ctx context.Context, id string) error
}

// SubcategoryFacade isolates the cross-module call the service needs to derive
// the type of a subcategory. The interface is defined here (consumer); implemented
// by category.SubcategoryFacade (producer) and injected via main.go.
// Returns a primitive string to avoid type coupling with the category package.
type SubcategoryFacade interface {
	GetSubcategoryType(ctx context.Context, subcategoryID string) (string, error)
}

// MonthGuard protege meses fechados (módulo report): bloqueia alterações em mês
// fechado-bloqueado (≥90 dias) e recalcula o snapshot de meses fechados-ajustáveis
// após uma alteração. Interface definida aqui (consumidor); implementada por
// report.Service e injetada no main.go. Pode ser nil (sem report → sem guarda).
type MonthGuard interface {
	EnsureEditable(ctx context.Context, competenceDate string) error
	AfterChange(ctx context.Context, competenceDates ...string) error
}

// CreditCardChecker valida que um cartão pode receber vínculo de lançamento.
// Interface definida no consumidor (transaction); implementada por um facade do
// módulo creditcard e injetada no main.go. Retorna apenas error (domainerr),
// sem acoplamento de tipo entre os módulos.
type CreditCardChecker interface {
	// CheckLinkable retorna nil se o cartão existe e está ativo; erro de domínio
	// (não encontrado / arquivado) caso contrário.
	CheckLinkable(ctx context.Context, cardID string) error
}
