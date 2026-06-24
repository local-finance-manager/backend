package category

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/shared"
)

// categoryPaginationDefaults are the module-specific defaults for category queries.
var categoryPaginationDefaults = shared.Pagination{
	Page:    1,
	Limit:   500,
	OrderBy: "name",
	Order:   "ASC",
}

var categoryAllowedOrderBy = []string{"name", "created_at"}

// ─── HandlerDeps ──────────────────────────────────────────────────────────────

// HandlerDeps holds all use case dependencies for the category handler.
type HandlerDeps struct {
	ListCategories          ListCategoriesUseCase
	GetCategory             GetCategoryUseCase
	CreateCategory          CreateCategoryUseCase
	UpdateCategory          UpdateCategoryUseCase
	DeleteCategory          DeleteCategoryUseCase
	ListSubcategories       ListSubcategoriesUseCase
	ListSubcategoriesByType ListSubcategoriesByTypeUseCase
	GetSubcategory          GetSubcategoryUseCase
	CreateSubcategory       CreateSubcategoryUseCase
	UpdateSubcategory       UpdateSubcategoryUseCase
	DeleteSubcategory       DeleteSubcategoryUseCase
}

// Handler handles HTTP requests for the category module.
type Handler struct {
	deps HandlerDeps
}

// NewHandler creates a new Handler with the provided dependencies.
func NewHandler(deps HandlerDeps) *Handler {
	return &Handler{deps: deps}
}

// ─── Response types ───────────────────────────────────────────────────────────

type categoryResp struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Icon         string `json:"icon,omitempty"`
	Color        string `json:"color,omitempty"`
	CanBeDeleted bool   `json:"can_be_deleted"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type categoryWithSubsResp struct {
	categoryResp
	Subcategories []subcategoryResp `json:"subcategories"`
}

type subcategoryResp struct {
	ID                  string `json:"id"`
	CategoryID          string `json:"category_id"`
	Name                string `json:"name"`
	Icon                string `json:"icon,omitempty"`
	Color               string `json:"color,omitempty"`
	CanBeDeleted        bool   `json:"can_be_deleted"`
	IsBalanceAdjustment bool   `json:"is_balance_adjustment"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// ─── Request types ────────────────────────────────────────────────────────────

type createCategoryReq struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

type updateCategoryReq struct {
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

type createSubcategoryReq struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
}

type updateSubcategoryReq struct {
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// ─── Converters ───────────────────────────────────────────────────────────────

func toCategoryResp(c Category) categoryResp {
	return categoryResp{
		ID:           c.ID,
		Name:         c.Name,
		Type:         string(c.Type),
		Icon:         c.Icon,
		Color:        c.Color,
		CanBeDeleted: c.CanBeDeleted,
		CreatedAt:    c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toSubcategoryResp(s Subcategory) subcategoryResp {
	return subcategoryResp{
		ID:                  s.ID,
		CategoryID:          s.CategoryID,
		Name:                s.Name,
		Icon:                s.Icon,
		Color:               s.Color,
		CanBeDeleted:        s.CanBeDeleted,
		IsBalanceAdjustment: s.IsBalanceAdjustment,
		CreatedAt:           s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:           s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toCategoryWithSubsResp(cws CategoryWithSubs) categoryWithSubsResp {
	subs := make([]subcategoryResp, len(cws.Subcategories))
	for i, s := range cws.Subcategories {
		subs[i] = toSubcategoryResp(s)
	}
	return categoryWithSubsResp{
		categoryResp:  toCategoryResp(cws.Category),
		Subcategories: subs,
	}
}

func mapCategoryPagedResp(result shared.PagedResult[Category]) shared.PagedResult[categoryResp] {
	data := make([]categoryResp, len(result.Data))
	for i, c := range result.Data {
		data[i] = toCategoryResp(c)
	}
	return shared.PagedResult[categoryResp]{Data: data, Pagination: result.Pagination}
}

func mapSubcategoryPagedResp(result shared.PagedResult[Subcategory]) shared.PagedResult[subcategoryResp] {
	data := make([]subcategoryResp, len(result.Data))
	for i, s := range result.Data {
		data[i] = toSubcategoryResp(s)
	}
	return shared.PagedResult[subcategoryResp]{Data: data, Pagination: result.Pagination}
}

// ─── Category handlers ────────────────────────────────────────────────────────

// ListCategories handles GET /api/categories
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	var f CategoryFilter
	if raw := r.URL.Query().Get("type"); raw != "" {
		ct := CategoryType(strings.ToLower(raw))
		if _, ok := validTypes[ct]; !ok {
			domainerr.WriteError(w, domainerr.NewBadRequest(
				"tipo inválido: use despesa, receita ou transferencia",
				domainerr.WithDisplayable()))
			return
		}
		f.Type = &ct
	}

	p := shared.ParsePagination(r, categoryPaginationDefaults, categoryAllowedOrderBy)
	result, err := h.deps.ListCategories.Execute(r.Context(), ListCategoriesInput{Filter: f, Pagination: p})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapCategoryPagedResp(result))
}

// ListSubcategoriesByType handles GET /api/categories/sub-categories?type=
func (h *Handler) ListSubcategoriesByType(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("type")
	if raw == "" {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"parâmetro type é obrigatório",
			domainerr.WithDisplayable()))
		return
	}
	ct := CategoryType(strings.ToLower(raw))
	if _, ok := validTypes[ct]; !ok {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"tipo inválido: use despesa, receita ou transferencia",
			domainerr.WithDisplayable()))
		return
	}

	subs, err := h.deps.ListSubcategoriesByType.Execute(r.Context(), ct)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	data := make([]subcategoryResp, len(subs))
	for i, s := range subs {
		data[i] = toSubcategoryResp(s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// GetCategory handles GET /api/categories/{id}
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cws, err := h.deps.GetCategory.Execute(r.Context(), id)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCategoryWithSubsResp(cws))
}

// CreateCategory handles POST /api/categories
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req createCategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}
	in := CreateCategoryInput{
		Name:  req.Name,
		Type:  CategoryType(strings.ToLower(req.Type)),
		Icon:  req.Icon,
		Color: req.Color,
	}
	c, err := h.deps.CreateCategory.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCategoryResp(c))
}

// UpdateCategory handles PUT /api/categories/{id}
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req updateCategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}
	in := UpdateCategoryInput{ID: id, Name: req.Name, Icon: req.Icon, Color: req.Color}
	c, err := h.deps.UpdateCategory.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCategoryResp(c))
}

// DeleteCategory handles DELETE /api/categories/{id}
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.deps.DeleteCategory.Execute(r.Context(), id); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Subcategory handlers ─────────────────────────────────────────────────────

// ListSubcategories handles GET /api/categories/{id}/subcategories
func (h *Handler) ListSubcategories(w http.ResponseWriter, r *http.Request) {
	categoryID := chi.URLParam(r, "id")
	p := shared.ParsePagination(r, categoryPaginationDefaults, categoryAllowedOrderBy)
	result, err := h.deps.ListSubcategories.Execute(r.Context(), ListSubcategoriesInput{
		CategoryID: categoryID,
		Pagination: p,
	})
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSubcategoryPagedResp(result))
}

// GetSubcategory handles GET /api/subcategories/{id}
func (h *Handler) GetSubcategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := h.deps.GetSubcategory.Execute(r.Context(), id)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubcategoryResp(s))
}

// CreateSubcategory handles POST /api/subcategories
func (h *Handler) CreateSubcategory(w http.ResponseWriter, r *http.Request) {
	var req createSubcategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}
	in := CreateSubcategoryInput{
		CategoryID: req.CategoryID,
		Name:       req.Name,
		Icon:       req.Icon,
		Color:      req.Color,
	}
	s, err := h.deps.CreateSubcategory.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toSubcategoryResp(s))
}

// UpdateSubcategory handles PUT /api/subcategories/{id}
func (h *Handler) UpdateSubcategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req updateSubcategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		domainerr.WriteError(w, domainerr.NewBadRequest(
			"corpo da requisição inválido",
			domainerr.WithDisplayable()))
		return
	}
	in := UpdateSubcategoryInput{ID: id, Name: req.Name, Icon: req.Icon, Color: req.Color}
	s, err := h.deps.UpdateSubcategory.Execute(r.Context(), in)
	if err != nil {
		domainerr.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSubcategoryResp(s))
}

// DeleteSubcategory handles DELETE /api/subcategories/{id}
func (h *Handler) DeleteSubcategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.deps.DeleteSubcategory.Execute(r.Context(), id); err != nil {
		domainerr.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
