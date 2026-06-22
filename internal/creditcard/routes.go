package creditcard

import "github.com/go-chi/chi/v5"

// Routes registra as rotas do módulo de cartão de crédito.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", h.ListCreditCards)
		r.Post("/", h.CreateCreditCard)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetCreditCard)
			r.Put("/", h.UpdateCreditCard)
			r.Delete("/", h.DeleteCreditCard)
			r.Patch("/archive", h.ArchiveCreditCard)
			r.Patch("/unarchive", h.UnarchiveCreditCard)
			r.Get("/summary", h.CardSummary)

			r.Get("/invoices", h.ListInvoices)
			r.Get("/invoices/{reference}", h.GetInvoice)
			r.Patch("/invoices/{reference}/pay", h.PayInvoice)
			r.Delete("/invoices/{reference}/pay", h.UndoInvoicePayment)
		})
	}
}
