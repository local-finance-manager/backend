package patrimonio_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/patrimonio"
	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Domínio: bordas ──────────────────────────────────────────────────────────

func TestProgress_NegativoZera(t *testing.T) {
	meta := int64(1000)
	c := patrimonio.Caixinha{Type: patrimonio.TypeReserva, MetaValor: &meta}
	if p := c.Progress(-50); p == nil || *p != 0 {
		t.Fatalf("saldo negativo deveria dar 0, veio %v", p)
	}
}

// ─── SQLite: opcionais e caminhos de erro ─────────────────────────────────────

func TestSQLite_CreateWithOptionals(t *testing.T) {
	db := newTestDB(t)
	r := patrimonio.NewSQLiteRepository(db)
	ctx := context.Background()
	meta, vm := int64(1000), int64(2000)
	da, col, ic := "2026-12-31", "#ffffff", "star"
	c := mkCaixinha("cx1", "Obj", patrimonio.TypeObjetivo)
	c.MetaValor, c.ValorMercado, c.DataAlvo, c.Color, c.Icon = &meta, &vm, &da, &col, &ic
	if err := r.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.Get(ctx, "cx1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Color == nil || *got.Color != "#ffffff" || got.Icon == nil ||
		got.DataAlvo == nil || got.ValorMercado == nil {
		t.Fatalf("opcionais não persistiram: %+v", got)
	}
}

func patClosedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()
	return db
}

func TestSQLite_ErrorPaths(t *testing.T) {
	r := patrimonio.NewSQLiteRepository(patClosedDB(t))
	ctx := context.Background()
	c := mkCaixinha("x", "y", patrimonio.TypeReserva)
	if err := r.Create(ctx, c); err == nil {
		t.Fatal("create: esperava erro com DB fechado")
	}
	if _, err := r.Get(ctx, "x"); err == nil {
		t.Fatal("get: esperava erro")
	}
	if _, err := r.List(ctx, true); err == nil {
		t.Fatal("list: esperava erro")
	}
	if err := r.Update(ctx, c); err == nil {
		t.Fatal("update: esperava erro")
	}
	if err := r.SetArchived(ctx, "x", true); err == nil {
		t.Fatal("archive: esperava erro")
	}
	if err := r.SetMarketValue(ctx, "x", 1, "2026-07-01"); err == nil {
		t.Fatal("market value: esperava erro")
	}
	if err := r.Delete(ctx, "x"); err == nil {
		t.Fatal("delete: esperava erro")
	}
}

// ─── Service: overview vazio (percentBp com total 0) ──────────────────────────

func TestOverview_SemGuardado(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	svc := newService(repo, &fakeMovements{balances: map[string]int64{}}, &fakeWriter{}, 0)
	ov, err := svc.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Guardado != 0 || ov.PatrimonioTotal != 0 || len(ov.Caixinhas) != 1 || ov.Caixinhas[0].Percent != 0 {
		t.Fatalf("overview vazio inesperado: %+v", ov)
	}
}

// ─── Handler: caminhos de erro ────────────────────────────────────────────────

func TestRoutes_ErrorPaths(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx-res"] = patrimonio.Caixinha{ID: "cx-res", Name: "Reserva", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{balances: map[string]int64{"cx-res": 0}}
	r := newRouter(repo, mov, &fakeWriter{}, 0)

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"create json inválido", http.MethodPost, "/patrimonio/caixinhas", "{invalido", http.StatusBadRequest},
		{"create validação", http.MethodPost, "/patrimonio/caixinhas", `{"name":"","type":"x"}`, http.StatusBadRequest},
		{"update json inválido", http.MethodPut, "/patrimonio/caixinhas/cx-res", "{", http.StatusBadRequest},
		{"update inexistente", http.MethodPut, "/patrimonio/caixinhas/nope", `{"name":"N","type":"reserva"}`, http.StatusNotFound},
		{"delete inexistente", http.MethodDelete, "/patrimonio/caixinhas/nope", "", http.StatusNotFound},
		{"archive inexistente", http.MethodPatch, "/patrimonio/caixinhas/nope/archive", "", http.StatusNotFound},
		{"unarchive inexistente", http.MethodPatch, "/patrimonio/caixinhas/nope/unarchive", "", http.StatusNotFound},
		{"market-value json inválido", http.MethodPatch, "/patrimonio/caixinhas/cx-res/market-value", "{", http.StatusBadRequest},
		{"market-value em reserva", http.MethodPatch, "/patrimonio/caixinhas/cx-res/market-value", `{"valor_mercado":1,"data":"2026-07-01"}`, http.StatusConflict},
		{"aportar json inválido", http.MethodPost, "/patrimonio/caixinhas/cx-res/aportar", "{", http.StatusBadRequest},
		{"aportar inexistente", http.MethodPost, "/patrimonio/caixinhas/nope/aportar", `{"amount":10,"date":"2026-07-01"}`, http.StatusNotFound},
		{"aportar validação", http.MethodPost, "/patrimonio/caixinhas/cx-res/aportar", `{"amount":0,"date":""}`, http.StatusBadRequest},
		{"resgatar json inválido", http.MethodPost, "/patrimonio/caixinhas/cx-res/resgatar", "{", http.StatusBadRequest},
		{"extrato inexistente", http.MethodGet, "/patrimonio/caixinhas/nope/extrato", "", http.StatusNotFound},
		{"get inexistente", http.MethodGet, "/patrimonio/caixinhas/nope", "", http.StatusNotFound},
	}
	for _, c := range cases {
		rec := do(t, r, c.method, c.path, c.body)
		if rec.Code != c.want {
			t.Fatalf("%s: esperava %d, veio %d (body=%s)", c.name, c.want, rec.Code, rec.Body.String())
		}
	}
}

func TestRoutes_ExtratoComMovimento(t *testing.T) {
	repo := newFakeRepo()
	repo.items["cx1"] = patrimonio.Caixinha{ID: "cx1", Name: "R", Type: patrimonio.TypeReserva}
	mov := &fakeMovements{
		balances: map[string]int64{"cx1": 100},
		movs: []shared.CaixinhaMovement{
			{TransactionID: "m1", CaixinhaID: "cx1", Direction: "aporte", Amount: 100, Date: "2026-07-01", Description: "guardando"},
		},
	}
	r := newRouter(repo, mov, &fakeWriter{}, 0)
	rec := do(t, r, http.MethodGet, "/patrimonio/caixinhas/cx1/extrato", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("extrato status=%d", rec.Code)
	}
	var resp struct {
		Data []struct {
			TransactionID string `json:"transaction_id"`
			Direction     string `json:"direction"`
			Amount        int64  `json:"amount"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].TransactionID != "m1" || resp.Data[0].Amount != 100 {
		t.Fatalf("extrato snake_case inesperado: %s", rec.Body.String())
	}
}
