package category

import (
	"regexp"
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
	// IsBalanceAdjustment marca subcategorias de ajuste de saldo (ex.: "Saldo Inicial").
	// Lançamentos nessas subcategorias entram no saldo acumulado, não no fluxo de
	// receitas/despesas (E6/RF-SALDO-02). Vem do seed; imutável via CRUD (RF-SALDO-04).
	IsBalanceAdjustment bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
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
	ErrCategoryNotFound    = domainerr.NewNotFound("categoria não encontrada", domainerr.WithDisplayable())
	ErrSubcategoryNotFound = domainerr.NewNotFound("subcategoria não encontrada", domainerr.WithDisplayable())
	ErrCategoryHasSubs     = domainerr.NewConflict(
		"não é possível excluir categoria com subcategorias cadastradas",
		domainerr.WithDisplayable())
	ErrCategoryNotDeletable = domainerr.NewConflict(
		"esta categoria do sistema não pode ser excluída",
		domainerr.WithDisplayable())
	ErrSubcategoryNotDeletable = domainerr.NewConflict(
		"esta subcategoria do sistema não pode ser excluída",
		domainerr.WithDisplayable())
	ErrSubcategoryHasTransactions = domainerr.NewConflict(
		"não é possível excluir: existem lançamentos vinculados a esta subcategoria",
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

const (
	maxNameLen = 100
	maxIconLen = 100
)

// colorRegex validates the optional #RRGGBB hex color format.
var colorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ValidateCreateCategory validates creation input at the domain boundary.
func ValidateCreateCategory(in CreateCategoryInput) error {
	name := strings.TrimSpace(in.Name)
	_, typeOK := validTypes[in.Type]
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= maxNameLen, "nome deve ter no máximo 100 caracteres").
		Check(typeOK, "tipo inválido: use despesa, receita ou transferencia").
		Check(len(in.Icon) <= maxIconLen, "ícone deve ter no máximo 100 caracteres").
		Check(in.Color == "" || colorRegex.MatchString(in.Color), "cor deve estar no formato #RRGGBB").
		Result()
}

// ValidateUpdateCategory validates update input at the domain boundary.
func ValidateUpdateCategory(in UpdateCategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= maxNameLen, "nome deve ter no máximo 100 caracteres").
		Check(len(in.Icon) <= maxIconLen, "ícone deve ter no máximo 100 caracteres").
		Check(in.Color == "" || colorRegex.MatchString(in.Color), "cor deve estar no formato #RRGGBB").
		Result()
}

// ValidateCreateSubcategory validates subcategory creation input.
func ValidateCreateSubcategory(in CreateSubcategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(strings.TrimSpace(in.CategoryID) != "", "category_id é obrigatório").
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= maxNameLen, "nome deve ter no máximo 100 caracteres").
		Check(len(in.Icon) <= maxIconLen, "ícone deve ter no máximo 100 caracteres").
		Check(in.Color == "" || colorRegex.MatchString(in.Color), "cor deve estar no formato #RRGGBB").
		Result()
}

// ValidateUpdateSubcategory validates subcategory update input.
func ValidateUpdateSubcategory(in UpdateSubcategoryInput) error {
	name := strings.TrimSpace(in.Name)
	return validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= maxNameLen, "nome deve ter no máximo 100 caracteres").
		Check(len(in.Icon) <= maxIconLen, "ícone deve ter no máximo 100 caracteres").
		Check(in.Color == "" || colorRegex.MatchString(in.Color), "cor deve estar no formato #RRGGBB").
		Result()
}
