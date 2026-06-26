package backup

import "github.com/go-chi/chi/v5"

// Routes registra as rotas do módulo de backup (Apêndice B).
func Routes(h *Handler) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/", h.Backup)
		r.Get("/status", h.Status)
		r.Get("/versions", h.ListVersions)
		r.Post("/restore", h.Restore)
		r.Get("/local-snapshots", h.ListLocalSnapshots)
		r.Post("/restore-local", h.RestoreLocal)
	}
}
