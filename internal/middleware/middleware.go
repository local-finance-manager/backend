package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
)

// Error recovers from panics and serializes qualquer erro para o formato
// padrão do govalidator ({ status, message, errors, displayable }).
// Deve ser o primeiro middleware registrado no router.
var Error = domainerr.Middleware

// Logger loga cada requisição com método, path, status e duração.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(ww, r)

			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration", time.Since(start).String(),
				"request_id", r.Header.Get("X-Request-Id"),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
