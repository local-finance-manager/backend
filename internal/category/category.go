package category

import (
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"
)

// CategoryType represents the financial transaction direction.
type CategoryType string

const (
	Expense  CategoryType = "despesa"
	Income   CategoryType = "receita"
	Transfer CategoryType = "transferencia"
)

// validTypes is used for fast O(1) lookup on CategoryType values.
var validTypes = map[CategoryType]struct{}{
	Expense:  {},
	Income:   {},
	Transfer: {},
}

// Category is the core domain aggregate for a transaction category.
type Category struct {
	ID           string
	Name         string
	Type         CategoryType
	Icon         string
	Color        string
	CanBeDeleted bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Subcategory belongs to exactly one Category.
type Subcategory struct {
	ID           string
	CategoryID   string
	Name         string
	Icon         string
	Color        string
	CanBeDeleted bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CategoryWithSubs is a read-only projection used by GetCategory.
type CategoryWithSubs struct {
	Category
	Subcategories []Subcategory
}

// CategoryFilter is the optional filter applied to category list queries.
type CategoryFilter struct {
	Type *CategoryType
}

// ─── Input types ─────────────────────────────────────────────────────────────

// CreateCategoryInput carries validated data for category creation.
type CreateCategoryInput struct {
	Name  string
	Type  CategoryType
	Icon  string
	Color string
}

// UpdateCategoryInput carries validated data for category update.
type UpdateCategoryInput struct {
	ID    string
	Name  string
	Icon  string
	Color string
}

// CreateSubcategoryInput carries validated data for subcategory creation.
type CreateSubcategoryInput struct {
	CategoryID string
	Name       string
	Icon       string
	Color      string
}

// UpdateSubcategoryInput carries validated data for subcategory update.
type UpdateSubcategoryInput struct {
	ID    string
	Name  string
	Icon  string
	Color string
}

// ─── Domain errors ────────────────────────────────────────────────────────────

var (
	ErrCategoryNotFound    = domainerr.NewNotFound("categoria não encontrada")
	ErrSubcategoryNotFound = domainerr.NewNotFound("subcategoria não encontrada")
	ErrCategoryHasSubs     = domainerr.NewConflict(
		"não é possível excluir categoria com subcategorias cadastradas",
		domainerr.WithDisplayable())
	ErrCategoryNotDeletable = domainerr.NewConflict(
		"esta categoria do sistema não pode ser excluída",
		domainerr.WithDisplayable())
	ErrSubcategoryNotDeletable = domainerr.NewConflict(
		"esta subcategoria do sistema não pode ser excluída",
		domainerr.WithDisplayable())
)

// ─── Validation ───────────────────────────────────────────────────────────────
//
// All validation functions use validation.Accumulator so every broken rule is
// reported in a single round-trip instead of fail-fast one-at-a-time.
//
// Return type contract:
//   - nil                    → input is valid
//   - *domainerr.BadRequestErr  → exactly one rule failed  (HTTP 400)
//   - *domainerr.CompositeErr   → two or more rules failed (HTTP 400, errors:[…])

// ValidateCreateCategory validates creation input at the domain boundary.
func ValidateCreateCategory(in CreateCategoryInput) error {
	name := strings.TrimSpace(in.Name)
	_, typeOK := validTypes[in.Type]
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= 100, "nome deve ter no máximo 100 caracteres").
		Check(typeOK, "tipo inválido: use despesa, receita ou transferencia").
		Result()
}

// ValidateUpdateCategory validates update input at the domain boundary.
func ValidateUpdateCategory(in UpdateCategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= 100, "nome deve ter no máximo 100 caracteres").
		Result()
}

// ValidateCreateSubcategory validates subcategory creation input.
func ValidateCreateSubcategory(in CreateSubcategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(strings.TrimSpace(in.CategoryID) != "", "category_id é obrigatório").
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= 100, "nome deve ter no máximo 100 caracteres").
		Result()
}

// ValidateUpdateSubcategory validates subcategory update input.
func ValidateUpdateSubcategory(in UpdateSubcategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= 100, "nome deve ter no máximo 100 caracteres").
		Result()
}
