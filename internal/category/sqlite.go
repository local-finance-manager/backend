package category

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── SQLiteCategoryRepository ─────────────────────────────────────────────────

// SQLiteCategoryRepository implements CategoryRepository using SQLite.
type SQLiteCategoryRepository struct {
	db *sql.DB
}

// NewSQLiteCategoryRepository creates a new SQLiteCategoryRepository.
func NewSQLiteCategoryRepository(db *sql.DB) *SQLiteCategoryRepository {
	return &SQLiteCategoryRepository{db: db}
}

var _ CategoryRepository = (*SQLiteCategoryRepository)(nil)

func (r *SQLiteCategoryRepository) List(ctx context.Context, f CategoryFilter, p shared.Pagination) ([]Category, int, error) {
	conds := []string{}
	args := []any{}

	if f.Type != nil {
		conds = append(conds, "type = ?")
		args = append(args, string(*f.Type))
	}
	if p.StartDate != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, p.StartDate.UTC().Format(time.RFC3339))
	}
	if p.EndDate != nil {
		conds = append(conds, "created_at <= ?")
		args = append(args, p.EndDate.UTC().Format(time.RFC3339))
	}

	where := buildWhere(conds)

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM categories "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("category repo: count: %w", err)
	}

	// p.OrderBy and p.Order are validated by ParsePagination (allowlist + normalize).
	dataSQL := fmt.Sprintf(
		"SELECT id, name, type, icon, color, can_be_deleted, created_at, updated_at"+
			" FROM categories %s ORDER BY %s %s LIMIT ? OFFSET ?",
		where, p.OrderBy, p.Order,
	)
	dataArgs := appendPaginationArgs(args, p)

	rows, err := r.db.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("category repo: list: %w", err)
	}
	defer rows.Close()

	cats := make([]Category, 0, total)
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("category repo: scan: %w", err)
		}
		cats = append(cats, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("category repo: list rows: %w", err)
	}
	return cats, total, nil
}

func (r *SQLiteCategoryRepository) Get(ctx context.Context, id string) (Category, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, type, icon, color, can_be_deleted, created_at, updated_at"+
			" FROM categories WHERE id = ?", id)
	c, err := scanCategory(row)
	if err == sql.ErrNoRows {
		return Category{}, ErrCategoryNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("category repo: get: %w", err)
	}
	return c, nil
}

func (r *SQLiteCategoryRepository) GetWithSubcategories(ctx context.Context, id string) (CategoryWithSubs, error) {
	c, err := r.Get(ctx, id)
	if err != nil {
		return CategoryWithSubs{}, err
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, category_id, name, icon, color, can_be_deleted, created_at, updated_at"+
			" FROM subcategories WHERE category_id = ? ORDER BY LOWER(name) ASC", id)
	if err != nil {
		return CategoryWithSubs{}, fmt.Errorf("category repo: get with subs: %w", err)
	}
	defer rows.Close()

	subs := []Subcategory{}
	for rows.Next() {
		s, err := scanSubcategory(rows)
		if err != nil {
			return CategoryWithSubs{}, fmt.Errorf("category repo: scan sub: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return CategoryWithSubs{}, fmt.Errorf("category repo: get subs rows: %w", err)
	}
	return CategoryWithSubs{Category: c, Subcategories: subs}, nil
}

func (r *SQLiteCategoryRepository) HasSubcategories(ctx context.Context, id string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM subcategories WHERE category_id = ?", id,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("category repo: has subcategories: %w", err)
	}
	return count > 0, nil
}

func (r *SQLiteCategoryRepository) Create(ctx context.Context, c Category) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO categories (id, name, type, icon, color, can_be_deleted, created_at, updated_at)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		c.ID, c.Name, string(c.Type), toNullString(c.Icon), toNullString(c.Color),
		boolToInt(c.CanBeDeleted), c.CreatedAt.UTC().Format(time.RFC3339), c.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("category repo: create: %w", err)
	}
	return nil
}

func (r *SQLiteCategoryRepository) Update(ctx context.Context, c Category) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE categories SET name = ?, icon = ?, color = ?, updated_at = ? WHERE id = ?",
		c.Name, toNullString(c.Icon), toNullString(c.Color), c.UpdatedAt.UTC().Format(time.RFC3339), c.ID,
	)
	if err != nil {
		return fmt.Errorf("category repo: update: %w", err)
	}
	return nil
}

func (r *SQLiteCategoryRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("category repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCategoryNotFound
	}
	return nil
}

// ─── SQLiteSubcategoryRepository ─────────────────────────────────────────────

// SQLiteSubcategoryRepository implements SubcategoryRepository using SQLite.
type SQLiteSubcategoryRepository struct {
	db *sql.DB
}

// NewSQLiteSubcategoryRepository creates a new SQLiteSubcategoryRepository.
func NewSQLiteSubcategoryRepository(db *sql.DB) *SQLiteSubcategoryRepository {
	return &SQLiteSubcategoryRepository{db: db}
}

var _ SubcategoryRepository = (*SQLiteSubcategoryRepository)(nil)

func (r *SQLiteSubcategoryRepository) List(ctx context.Context, categoryID string, p shared.Pagination) ([]Subcategory, int, error) {
	conds := []string{"category_id = ?"}
	args := []any{categoryID}

	if p.StartDate != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, p.StartDate.UTC().Format(time.RFC3339))
	}
	if p.EndDate != nil {
		conds = append(conds, "created_at <= ?")
		args = append(args, p.EndDate.UTC().Format(time.RFC3339))
	}

	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM subcategories "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("subcategory repo: count: %w", err)
	}

	// p.OrderBy and p.Order are validated by ParsePagination (allowlist + normalize).
	dataSQL := fmt.Sprintf(
		"SELECT id, category_id, name, icon, color, can_be_deleted, created_at, updated_at"+
			" FROM subcategories %s ORDER BY %s %s LIMIT ? OFFSET ?",
		where, p.OrderBy, p.Order,
	)
	dataArgs := appendPaginationArgs(args, p)

	rows, err := r.db.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("subcategory repo: list: %w", err)
	}
	defer rows.Close()

	subs := make([]Subcategory, 0, total)
	for rows.Next() {
		s, err := scanSubcategory(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("subcategory repo: scan: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("subcategory repo: list rows: %w", err)
	}
	return subs, total, nil
}

func (r *SQLiteSubcategoryRepository) ListAllByType(ctx context.Context, t CategoryType) ([]Subcategory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.category_id, s.name, s.icon, s.color,
		       s.can_be_deleted, s.created_at, s.updated_at
		FROM   subcategories s
		JOIN   categories c ON c.id = s.category_id
		WHERE  c.type = ?
		ORDER BY LOWER(s.name) ASC
	`, string(t))
	if err != nil {
		return nil, fmt.Errorf("subcategory repo: list by type: %w", err)
	}
	defer rows.Close()

	subs := []Subcategory{}
	for rows.Next() {
		s, err := scanSubcategory(rows)
		if err != nil {
			return nil, fmt.Errorf("subcategory repo: scan: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("subcategory repo: list by type rows: %w", err)
	}
	return subs, nil
}

func (r *SQLiteSubcategoryRepository) Get(ctx context.Context, id string) (Subcategory, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, category_id, name, icon, color, can_be_deleted, created_at, updated_at"+
			" FROM subcategories WHERE id = ?", id)
	s, err := scanSubcategory(row)
	if err == sql.ErrNoRows {
		return Subcategory{}, ErrSubcategoryNotFound
	}
	if err != nil {
		return Subcategory{}, fmt.Errorf("subcategory repo: get: %w", err)
	}
	return s, nil
}

func (r *SQLiteSubcategoryRepository) Create(ctx context.Context, s Subcategory) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO subcategories (id, category_id, name, icon, color, can_be_deleted, created_at, updated_at)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.CategoryID, s.Name, toNullString(s.Icon), toNullString(s.Color),
		boolToInt(s.CanBeDeleted), s.CreatedAt.UTC().Format(time.RFC3339), s.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("subcategory repo: create: %w", err)
	}
	return nil
}

func (r *SQLiteSubcategoryRepository) Update(ctx context.Context, s Subcategory) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE subcategories SET name = ?, icon = ?, color = ?, updated_at = ? WHERE id = ?",
		s.Name, toNullString(s.Icon), toNullString(s.Color), s.UpdatedAt.UTC().Format(time.RFC3339), s.ID,
	)
	if err != nil {
		return fmt.Errorf("subcategory repo: update: %w", err)
	}
	return nil
}

func (r *SQLiteSubcategoryRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM subcategories WHERE id = ?", id)
	if err != nil {
		if isForeignKeyConstraintError(err) {
			return ErrSubcategoryHasTransactions
		}
		return fmt.Errorf("subcategory repo: delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSubcategoryNotFound
	}
	return nil
}

func isForeignKeyConstraintError(err error) bool {
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanCategory(s scanner) (Category, error) {
	var c Category
	var typeStr string
	var icon, color sql.NullString
	var canBeDel int
	var createdAt, updatedAt string

	if err := s.Scan(&c.ID, &c.Name, &typeStr, &icon, &color, &canBeDel, &createdAt, &updatedAt); err != nil {
		return Category{}, err
	}
	c.Type = CategoryType(typeStr)
	c.Icon = icon.String
	c.Color = color.String
	c.CanBeDeleted = canBeDel != 0
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

func scanSubcategory(s scanner) (Subcategory, error) {
	var sub Subcategory
	var icon, color sql.NullString
	var canBeDel int
	var createdAt, updatedAt string

	if err := s.Scan(&sub.ID, &sub.CategoryID, &sub.Name, &icon, &color, &canBeDel, &createdAt, &updatedAt); err != nil {
		return Subcategory{}, err
	}
	sub.Icon = icon.String
	sub.Color = color.String
	sub.CanBeDeleted = canBeDel != 0
	sub.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sub.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return sub, nil
}

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func buildWhere(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(conds, " AND ")
}

func appendPaginationArgs(base []any, p shared.Pagination) []any {
	args := make([]any, 0, len(base)+2)
	args = append(args, base...)
	args = append(args, p.Limit, p.Offset())
	return args
}
