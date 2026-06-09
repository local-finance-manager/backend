package category

import (
	"github.com/go-chi/chi/v5"
)

// Routes registers all category and nested subcategory routes.
// /sub-categories is registered BEFORE /{id} so chi's literal match takes priority.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", h.ListCategories)
		r.Post("/", h.CreateCategory)

		// Literal route must come before /{id} (documents intent; chi handles it regardless).
		r.Get("/sub-categories", h.ListSubcategoriesByType)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetCategory)
			r.Put("/", h.UpdateCategory)
			r.Delete("/", h.DeleteCategory)

			r.Get("/subcategories", h.ListSubcategories)
		})
	}
}

// SubcategoryRoutes registers top-level subcategory CRUD routes.
func SubcategoryRoutes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/", h.CreateSubcategory)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetSubcategory)
			r.Put("/", h.UpdateSubcategory)
			r.Delete("/", h.DeleteSubcategory)
		})
	}
}
