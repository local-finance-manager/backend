package installment

import "github.com/go-chi/chi/v5"

// Routes registra as rotas do módulo de parcelamento (Apêndice B).
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/preview", h.Preview)
		r.Post("/", h.Create)
		r.Get("/", h.List)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.UpdateSeries)
			r.Delete("/", h.Delete)
			r.Patch("/cancel-remaining", h.CancelRemaining)
		})
	}
}
