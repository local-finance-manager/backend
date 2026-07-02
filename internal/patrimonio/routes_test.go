package patrimonio_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/local-finance-manager/backend/internal/patrimonio"
)

func newRouter(repo *fakeRepo, mov *fakeMovements, w *fakeWriter, disp int64) http.Handler {
	svc := newService(repo, mov, w, disp)
	h := patrimonio.NewHandler(svc)
	r := chi.NewRouter()
	r.Route("/patrimonio", patrimonio.Routes(h))
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRoutes_CreateOverviewList(t *testing.T) {
	repo := newFakeRepo()
	mov := &fakeMovements{balances: map[string]int64{}}
	r := newRouter(repo, mov, &fakeWriter{}, 50000)

	// create
	rec := do(t, r, http.MethodPost, "/patrimonio/caixinhas",
		`{"name":"Reserva","type":"reserva","meta_valor":600000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || created.Name != "Reserva" || created.Type != "reserva" {
		t.Fatalf("resposta create inesperada: %s", rec.Body.String())
	}
	mov.balances[created.ID] = 300000

	// overview
	rec = do(t, r, http.MethodGet, "/patrimonio/overview", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("overview status=%d", rec.Code)
	}
	var ov struct {
		Disponivel int64 `json:"disponivel"`
		Guardado   int64 `json:"guardado"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ov)
	if ov.Disponivel != 50000 || ov.Guardado != 300000 {
		t.Fatalf("overview inesperado: %s", rec.Body.String())
	}

	// list
	rec = do(t, r, http.MethodGet, "/patrimonio/caixinhas", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d", rec.Code)
	}
}

func TestRoutes_ResgatarAcimaDoSaldo409(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 10000}}
	r := newRouter(repo, mov, &fakeWriter{}, 0)

	rec := do(t, r, http.MethodPost, "/patrimonio/caixinhas/cx1/resgatar",
		`{"amount":20000,"date":"2026-07-01"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoutes_AportarEExtrato(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 0}}
	r := newRouter(repo, mov, &fakeWriter{}, 0)

	rec := do(t, r, http.MethodPost, "/patrimonio/caixinhas/cx1/aportar",
		`{"amount":15000,"date":"2026-07-01"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("aportar status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(t, r, http.MethodGet, "/patrimonio/caixinhas/cx1/extrato", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("extrato status=%d", rec.Code)
	}
}

func TestRoutes_CreateInvalido400(t *testing.T) {
	r := newRouter(newFakeRepo(), &fakeMovements{balances: map[string]int64{}}, &fakeWriter{}, 0)
	rec := do(t, r, http.MethodPost, "/patrimonio/caixinhas", `{"name":"","type":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, veio %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoutes_GetNotFound404(t *testing.T) {
	r := newRouter(newFakeRepo(), &fakeMovements{balances: map[string]int64{}}, &fakeWriter{}, 0)
	rec := do(t, r, http.MethodGet, "/patrimonio/caixinhas/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d", rec.Code)
	}
}

func TestRoutes_UpdateArchiveMarketValueDelete(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeInvestimento}
	mov := &fakeMovements{balances: map[string]int64{"cx1": 0}}
	r := newRouter(repo, mov, &fakeWriter{}, 0)

	if rec := do(t, r, http.MethodPut, "/patrimonio/caixinhas/cx1",
		`{"name":"Ações BR","type":"investimento"}`); rec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, r, http.MethodPatch, "/patrimonio/caixinhas/cx1/market-value",
		`{"valor_mercado":120000,"data":"2026-07-01"}`); rec.Code != http.StatusOK {
		t.Fatalf("market-value status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, r, http.MethodPatch, "/patrimonio/caixinhas/cx1/archive", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("archive status=%d", rec.Code)
	}
	if rec := do(t, r, http.MethodPatch, "/patrimonio/caixinhas/cx1/unarchive", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("unarchive status=%d", rec.Code)
	}
	if rec := do(t, r, http.MethodDelete, "/patrimonio/caixinhas/cx1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoutes_Rendimento(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "Reserva", Type: patrimonio.TypeReserva}
	w := &fakeWriter{}
	r := newRouter(repo, &fakeMovements{balances: map[string]int64{"cx1": 0}}, w, 0)

	if rec := do(t, r, http.MethodPost, "/patrimonio/caixinhas/cx1/rendimento",
		`{"amount":300,"date":"2026-07-31"}`); rec.Code != http.StatusCreated {
		t.Fatalf("rendimento status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(w.registered) != 1 || !w.registered[0].Rendimento {
		t.Fatalf("deveria registrar rendimento: %+v", w.registered)
	}
}

func TestRoutes_DefinirSaldoInicial(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "Reserva", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{}, opening: map[string][]string{}}
	w := &fakeWriter{}
	r := newRouter(repo, mov, w, 0)

	if rec := do(t, r, http.MethodPatch, "/patrimonio/caixinhas/cx1/saldo-inicial",
		`{"valor":500000,"data":"2026-07-01"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("saldo-inicial status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(w.registered) != 1 || !w.registered[0].Opening {
		t.Fatalf("deveria registrar opening: %+v", w.registered)
	}
	// json inválido → 400
	if rec := do(t, r, http.MethodPatch, "/patrimonio/caixinhas/cx1/saldo-inicial", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, veio %d", rec.Code)
	}
}

func TestRoutes_DeleteMovimento(t *testing.T) {
	repo := newFakeRepo()
	w := &fakeWriter{}
	r := newRouter(repo, &fakeMovements{balances: map[string]int64{}}, w, 0)
	if rec := do(t, r, http.MethodDelete, "/patrimonio/movimentos/tx-1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete movimento status=%d", rec.Code)
	}
	if len(w.deleted) != 1 || w.deleted[0] != "tx-1" {
		t.Fatalf("delete não propagou: %+v", w.deleted)
	}
}
