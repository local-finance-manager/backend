package patrimonio

import "github.com/go-chi/chi/v5"

// Routes monta as rotas do módulo de patrimônio sob /api/patrimonio.
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/overview", h.GetOverview)

		r.Route("/caixinhas", func(r chi.Router) {
			r.Get("/", h.ListCaixinhas)
			r.Post("/", h.CreateCaixinha)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.GetCaixinha)
				r.Put("/", h.UpdateCaixinha)
				r.Delete("/", h.DeleteCaixinha)
				r.Patch("/archive", h.Archive)
				r.Patch("/unarchive", h.Unarchive)
				r.Patch("/market-value", h.UpdateMarketValue)
				r.Patch("/saldo-inicial", h.DefinirSaldoInicial)
				r.Post("/aportar", h.Aportar)
				r.Post("/resgatar", h.Resgatar)
				r.Post("/rendimento", h.Rendimento)
				r.Get("/extrato", h.Extrato)
			})
		})

		r.Delete("/movimentos/{txId}", h.DeleteMovimento)
	}
}
