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
