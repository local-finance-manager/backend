package category_test

import (
	"context"
	"testing"
	"time"

	"github.com/local-finance-manager/backend/internal/category"
)

func TestCategoryTreeFacade(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed(`INSERT INTO categories (id,name,type,can_be_deleted,created_at,updated_at) VALUES ('c1','Moradia','despesa',1,?,?)`, now, now)
	seed(`INSERT INTO categories (id,name,type,color,can_be_deleted,created_at,updated_at) VALUES ('c2','Salário','receita','#0f0',1,?,?)`, now, now)
	seed(`INSERT INTO categories (id,name,type,can_be_deleted,created_at,updated_at) VALUES ('c3','Vazia','despesa',1,?,?)`, now, now) // sem subs → ramo LEFT JOIN nulo
	seed(`INSERT INTO subcategories (id,category_id,name,can_be_deleted,created_at,updated_at) VALUES ('s1','c1','Aluguel',1,?,?)`, now, now)
	seed(`INSERT INTO subcategories (id,category_id,name,can_be_deleted,created_at,updated_at) VALUES ('s2','c1','Condomínio',1,?,?)`, now, now)

	facade := category.NewCategoryTreeFacade(db)
	tree, err := facade.Tree(context.Background())
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(tree) != 3 {
		t.Fatalf("esperava 3 categorias, got %d", len(tree))
	}
	bySub := map[string]int{}
	for _, c := range tree {
		bySub[c.CategoryID] = len(c.Subcategories)
	}
	if bySub["c1"] != 2 || bySub["c2"] != 0 || bySub["c3"] != 0 {
		t.Errorf("subcategorias por categoria: %+v", bySub)
	}

	db.Close()
	if _, err := facade.Tree(context.Background()); err == nil {
		t.Error("Tree com DB fechado deveria falhar")
	}
}
