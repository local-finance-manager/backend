package installment

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Repository persiste grupos de parcelamento e suas parcelas (linhas em transactions).
// A geração grupo + N parcelas é atômica (uma tx) — ver sqlite.go.
type Repository interface {
	Create(ctx context.Context, g InstallmentGroup, parcelas []Parcela) error
	Get(ctx context.Context, id string) (InstallmentGroup, []Installment, error)
	List(ctx context.Context, f Filter, p shared.Pagination) ([]GroupSummary, int, error)
	UpdateSeries(ctx context.Context, id, title string, description *string, subcategoryID, parcelaType string) error
	CancelRemaining(ctx context.Context, id string) (cancelled int, err error)
	Delete(ctx context.Context, id string) error
}

// SubcategoryReader deriva o type da subcategoria (valida despesa e define o type da
// parcela). Implementado por category.SubcategoryFacade (reuso); injetado no main.go.
type SubcategoryReader interface {
	GetSubcategoryType(ctx context.Context, subcategoryID string) (string, error)
}

// CreditCardChecker valida que o cartão existe e não está arquivado.
// Implementado por creditcard.CreditCardChecker (reuso); injetado no main.go.
type CreditCardChecker interface {
	CheckLinkable(ctx context.Context, cardID string) error
}

// InvoiceReferenceResolver resolve a reference (YYYY-MM) da fatura de cada competência,
// reusando o ciclo do cartão. Implementado por creditcard.InvoiceReferenceFacade.
type InvoiceReferenceResolver interface {
	ReferencesFor(ctx context.Context, cardID string, dates []string) ([]string, error)
}
