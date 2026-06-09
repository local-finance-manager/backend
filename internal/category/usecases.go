package category

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Category use case interfaces ────────────────────────────────────────────

// ListCategoriesInput is the input for listing categories with optional filter and pagination.
type ListCategoriesInput struct {
	Filter     CategoryFilter
	Pagination shared.Pagination
}

// ListSubcategoriesInput is the input for listing subcategories of a given category.
type ListSubcategoriesInput struct {
	CategoryID string
	Pagination shared.Pagination
}

// ListCategoriesUseCase returns a paginated list of categories.
type ListCategoriesUseCase interface {
	Execute(ctx context.Context, in ListCategoriesInput) (shared.PagedResult[Category], error)
}

// GetCategoryUseCase returns a single category with its subcategories.
type GetCategoryUseCase interface {
	Execute(ctx context.Context, id string) (CategoryWithSubs, error)
}

// CreateCategoryUseCase creates and persists a new category.
type CreateCategoryUseCase interface {
	Execute(ctx context.Context, in CreateCategoryInput) (Category, error)
}

// UpdateCategoryUseCase updates mutable fields of an existing category.
type UpdateCategoryUseCase interface {
	Execute(ctx context.Context, in UpdateCategoryInput) (Category, error)
}

// DeleteCategoryUseCase removes a category, enforcing business constraints.
type DeleteCategoryUseCase interface {
	Execute(ctx context.Context, id string) error
}

// ─── Subcategory use case interfaces ─────────────────────────────────────────

// ListSubcategoriesUseCase returns a paginated list of subcategories for a category.
type ListSubcategoriesUseCase interface {
	Execute(ctx context.Context, in ListSubcategoriesInput) (shared.PagedResult[Subcategory], error)
}

// ListSubcategoriesByTypeUseCase returns all subcategories whose parent type matches.
// No pagination — designed for populating UI select lists.
type ListSubcategoriesByTypeUseCase interface {
	Execute(ctx context.Context, t CategoryType) ([]Subcategory, error)
}

// GetSubcategoryUseCase returns a single subcategory by ID.
type GetSubcategoryUseCase interface {
	Execute(ctx context.Context, id string) (Subcategory, error)
}

// CreateSubcategoryUseCase creates and persists a new subcategory.
type CreateSubcategoryUseCase interface {
	Execute(ctx context.Context, in CreateSubcategoryInput) (Subcategory, error)
}

// UpdateSubcategoryUseCase updates mutable fields of an existing subcategory.
type UpdateSubcategoryUseCase interface {
	Execute(ctx context.Context, in UpdateSubcategoryInput) (Subcategory, error)
}

// DeleteSubcategoryUseCase removes a subcategory, enforcing business constraints.
type DeleteSubcategoryUseCase interface {
	Execute(ctx context.Context, id string) error
}
