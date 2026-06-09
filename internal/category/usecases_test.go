package category

import (
	"context"
	"testing"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Compile-time interface satisfaction checks ───────────────────────────────

var (
	_ ListCategoriesUseCase          = (*listCategoriesImpl)(nil)
	_ GetCategoryUseCase             = (*getCategoryImpl)(nil)
	_ CreateCategoryUseCase          = (*createCategoryImpl)(nil)
	_ UpdateCategoryUseCase          = (*updateCategoryImpl)(nil)
	_ DeleteCategoryUseCase          = (*deleteCategoryImpl)(nil)
	_ ListSubcategoriesUseCase       = (*listSubcategoriesImpl)(nil)
	_ ListSubcategoriesByTypeUseCase = (*listSubcategoriesByTypeImpl)(nil)
	_ GetSubcategoryUseCase          = (*getSubcategoryImpl)(nil)
	_ CreateSubcategoryUseCase       = (*createSubcategoryImpl)(nil)
	_ UpdateSubcategoryUseCase       = (*updateSubcategoryImpl)(nil)
	_ DeleteSubcategoryUseCase       = (*deleteSubcategoryImpl)(nil)
)

// ─── Fake repositories ────────────────────────────────────────────────────────

type fakeCatRepo struct {
	data     map[string]Category
	hasSubs  bool
	forceErr error
}

func newFakeCatRepo() *fakeCatRepo {
	return &fakeCatRepo{data: make(map[string]Category)}
}

func (f *fakeCatRepo) List(_ context.Context, _ CategoryFilter, _ shared.Pagination) ([]Category, int, error) {
	if f.forceErr != nil {
		return nil, 0, f.forceErr
	}
	cats := make([]Category, 0, len(f.data))
	for _, c := range f.data {
		cats = append(cats, c)
	}
	return cats, len(cats), nil
}

func (f *fakeCatRepo) Get(_ context.Context, id string) (Category, error) {
	if f.forceErr != nil {
		return Category{}, f.forceErr
	}
	c, ok := f.data[id]
	if !ok {
		return Category{}, ErrCategoryNotFound
	}
	return c, nil
}

func (f *fakeCatRepo) GetWithSubcategories(_ context.Context, id string) (CategoryWithSubs, error) {
	if f.forceErr != nil {
		return CategoryWithSubs{}, f.forceErr
	}
	c, ok := f.data[id]
	if !ok {
		return CategoryWithSubs{}, ErrCategoryNotFound
	}
	return CategoryWithSubs{Category: c, Subcategories: nil}, nil
}

func (f *fakeCatRepo) HasSubcategories(_ context.Context, _ string) (bool, error) {
	if f.forceErr != nil {
		return false, f.forceErr
	}
	return f.hasSubs, nil
}

func (f *fakeCatRepo) Create(_ context.Context, c Category) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[c.ID] = c
	return nil
}

func (f *fakeCatRepo) Update(_ context.Context, c Category) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[c.ID] = c
	return nil
}

func (f *fakeCatRepo) Delete(_ context.Context, id string) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	delete(f.data, id)
	return nil
}

// ─── Fake sub repository ──────────────────────────────────────────────────────

type fakeSubRepo struct {
	data     map[string]Subcategory
	forceErr error
}

func newFakeSubRepo() *fakeSubRepo {
	return &fakeSubRepo{data: make(map[string]Subcategory)}
}

func (f *fakeSubRepo) List(_ context.Context, _ string, _ shared.Pagination) ([]Subcategory, int, error) {
	if f.forceErr != nil {
		return nil, 0, f.forceErr
	}
	subs := make([]Subcategory, 0, len(f.data))
	for _, s := range f.data {
		subs = append(subs, s)
	}
	return subs, len(subs), nil
}

func (f *fakeSubRepo) ListAllByType(_ context.Context, _ CategoryType) ([]Subcategory, error) {
	if f.forceErr != nil {
		return nil, f.forceErr
	}
	subs := make([]Subcategory, 0, len(f.data))
	for _, s := range f.data {
		subs = append(subs, s)
	}
	return subs, nil
}

func (f *fakeSubRepo) Get(_ context.Context, id string) (Subcategory, error) {
	if f.forceErr != nil {
		return Subcategory{}, f.forceErr
	}
	s, ok := f.data[id]
	if !ok {
		return Subcategory{}, ErrSubcategoryNotFound
	}
	return s, nil
}

func (f *fakeSubRepo) Create(_ context.Context, s Subcategory) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[s.ID] = s
	return nil
}

func (f *fakeSubRepo) Update(_ context.Context, s Subcategory) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	f.data[s.ID] = s
	return nil
}

func (f *fakeSubRepo) Delete(_ context.Context, id string) error {
	if f.forceErr != nil {
		return f.forceErr
	}
	delete(f.data, id)
	return nil
}

// ─── Category use case tests ──────────────────────────────────────────────────

func TestCreateCategory_Success(t *testing.T) {
	repo := newFakeCatRepo()
	uc := NewCreateCategory(repo)

	c, err := uc.Execute(context.Background(), CreateCategoryInput{Name: "Moradia", Type: Expense})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID == "" {
		t.Error("expected non-empty ID")
	}
	if c.Name != "Moradia" {
		t.Errorf("name: got %q, want Moradia", c.Name)
	}
	if c.Type != Expense {
		t.Errorf("type: got %q, want %q", c.Type, Expense)
	}
	if !c.CanBeDeleted {
		t.Error("expected CanBeDeleted=true for user-created categories")
	}
}

func TestCreateCategory_InvalidInput(t *testing.T) {
	repo := newFakeCatRepo()
	uc := NewCreateCategory(repo)

	_, err := uc.Execute(context.Background(), CreateCategoryInput{Name: "", Type: Expense})
	if err == nil {
		t.Error("expected validation error, got nil")
	}
}

func TestUpdateCategory_Success(t *testing.T) {
	repo := newFakeCatRepo()
	existing := Category{ID: "cat-1", Name: "Antigo", Type: Expense, CanBeDeleted: true}
	repo.data["cat-1"] = existing

	uc := NewUpdateCategory(repo)
	updated, err := uc.Execute(context.Background(), UpdateCategoryInput{
		ID: "cat-1", Name: "Novo Nome", Icon: "icon.svg", Color: "#FFF",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "Novo Nome" {
		t.Errorf("name: got %q, want Novo Nome", updated.Name)
	}
	if updated.Icon != "icon.svg" {
		t.Errorf("icon: got %q, want icon.svg", updated.Icon)
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	uc := NewUpdateCategory(newFakeCatRepo())
	_, err := uc.Execute(context.Background(), UpdateCategoryInput{ID: "missing", Name: "x"})
	if err == nil {
		t.Error("expected error for missing category")
	}
}

func TestDeleteCategory_Success(t *testing.T) {
	repo := newFakeCatRepo()
	repo.data["cat-1"] = Category{ID: "cat-1", Name: "Test", CanBeDeleted: true}
	repo.hasSubs = false

	uc := NewDeleteCategory(repo)
	if err := uc.Execute(context.Background(), "cat-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := repo.data["cat-1"]; exists {
		t.Error("expected category to be deleted")
	}
}

func TestDeleteCategory_NotDeletable(t *testing.T) {
	repo := newFakeCatRepo()
	repo.data["cat-sys"] = Category{ID: "cat-sys", Name: "Sistema", CanBeDeleted: false}

	uc := NewDeleteCategory(repo)
	err := uc.Execute(context.Background(), "cat-sys")
	if err == nil {
		t.Error("expected conflict error, got nil")
	}
}

func TestDeleteCategory_HasSubcategories(t *testing.T) {
	repo := newFakeCatRepo()
	repo.data["cat-1"] = Category{ID: "cat-1", Name: "Test", CanBeDeleted: true}
	repo.hasSubs = true

	uc := NewDeleteCategory(repo)
	err := uc.Execute(context.Background(), "cat-1")
	if err == nil {
		t.Error("expected conflict error when category has subcategories")
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	uc := NewDeleteCategory(newFakeCatRepo())
	err := uc.Execute(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestListCategories_ReturnsPaged(t *testing.T) {
	repo := newFakeCatRepo()
	repo.data["c1"] = Category{ID: "c1", Name: "B"}
	repo.data["c2"] = Category{ID: "c2", Name: "A"}

	uc := NewListCategories(repo)
	p := shared.DefaultPagination()
	result, err := uc.Execute(context.Background(), ListCategoriesInput{Pagination: p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pagination.Total != 2 {
		t.Errorf("total: got %d, want 2", result.Pagination.Total)
	}
}

// ─── Subcategory use case tests ───────────────────────────────────────────────

func TestCreateSubcategory_Success(t *testing.T) {
	catRepo := newFakeCatRepo()
	catRepo.data["cat-1"] = Category{ID: "cat-1", Name: "Moradia", Type: Expense, CanBeDeleted: true}
	subRepo := newFakeSubRepo()

	uc := NewCreateSubcategory(catRepo, subRepo)
	s, err := uc.Execute(context.Background(), CreateSubcategoryInput{
		CategoryID: "cat-1",
		Name:       "Aluguel",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.CategoryID != "cat-1" {
		t.Errorf("category_id: got %q, want cat-1", s.CategoryID)
	}
}

func TestCreateSubcategory_CategoryNotFound(t *testing.T) {
	uc := NewCreateSubcategory(newFakeCatRepo(), newFakeSubRepo())
	_, err := uc.Execute(context.Background(), CreateSubcategoryInput{
		CategoryID: "missing",
		Name:       "Sub",
	})
	if err == nil {
		t.Error("expected error for missing category")
	}
}

func TestDeleteSubcategory_Success(t *testing.T) {
	repo := newFakeSubRepo()
	repo.data["sub-1"] = Subcategory{ID: "sub-1", Name: "Sub", CanBeDeleted: true}

	uc := NewDeleteSubcategory(repo)
	if err := uc.Execute(context.Background(), "sub-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSubcategory_NotDeletable(t *testing.T) {
	repo := newFakeSubRepo()
	repo.data["sub-sys"] = Subcategory{ID: "sub-sys", Name: "Sistema", CanBeDeleted: false}

	uc := NewDeleteSubcategory(repo)
	err := uc.Execute(context.Background(), "sub-sys")
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestListSubcategoriesByType_ReturnsSorted(t *testing.T) {
	repo := newFakeSubRepo()
	repo.data["s1"] = Subcategory{ID: "s1", Name: "Zebra"}
	repo.data["s2"] = Subcategory{ID: "s2", Name: "Aluguel"}

	uc := NewListSubcategoriesByType(repo)
	subs, err := uc.Execute(context.Background(), Expense)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subs, got %d", len(subs))
	}
	if subs[0].Name != "Aluguel" {
		t.Errorf("expected first item=Aluguel (sorted), got %q", subs[0].Name)
	}
}

func TestListSubcategories_CategoryNotFound(t *testing.T) {
	uc := NewListSubcategories(newFakeCatRepo(), newFakeSubRepo())
	_, err := uc.Execute(context.Background(), ListSubcategoriesInput{
		CategoryID: "missing",
		Pagination: shared.DefaultPagination(),
	})
	if err == nil {
		t.Error("expected error for missing category")
	}
}
