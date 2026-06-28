package transaction

import "github.com/go-chi/chi/v5"

// Routes registers all transaction routes under the given router prefix.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", h.ListTransactions)
		r.Post("/", h.CreateTransaction)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetTransaction)
			r.Put("/", h.UpdateTransaction)
			r.Delete("/", h.DeleteTransaction)

			r.Patch("/confirm", h.ConfirmTransaction)
			r.Patch("/cancel", h.CancelTransaction)
		})
	}
}
