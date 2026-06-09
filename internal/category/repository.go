package category

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CategoryRepository defines persistence operations for Category aggregates.
// Defined at the consumer site (use cases) following ISP.
type CategoryRepository interface {
	List(ctx context.Context, f CategoryFilter, p shared.Pagination) ([]Category, int, error)
	Get(ctx context.Context, id string) (Category, error)
	GetWithSubcategories(ctx context.Context, id string) (CategoryWithSubs, error)
	HasSubcategories(ctx context.Context, id string) (bool, error)
	Create(ctx context.Context, c Category) error
	Update(ctx context.Context, c Category) error
	Delete(ctx context.Context, id string) error
}

// SubcategoryRepository defines persistence operations for Subcategory aggregates.
// Defined at the consumer site (use cases) following ISP.
type SubcategoryRepository interface {
	List(ctx context.Context, categoryID string, p shared.Pagination) ([]Subcategory, int, error)
	ListAllByType(ctx context.Context, t CategoryType) ([]Subcategory, error)
	Get(ctx context.Context, id string) (Subcategory, error)
	Create(ctx context.Context, s Subcategory) error
	Update(ctx context.Context, s Subcategory) error
	Delete(ctx context.Context, id string) error
}
