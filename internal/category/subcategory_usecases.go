package category

import (
	"context"
	"sort"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/google/uuid"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── listSubcategoriesImpl ────────────────────────────────────────────────────

type listSubcategoriesImpl struct {
	catRepo CategoryRepository
	subRepo SubcategoryRepository
}

// NewListSubcategories returns a ListSubcategoriesUseCase implementation.
func NewListSubcategories(catRepo CategoryRepository, subRepo SubcategoryRepository) ListSubcategoriesUseCase {
	return &listSubcategoriesImpl{catRepo: catRepo, subRepo: subRepo}
}

func (uc *listSubcategoriesImpl) Execute(ctx context.Context, in ListSubcategoriesInput) (shared.PagedResult[Subcategory], error) {
	if _, err := uc.catRepo.Get(ctx, in.CategoryID); err != nil {
		return shared.PagedResult[Subcategory]{}, err
	}
	subs, total, err := uc.subRepo.List(ctx, in.CategoryID, in.Pagination)
	if err != nil {
		return shared.PagedResult[Subcategory]{}, domainerr.NewInternal("erro ao listar subcategorias")
	}
	return shared.NewPagedResult(subs, total, in.Pagination), nil
}

// ─── listSubcategoriesByTypeImpl ──────────────────────────────────────────────

type listSubcategoriesByTypeImpl struct {
	repo SubcategoryRepository
}

// NewListSubcategoriesByType returns a ListSubcategoriesByTypeUseCase implementation.
func NewListSubcategoriesByType(repo SubcategoryRepository) ListSubcategoriesByTypeUseCase {
	return &listSubcategoriesByTypeImpl{repo: repo}
}

func (uc *listSubcategoriesByTypeImpl) Execute(ctx context.Context, t CategoryType) ([]Subcategory, error) {
	subs, err := uc.repo.ListAllByType(ctx, t)
	if err != nil {
		return nil, domainerr.NewInternal("erro ao listar subcategorias por tipo")
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name < subs[j].Name })
	return subs, nil
}

// ─── getSubcategoryImpl ───────────────────────────────────────────────────────

type getSubcategoryImpl struct {
	repo SubcategoryRepository
}

// NewGetSubcategory returns a GetSubcategoryUseCase implementation.
func NewGetSubcategory(repo SubcategoryRepository) GetSubcategoryUseCase {
	return &getSubcategoryImpl{repo: repo}
}

func (uc *getSubcategoryImpl) Execute(ctx context.Context, id string) (Subcategory, error) {
	s, err := uc.repo.Get(ctx, id)
	if err != nil {
		return Subcategory{}, err
	}
	return s, nil
}

// ─── createSubcategoryImpl ────────────────────────────────────────────────────

type createSubcategoryImpl struct {
	catRepo CategoryRepository
	subRepo SubcategoryRepository
}

// NewCreateSubcategory returns a CreateSubcategoryUseCase implementation.
func NewCreateSubcategory(catRepo CategoryRepository, subRepo SubcategoryRepository) CreateSubcategoryUseCase {
	return &createSubcategoryImpl{catRepo: catRepo, subRepo: subRepo}
}

func (uc *createSubcategoryImpl) Execute(ctx context.Context, in CreateSubcategoryInput) (Subcategory, error) {
	if err := ValidateCreateSubcategory(in); err != nil {
		return Subcategory{}, err
	}
	if _, err := uc.catRepo.Get(ctx, in.CategoryID); err != nil {
		return Subcategory{}, err
	}
	now := time.Now().UTC()
	s := Subcategory{
		ID:           uuid.New().String(),
		CategoryID:   in.CategoryID,
		Name:         in.Name,
		Icon:         in.Icon,
		Color:        in.Color,
		CanBeDeleted: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := uc.subRepo.Create(ctx, s); err != nil {
		return Subcategory{}, domainerr.NewInternal("erro ao criar subcategoria")
	}
	return s, nil
}

// ─── updateSubcategoryImpl ────────────────────────────────────────────────────

type updateSubcategoryImpl struct {
	repo SubcategoryRepository
}

// NewUpdateSubcategory returns an UpdateSubcategoryUseCase implementation.
func NewUpdateSubcategory(repo SubcategoryRepository) UpdateSubcategoryUseCase {
	return &updateSubcategoryImpl{repo: repo}
}

func (uc *updateSubcategoryImpl) Execute(ctx context.Context, in UpdateSubcategoryInput) (Subcategory, error) {
	if err := ValidateUpdateSubcategory(in); err != nil {
		return Subcategory{}, err
	}
	s, err := uc.repo.Get(ctx, in.ID)
	if err != nil {
		return Subcategory{}, err
	}
	s.Name = in.Name
	s.Icon = in.Icon
	s.Color = in.Color
	s.UpdatedAt = time.Now().UTC()
	if err := uc.repo.Update(ctx, s); err != nil {
		return Subcategory{}, domainerr.NewInternal("erro ao atualizar subcategoria")
	}
	return s, nil
}

// ─── deleteSubcategoryImpl ────────────────────────────────────────────────────

type deleteSubcategoryImpl struct {
	repo SubcategoryRepository
}

// NewDeleteSubcategory returns a DeleteSubcategoryUseCase implementation.
func NewDeleteSubcategory(repo SubcategoryRepository) DeleteSubcategoryUseCase {
	return &deleteSubcategoryImpl{repo: repo}
}

func (uc *deleteSubcategoryImpl) Execute(ctx context.Context, id string) error {
	s, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !s.CanBeDeleted {
		return ErrSubcategoryNotDeletable
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return domainerr.NewInternal("erro ao excluir subcategoria")
	}
	return nil
}
