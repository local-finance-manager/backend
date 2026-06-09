package category_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/category"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Test DB setup ────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS categories (
		id             TEXT PRIMARY KEY,
		name           TEXT NOT NULL,
		type           TEXT NOT NULL CHECK(type IN ('despesa','receita','transferencia')),
		icon           TEXT,
		color          TEXT,
		can_be_deleted INTEGER NOT NULL DEFAULT 1,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create categories: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS subcategories (
		id             TEXT PRIMARY KEY,
		category_id    TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
		name           TEXT NOT NULL,
		icon           TEXT,
		color          TEXT,
		can_be_deleted INTEGER NOT NULL DEFAULT 1,
		created_at     TEXT NOT NULL,
		updated_at     TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create subcategories: %v", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_sub_cat ON subcategories(category_id)`)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	return db
}

func mkCat(id, name string, t category.CategoryType, deletable bool) category.Category {
	now := time.Now().UTC()
	return category.Category{
		ID: id, Name: name, Type: t,
		CanBeDeleted: deletable, CreatedAt: now, UpdatedAt: now,
	}
}

func mkSub(id, catID, name string, deletable bool) category.Subcategory {
	now := time.Now().UTC()
	return category.Subcategory{
		ID: id, CategoryID: catID, Name: name,
		CanBeDeleted: deletable, CreatedAt: now, UpdatedAt: now,
	}
}

// ─── CategoryRepository tests ─────────────────────────────────────────────────

func TestCategoryRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	c := mkCat("cat-1", "Moradia", category.Expense, true)
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "cat-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "cat-1" || got.Name != "Moradia" || got.Type != category.Expense {
		t.Errorf("unexpected: %+v", got)
	}
	if !got.CanBeDeleted {
		t.Error("expected CanBeDeleted=true")
	}
}

func TestCategoryRepo_GetNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)

	_, err := repo.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestCategoryRepo_NullableFields(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	c := mkCat("cat-null", "Test", category.Expense, true)
	c.Icon = ""
	c.Color = ""
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "cat-null")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Icon != "" || got.Color != "" {
		t.Errorf("expected empty icon/color, got icon=%q color=%q", got.Icon, got.Color)
	}
}

func TestCategoryRepo_Update(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	c := mkCat("cat-upd", "Old Name", category.Expense, true)
	repo.Create(ctx, c)

	c.Name = "New Name"
	c.Icon = "icon.svg"
	c.Color = "#ABC"
	c.UpdatedAt = time.Now().UTC()
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := repo.Get(ctx, "cat-upd")
	if got.Name != "New Name" {
		t.Errorf("name: got %q, want New Name", got.Name)
	}
}

func TestCategoryRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkCat("cat-del", "Del", category.Expense, true))
	if err := repo.Delete(ctx, "cat-del"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := repo.Get(ctx, "cat-del")
	if err == nil {
		t.Error("expected not-found after delete")
	}
}

func TestCategoryRepo_DeleteNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	err := repo.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error when deleting nonexistent")
	}
}

func TestCategoryRepo_HasSubcategories(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "Test", category.Expense, true))

	has, err := catRepo.HasSubcategories(ctx, "cat-1")
	if err != nil {
		t.Fatalf("has subs: %v", err)
	}
	if has {
		t.Error("expected no subcategories initially")
	}

	subRepo.Create(ctx, mkSub("sub-1", "cat-1", "Sub", true))
	has, _ = catRepo.HasSubcategories(ctx, "cat-1")
	if !has {
		t.Error("expected HasSubcategories=true after inserting sub")
	}
}

func TestCategoryRepo_DeleteBlockedByFK(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-fk", "Test", category.Expense, true))
	subRepo.Create(ctx, mkSub("sub-fk", "cat-fk", "Sub", true))

	// FK RESTRICT — must fail
	err := catRepo.Delete(ctx, "cat-fk")
	if err == nil {
		t.Error("expected FK constraint error when deleting category with subcategories")
	}
}

func TestCategoryRepo_List(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		repo.Create(ctx, mkCat(fmt.Sprintf("cat-%d", i), fmt.Sprintf("Cat %d", i), category.Expense, true))
	}

	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "name", Order: "ASC"}
	cats, total, err := repo.List(ctx, category.CategoryFilter{}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if len(cats) != 3 {
		t.Errorf("len: got %d, want 3", len(cats))
	}
}

func TestCategoryRepo_ListWithTypeFilter(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteCategoryRepository(db)
	ctx := context.Background()

	repo.Create(ctx, mkCat("c1", "Expense Cat", category.Expense, true))
	repo.Create(ctx, mkCat("c2", "Income Cat", category.Income, true))

	expenseType := category.Expense
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "name", Order: "ASC"}
	cats, total, err := repo.List(ctx, category.CategoryFilter{Type: &expenseType}, p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1", total)
	}
	if len(cats) != 1 || cats[0].Type != category.Expense {
		t.Error("expected only expense categories")
	}
}

func TestCategoryRepo_GetWithSubcategories(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "Moradia", category.Expense, true))
	subRepo.Create(ctx, mkSub("s1", "cat-1", "Aluguel", true))
	subRepo.Create(ctx, mkSub("s2", "cat-1", "Água", true))

	cws, err := catRepo.GetWithSubcategories(ctx, "cat-1")
	if err != nil {
		t.Fatalf("get with subs: %v", err)
	}
	if len(cws.Subcategories) != 2 {
		t.Errorf("subs: got %d, want 2", len(cws.Subcategories))
	}
}

// ─── SubcategoryRepository tests ─────────────────────────────────────────────

func TestSubcategoryRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "Moradia", category.Expense, true))

	s := mkSub("sub-1", "cat-1", "Aluguel", true)
	if err := subRepo.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := subRepo.Get(ctx, "sub-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Aluguel" || got.CategoryID != "cat-1" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestSubcategoryRepo_GetNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteSubcategoryRepository(db)
	_, err := repo.Get(context.Background(), "missing")
	if err == nil {
		t.Error("expected not-found error")
	}
}

func TestSubcategoryRepo_ColorAndIcon_NonNull(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "T", category.Expense, true))
	s := mkSub("sub-color", "cat-1", "With Color", true)
	s.Icon = "icon.png"
	s.Color = "#FF0000"
	subRepo.Create(ctx, s)

	got, _ := subRepo.Get(ctx, "sub-color")
	if got.Icon != "icon.png" {
		t.Errorf("icon: got %q, want icon.png", got.Icon)
	}
	if got.Color != "#FF0000" {
		t.Errorf("color: got %q, want #FF0000", got.Color)
	}
}

func TestSubcategoryRepo_ListAllByType(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-expense", "Moradia", category.Expense, true))
	catRepo.Create(ctx, mkCat("cat-income", "Salário", category.Income, true))
	subRepo.Create(ctx, mkSub("s1", "cat-expense", "Aluguel", true))
	subRepo.Create(ctx, mkSub("s2", "cat-expense", "Água", true))
	subRepo.Create(ctx, mkSub("s3", "cat-income", "Salário Mensal", true))

	subs, err := subRepo.ListAllByType(ctx, category.Expense)
	if err != nil {
		t.Fatalf("list by type: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("expected 2 expense subs, got %d", len(subs))
	}
}

func TestSubcategoryRepo_ListAllByType_Empty(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteSubcategoryRepository(db)

	subs, err := repo.ListAllByType(context.Background(), category.Transfer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0, got %d", len(subs))
	}
}

func TestSubcategoryRepo_List_Pagination(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "Test", category.Expense, true))
	for i := 1; i <= 5; i++ {
		subRepo.Create(ctx, mkSub(fmt.Sprintf("sub-%d", i), "cat-1", fmt.Sprintf("Sub %02d", i), true))
	}

	p := shared.Pagination{Page: 2, Limit: 2, OrderBy: "name", Order: "ASC"}
	subs, total, err := subRepo.List(ctx, "cat-1", p)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(subs) != 2 {
		t.Errorf("page 2 len: got %d, want 2", len(subs))
	}
}

func TestSubcategoryRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()

	catRepo.Create(ctx, mkCat("cat-1", "T", category.Expense, true))
	subRepo.Create(ctx, mkSub("sub-del", "cat-1", "Del", true))

	if err := subRepo.Delete(ctx, "sub-del"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := subRepo.Get(ctx, "sub-del")
	if err == nil {
		t.Error("expected not-found after delete")
	}
}

func TestSubcategoryRepo_DeleteNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := category.NewSQLiteSubcategoryRepository(db)
	err := repo.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected not-found error")
	}
}
