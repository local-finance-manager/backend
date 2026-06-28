package category

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CategoryTreeFacade satisfaz report.CategoryTreeReader (structural typing): fornece
// a árvore de categorias (nomes/cores/tipos + subcategorias) para compor relatórios.
// Definido aqui (produtor, dono das tabelas); injetado no main.go. Retorna shared.*.
type CategoryTreeFacade struct{ db *sql.DB }

// NewCategoryTreeFacade cria o facade.
func NewCategoryTreeFacade(db *sql.DB) *CategoryTreeFacade { return &CategoryTreeFacade{db: db} }

// Tree devolve todas as categorias com suas subcategorias.
func (f *CategoryTreeFacade) Tree(ctx context.Context) ([]shared.CategoryNode, error) {
	rows, err := f.db.QueryContext(ctx, `
		SELECT c.id, c.name, COALESCE(c.color, ''), c.type, s.id, s.name
		FROM categories c
		LEFT JOIN subcategories s ON s.category_id = c.id
		ORDER BY c.id, s.name`)
	if err != nil {
		return nil, fmt.Errorf("category tree: query: %w", err)
	}
	defer rows.Close()

	byID := map[string]*shared.CategoryNode{}
	order := []string{}
	for rows.Next() {
		var cid, cname, ccolor, ctype string
		var sid, sname sql.NullString
		if err := rows.Scan(&cid, &cname, &ccolor, &ctype, &sid, &sname); err != nil {
			return nil, fmt.Errorf("category tree: scan: %w", err)
		}
		node, ok := byID[cid]
		if !ok {
			node = &shared.CategoryNode{CategoryID: cid, CategoryName: cname, CategoryColor: ccolor, Type: ctype}
			byID[cid] = node
			order = append(order, cid)
		}
		if sid.Valid {
			node.Subcategories = append(node.Subcategories, shared.SubcategoryNode{ID: sid.String, Name: sname.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]shared.CategoryNode, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}
