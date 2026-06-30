package report

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func newRouter(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/reports", Routes(NewHandler(svc)))
	return r
}

func do(t *testing.T, router http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	rr := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	rr.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, rr)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestReportRoutes_FullFlow(t *testing.T) {
	a, tt := despesa(120000)
	svc := newSvc(t, "2026-07-05", &fakeRealized{aggs: a, totals: tt})
	router := newRouter(svc)

	// leituras (mês corrente sem fechamento → calculado ao vivo)
	for _, p := range []string{
		"/api/reports/monthly?reference=2026-06",
		"/api/reports/monthly?reference=2026-06&mode=projetivo",
		"/api/reports/monthly", // default reference
		"/api/reports/quarterly?year=2026&quarter=2",
		"/api/reports/semiannual?year=2026&half=1",
		"/api/reports/annual?year=2026",
		"/api/reports/closings",
	} {
		if c, _ := do(t, router, http.MethodGet, p, ""); c != http.StatusOK {
			t.Errorf("GET %s: %d", p, c)
		}
	}

	// fechar mês (junho já terminou em 2026-07-05)
	if c, body := do(t, router, http.MethodPost, "/api/reports/closings", `{"reference":"2026-06"}`); c != http.StatusCreated {
		t.Fatalf("close: %d %v", c, body)
	}
	// fechar de novo → 409
	if c, _ := do(t, router, http.MethodPost, "/api/reports/closings", `{"reference":"2026-06"}`); c != http.StatusConflict {
		t.Errorf("close again: %d want 409", c)
	}
	// lock-state do mês fechado
	if c, body := do(t, router, http.MethodGet, "/api/reports/closings/2026-06/lock-state", ""); c != http.StatusOK || body["status"] != "fechado_ajustavel" {
		t.Errorf("lock-state: %d %v", c, body)
	}
	// recalcular
	if c, _ := do(t, router, http.MethodPost, "/api/reports/closings/2026-06/recalculate", ""); c != http.StatusNoContent {
		t.Errorf("recalculate: %d", c)
	}
}

func TestReportRoutes_Errors(t *testing.T) {
	a, tt := despesa(120000)
	// hoje = 2026-06-15 (junho ainda não terminou)
	svc := newSvc(t, "2026-06-15", &fakeRealized{aggs: a, totals: tt})
	router := newRouter(svc)

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"close não terminou", http.MethodPost, "/api/reports/closings", `{"reference":"2026-06"}`, http.StatusConflict},
		{"close bad json", http.MethodPost, "/api/reports/closings", `{`, http.StatusBadRequest},
		{"close ref inválida", http.MethodPost, "/api/reports/closings", `{"reference":"xx"}`, http.StatusBadRequest},
		{"quarter inválido", http.MethodGet, "/api/reports/quarterly?year=2026&quarter=9", "", http.StatusBadRequest},
		{"semestre inválido", http.MethodGet, "/api/reports/semiannual?year=2026&half=3", "", http.StatusBadRequest},
		{"lock-state ref inválida", http.MethodGet, "/api/reports/closings/xx/lock-state", "", http.StatusBadRequest},
		{"monthly ref inválida", http.MethodGet, "/api/reports/monthly?reference=xx", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if c, _ := do(t, router, tc.method, tc.path, tc.body); c != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, c, tc.want)
		}
	}
	// recalcular mês aberto = no-op 204
	if c, _ := do(t, router, http.MethodPost, "/api/reports/closings/2026-06/recalculate", ""); c != http.StatusNoContent {
		t.Errorf("recalc aberto: %d want 204", c)
	}
}

// Semestral 2º semestre cobre o ramo de período anterior dentro do mesmo ano.
func TestReportRoutes_SemiannualSecondHalf(t *testing.T) {
	a, tt := despesa(120000)
	svc := newSvc(t, "2027-01-05", &fakeRealized{aggs: a, totals: tt})
	router := newRouter(svc)
	if c, _ := do(t, router, http.MethodGet, "/api/reports/semiannual?year=2026&half=2", ""); c != http.StatusOK {
		t.Errorf("semiannual S2: %d", c)
	}
}

var _ = time.Now
