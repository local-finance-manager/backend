package report

import "github.com/go-chi/chi/v5"

// Routes registra as rotas do módulo de relatórios sob /api/reports.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/monthly", h.Monthly)
		r.Get("/quarterly", h.Quarterly)
		r.Get("/semiannual", h.Semiannual)
		r.Get("/annual", h.Annual)

		r.Route("/closings", func(r chi.Router) {
			r.Get("/", h.ListClosings)
			r.Post("/", h.CloseMonth)
			r.Post("/{reference}/recalculate", h.Recalculate)
			r.Get("/{reference}/lock-state", h.LockState)
		})
	}
}
