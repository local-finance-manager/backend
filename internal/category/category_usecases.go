package category

import (
	"context"
	"sort"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── listCategoriesImpl ───────────────────────────────────────────────────────

type listCategoriesImpl struct {
	repo CategoryRepository
}

// NewListCategories returns a ListCategoriesUseCase implementation.
func NewListCategories(repo CategoryRepository) ListCategoriesUseCase {
	return &listCategoriesImpl{repo: repo}
}

func (uc *listCategoriesImpl) Execute(ctx context.Context, in ListCategoriesInput) (shared.PagedResult[Category], error) {
	cats, total, err := uc.repo.List(ctx, in.Filter, in.Pagination)
	if err != nil {
		return shared.PagedResult[Category]{}, domainerr.NewInternal("erro ao listar categorias")
	}
	// Secondary sort for correct pt-BR collation (SQL sorts by byte value).
	sort.Slice(cats, func(i, j int) bool { return cats[i].Name < cats[j].Name })
	return shared.NewPagedResult(cats, total, in.Pagination), nil
}

// ─── getCategoryImpl ──────────────────────────────────────────────────────────

type getCategoryImpl struct {
	repo CategoryRepository
}

// NewGetCategory returns a GetCategoryUseCase implementation.
func NewGetCategory(repo CategoryRepository) GetCategoryUseCase {
	return &getCategoryImpl{repo: repo}
}

func (uc *getCategoryImpl) Execute(ctx context.Context, id string) (CategoryWithSubs, error) {
	cws, err := uc.repo.GetWithSubcategories(ctx, id)
	if err != nil {
		return CategoryWithSubs{}, err
	}
	return cws, nil
}

// ─── createCategoryImpl ───────────────────────────────────────────────────────

type createCategoryImpl struct {
	repo CategoryRepository
}

// NewCreateCategory returns a CreateCategoryUseCase implementation.
func NewCreateCategory(repo CategoryRepository) CreateCategoryUseCase {
	return &createCategoryImpl{repo: repo}
}

func (uc *createCategoryImpl) Execute(ctx context.Context, in CreateCategoryInput) (Category, error) {
	if err := ValidateCreateCategory(in); err != nil {
		return Category{}, err
	}
	now := time.Now().UTC()
	c := Category{
		ID:           uuid.New().String(),
		Name:         in.Name,
		Type:         in.Type,
		Icon:         in.Icon,
		Color:        in.Color,
		CanBeDeleted: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := uc.repo.Create(ctx, c); err != nil {
		return Category{}, domainerr.NewInternal("erro ao criar categoria")
	}
	return c, nil
}

// ─── updateCategoryImpl ───────────────────────────────────────────────────────

type updateCategoryImpl struct {
	repo CategoryRepository
}

// NewUpdateCategory returns an UpdateCategoryUseCase implementation.
func NewUpdateCategory(repo CategoryRepository) UpdateCategoryUseCase {
	return &updateCategoryImpl{repo: repo}
}

func (uc *updateCategoryImpl) Execute(ctx context.Context, in UpdateCategoryInput) (Category, error) {
	if err := ValidateUpdateCategory(in); err != nil {
		return Category{}, err
	}
	c, err := uc.repo.Get(ctx, in.ID)
	if err != nil {
		return Category{}, err
	}
	c.Name = in.Name
	c.Icon = in.Icon
	c.Color = in.Color
	c.UpdatedAt = time.Now().UTC()
	if err := uc.repo.Update(ctx, c); err != nil {
		return Category{}, domainerr.NewInternal("erro ao atualizar categoria")
	}
	return c, nil
}

// ─── deleteCategoryImpl ───────────────────────────────────────────────────────

type deleteCategoryImpl struct {
	repo CategoryRepository
}

// NewDeleteCategory returns a DeleteCategoryUseCase implementation.
func NewDeleteCategory(repo CategoryRepository) DeleteCategoryUseCase {
	return &deleteCategoryImpl{repo: repo}
}

func (uc *deleteCategoryImpl) Execute(ctx context.Context, id string) error {
	c, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !c.CanBeDeleted {
		return ErrCategoryNotDeletable
	}
	hasSubs, err := uc.repo.HasSubcategories(ctx, id)
	if err != nil {
		return domainerr.NewInternal("erro ao verificar subcategorias")
	}
	if hasSubs {
		return ErrCategoryHasSubs
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return domainerr.NewInternal("erro ao excluir categoria")
	}
	return nil
}
