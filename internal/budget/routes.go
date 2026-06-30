package budget

import "github.com/go-chi/chi/v5"

// Routes registra as rotas do módulo de alocação de receitas sob /api/income.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/plan", h.GetPlan)

		r.Route("/destinations", func(r chi.Router) {
			r.Post("/", h.CreateDestination)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", h.UpdateDestination)
				r.Delete("/", h.DeleteDestination)
				r.Post("/materialize", h.Materialize)
				r.Delete("/materialize", h.Undo)
			})
		})

		r.Route("/plan/{reference}", func(r chi.Router) {
			r.Post("/materialize-all", h.MaterializeAll)
			r.Post("/apply-template", h.ApplyTemplate)
			r.Post("/copy-previous", h.CopyPrevious)
		})

		r.Route("/templates", func(r chi.Router) {
			r.Get("/", h.ListTemplates)
			r.Post("/", h.CreateTemplate)
		})
	}
}
