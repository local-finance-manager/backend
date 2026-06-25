package category_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/category"
	"github.com/local-finance-manager/backend/internal/shared"
)

// newCategoryRouter monta o stack HTTP real (repos sqlite + use cases + handler + rotas)
// sobre um banco :memory: — cobre handler.go, routes.go e exercita use cases/sqlite.
func newCategoryRouter(db *sql.DB) http.Handler {
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	getSub := category.NewGetSubcategory(subRepo)
	getCat := category.NewGetCategory(catRepo)

	h := category.NewHandler(category.HandlerDeps{
		ListCategories:          category.NewListCategories(catRepo),
		GetCategory:             getCat,
		CreateCategory:          category.NewCreateCategory(catRepo),
		UpdateCategory:          category.NewUpdateCategory(catRepo),
		DeleteCategory:          category.NewDeleteCategory(catRepo),
		ListSubcategories:       category.NewListSubcategories(catRepo, subRepo),
		ListSubcategoriesByType: category.NewListSubcategoriesByType(subRepo),
		GetSubcategory:          getSub,
		CreateSubcategory:       category.NewCreateSubcategory(catRepo, subRepo),
		UpdateSubcategory:       category.NewUpdateSubcategory(subRepo),
		DeleteSubcategory:       category.NewDeleteSubcategory(subRepo),
	})

	r := chi.NewRouter()
	r.Route("/api/categories", category.Routes(h))
	r.Route("/api/subcategories", category.SubcategoryRoutes(h))
	return r
}

func do(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != "" {
		rdr = bytes.NewReader([]byte(body))
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestCategoryRoutes_FullCRUD(t *testing.T) {
	db := newTestDB(t)
	router := newCategoryRouter(db)

	// Create category
	code, body := do(t, router, http.MethodPost, "/api/categories", `{"name":"Lazer","type":"despesa","color":"#FFFFFF"}`)
	if code != http.StatusCreated {
		t.Fatalf("create category: got %d body %v", code, body)
	}
	catID, _ := body["id"].(string)
	if catID == "" {
		t.Fatalf("create category: no id in %v", body)
	}

	// Create category — validation error (empty name)
	if code, _ := do(t, router, http.MethodPost, "/api/categories", `{"name":"","type":"despesa"}`); code != http.StatusBadRequest {
		t.Errorf("create invalid category: got %d, want 400", code)
	}
	// Create category — invalid JSON
	if code, _ := do(t, router, http.MethodPost, "/api/categories", `{`); code != http.StatusBadRequest {
		t.Errorf("create bad json: got %d, want 400", code)
	}

	// List categories
	if code, body := do(t, router, http.MethodGet, "/api/categories?type=despesa", ""); code != http.StatusOK || body["data"] == nil {
		t.Errorf("list categories: got %d body %v", code, body)
	}

	// Get category
	if code, _ := do(t, router, http.MethodGet, "/api/categories/"+catID, ""); code != http.StatusOK {
		t.Errorf("get category: got %d", code)
	}
	// Get category — not found
	if code, _ := do(t, router, http.MethodGet, "/api/categories/nope", ""); code != http.StatusNotFound {
		t.Errorf("get missing category: got %d, want 404", code)
	}

	// Update category
	if code, _ := do(t, router, http.MethodPut, "/api/categories/"+catID, `{"name":"Lazer e Hobbies","color":"#000000"}`); code != http.StatusOK {
		t.Errorf("update category: got %d", code)
	}

	// Create subcategory
	code, sb := do(t, router, http.MethodPost, "/api/subcategories", `{"category_id":"`+catID+`","name":"Cinema"}`)
	if code != http.StatusCreated {
		t.Fatalf("create subcategory: got %d body %v", code, sb)
	}
	subID, _ := sb["id"].(string)

	// Create subcategory — validation error
	if code, _ := do(t, router, http.MethodPost, "/api/subcategories", `{"category_id":"`+catID+`","name":""}`); code != http.StatusBadRequest {
		t.Errorf("create invalid subcategory: got %d, want 400", code)
	}

	// List subcategories of a category
	if code, _ := do(t, router, http.MethodGet, "/api/categories/"+catID+"/subcategories", ""); code != http.StatusOK {
		t.Errorf("list subcategories: got %d", code)
	}
	// List subcategories by type
	if code, _ := do(t, router, http.MethodGet, "/api/categories/sub-categories?type=despesa", ""); code != http.StatusOK {
		t.Errorf("list subcategories by type: got %d", code)
	}

	// Get subcategory
	if code, _ := do(t, router, http.MethodGet, "/api/subcategories/"+subID, ""); code != http.StatusOK {
		t.Errorf("get subcategory: got %d", code)
	}
	// Get subcategory — not found
	if code, _ := do(t, router, http.MethodGet, "/api/subcategories/nope", ""); code != http.StatusNotFound {
		t.Errorf("get missing subcategory: got %d, want 404", code)
	}

	// Update subcategory
	if code, _ := do(t, router, http.MethodPut, "/api/subcategories/"+subID, `{"name":"Cinema e Streaming"}`); code != http.StatusOK {
		t.Errorf("update subcategory: got %d", code)
	}

	// Delete the category while it has a subcategory → 409
	if code, _ := do(t, router, http.MethodDelete, "/api/categories/"+catID, ""); code != http.StatusConflict {
		t.Errorf("delete category with subs: got %d, want 409", code)
	}

	// Delete subcategory, then category
	if code, _ := do(t, router, http.MethodDelete, "/api/subcategories/"+subID, ""); code != http.StatusNoContent {
		t.Errorf("delete subcategory: got %d, want 204", code)
	}
	if code, _ := do(t, router, http.MethodDelete, "/api/categories/"+catID, ""); code != http.StatusNoContent {
		t.Errorf("delete category: got %d, want 204", code)
	}
}

// TestSubcategoryRepo_DeleteInUse cobre isForeignKeyConstraintError: excluir subcategoria
// referenciada por um lançamento retorna ErrSubcategoryHasTransactions.
func TestSubcategoryRepo_DeleteInUse(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE transactions (
		id TEXT PRIMARY KEY,
		subcategory_id TEXT NOT NULL REFERENCES subcategories(id) ON DELETE RESTRICT
	)`); err != nil {
		t.Fatalf("create transactions: %v", err)
	}
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()
	catRepo.Create(ctx, mkCat("c", "C", category.Expense, true))
	subRepo.Create(ctx, mkSub("s", "c", "S", true))
	if _, err := db.Exec(`INSERT INTO transactions (id, subcategory_id) VALUES ('t1','s')`); err != nil {
		t.Fatalf("insert txn: %v", err)
	}
	if err := subRepo.Delete(ctx, "s"); err != category.ErrSubcategoryHasTransactions {
		t.Errorf("expected ErrSubcategoryHasTransactions, got %v", err)
	}
}

func TestCategoryRoutes_ErrorPaths(t *testing.T) {
	db := newTestDB(t)
	router := newCategoryRouter(db)
	// uma categoria válida para o create de subcategoria
	code, body := do(t, router, http.MethodPost, "/api/categories", `{"name":"Casa","type":"despesa"}`)
	if code != http.StatusCreated {
		t.Fatalf("seed category: %d %v", code, body)
	}
	catID, _ := body["id"].(string)

	// ListSubcategoriesByType: type ausente / inválido → 400
	if c, _ := do(t, router, http.MethodGet, "/api/categories/sub-categories", ""); c != http.StatusBadRequest {
		t.Errorf("sub-categories sem type: got %d, want 400", c)
	}
	if c, _ := do(t, router, http.MethodGet, "/api/categories/sub-categories?type=xpto", ""); c != http.StatusBadRequest {
		t.Errorf("sub-categories type inválido: got %d, want 400", c)
	}

	// UpdateCategory: json inválido (400) e não encontrado (404)
	if c, _ := do(t, router, http.MethodPut, "/api/categories/"+catID, `{`); c != http.StatusBadRequest {
		t.Errorf("update cat bad json: got %d, want 400", c)
	}
	if c, _ := do(t, router, http.MethodPut, "/api/categories/nope", `{"name":"X"}`); c != http.StatusNotFound {
		t.Errorf("update cat missing: got %d, want 404", c)
	}

	// CreateSubcategory: json inválido (400) e categoria inexistente (404)
	if c, _ := do(t, router, http.MethodPost, "/api/subcategories", `{`); c != http.StatusBadRequest {
		t.Errorf("create sub bad json: got %d, want 400", c)
	}
	if c, _ := do(t, router, http.MethodPost, "/api/subcategories", `{"category_id":"nope","name":"X"}`); c != http.StatusNotFound {
		t.Errorf("create sub categoria inexistente: got %d, want 404", c)
	}

	// cria uma subcategoria para testar update/delete dela
	_, sb := do(t, router, http.MethodPost, "/api/subcategories", `{"category_id":"`+catID+`","name":"Aluguel"}`)
	subID, _ := sb["id"].(string)

	// UpdateSubcategory: json inválido (400) e não encontrado (404)
	if c, _ := do(t, router, http.MethodPut, "/api/subcategories/"+subID, `{`); c != http.StatusBadRequest {
		t.Errorf("update sub bad json: got %d, want 400", c)
	}
	if c, _ := do(t, router, http.MethodPut, "/api/subcategories/nope", `{"name":"X"}`); c != http.StatusNotFound {
		t.Errorf("update sub missing: got %d, want 404", c)
	}

	// DeleteSubcategory / DeleteCategory inexistentes → 404
	if c, _ := do(t, router, http.MethodDelete, "/api/subcategories/nope", ""); c != http.StatusNotFound {
		t.Errorf("delete sub missing: got %d, want 404", c)
	}
	if c, _ := do(t, router, http.MethodDelete, "/api/categories/nope", ""); c != http.StatusNotFound {
		t.Errorf("delete cat missing: got %d, want 404", c)
	}

	// ListCategories com type inválido → 400
	if c, _ := do(t, router, http.MethodGet, "/api/categories?type=xpto", ""); c != http.StatusBadRequest {
		t.Errorf("list cat type inválido: got %d, want 400", c)
	}

	// Listagens com paginação (exercita ParsePagination + ramos de ordenação)
	if c, _ := do(t, router, http.MethodGet, "/api/categories?page=1&limit=5&order_by=name&order=asc", ""); c != http.StatusOK {
		t.Errorf("list cat paginada: got %d", c)
	}
	if c, _ := do(t, router, http.MethodGet, "/api/categories/"+catID+"/subcategories?page=1&limit=5", ""); c != http.StatusOK {
		t.Errorf("list sub paginada: got %d", c)
	}
}

// TestCategoryRepo_DBErrors cobre os ramos de erro de I/O dos repositórios (DB fechado).
func TestCategoryRepo_DBErrors(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()
	p := shared.Pagination{Page: 1, Limit: 10, OrderBy: "created_at", Order: "DESC"}
	db.Close() // a partir daqui toda query falha

	if err := catRepo.Create(ctx, mkCat("c", "C", category.Expense, true)); err == nil {
		t.Error("Create deveria falhar com DB fechado")
	}
	if _, err := catRepo.Get(ctx, "c"); err == nil {
		t.Error("Get deveria falhar")
	}
	if _, _, err := catRepo.List(ctx, category.CategoryFilter{}, p); err == nil {
		t.Error("List deveria falhar")
	}
	if err := catRepo.Update(ctx, mkCat("c", "C", category.Expense, true)); err == nil {
		t.Error("Update deveria falhar")
	}
	if err := catRepo.Delete(ctx, "c"); err == nil {
		t.Error("Delete deveria falhar")
	}
	if _, err := catRepo.HasSubcategories(ctx, "c"); err == nil {
		t.Error("HasSubcategories deveria falhar")
	}
	if _, err := catRepo.GetWithSubcategories(ctx, "c"); err == nil {
		t.Error("GetWithSubcategories deveria falhar")
	}

	if err := subRepo.Create(ctx, mkSub("s", "c", "S", true)); err == nil {
		t.Error("sub Create deveria falhar")
	}
	if _, err := subRepo.Get(ctx, "s"); err == nil {
		t.Error("sub Get deveria falhar")
	}
	if _, _, err := subRepo.List(ctx, "c", p); err == nil {
		t.Error("sub List deveria falhar")
	}
	if _, err := subRepo.ListAllByType(ctx, category.Expense); err == nil {
		t.Error("ListAllByType deveria falhar")
	}
	if err := subRepo.Update(ctx, mkSub("s", "c", "S", true)); err == nil {
		t.Error("sub Update deveria falhar")
	}
	if err := subRepo.Delete(ctx, "s"); err == nil {
		t.Error("sub Delete deveria falhar")
	}
}

func TestSubcategoryFacade_GetSubcategoryType(t *testing.T) {
	db := newTestDB(t)
	catRepo := category.NewSQLiteCategoryRepository(db)
	subRepo := category.NewSQLiteSubcategoryRepository(db)
	ctx := context.Background()
	catRepo.Create(ctx, mkCat("cat-fac", "Receitas", category.Income, true))
	subRepo.Create(ctx, mkSub("sub-fac", "cat-fac", "Salário", true))

	facade := category.NewSubcategoryFacade(category.NewGetSubcategory(subRepo), category.NewGetCategory(catRepo))
	typ, err := facade.GetSubcategoryType(ctx, "sub-fac")
	if err != nil {
		t.Fatalf("GetSubcategoryType: %v", err)
	}
	if typ != "receita" {
		t.Errorf("type: got %s, want receita", typ)
	}
	if _, err := facade.GetSubcategoryType(ctx, "missing"); err == nil {
		t.Error("expected error for missing subcategory")
	}
}
