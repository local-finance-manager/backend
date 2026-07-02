package patrimonio_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/patrimonio"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE caixinhas (
		id                 TEXT PRIMARY KEY,
		name               TEXT NOT NULL,
		type               TEXT NOT NULL,
		meta_valor         INTEGER,
		data_alvo          TEXT,
		valor_mercado      INTEGER,
		data_valor_mercado TEXT,
		color              TEXT,
		icon               TEXT,
		display_order      INTEGER NOT NULL DEFAULT 0,
		archived           INTEGER NOT NULL DEFAULT 0,
		created_at         TEXT NOT NULL,
		updated_at         TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create caixinhas: %v", err)
	}
	return db
}

func mkCaixinha(id, name string, typ patrimonio.CaixinhaType) patrimonio.Caixinha {
	now := time.Now().UTC()
	return patrimonio.Caixinha{ID: id, Name: name, Type: typ, CreatedAt: now, UpdatedAt: now}
}

func TestSQLite_CreateGetListUpdateDelete(t *testing.T) {
	db := newTestDB(t)
	r := patrimonio.NewSQLiteRepository(db)
	ctx := context.Background()

	meta := int64(600000)
	c := mkCaixinha("cx1", "Reserva", patrimonio.TypeReserva)
	c.MetaValor = &meta
	if err := r.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.Get(ctx, "cx1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Reserva" || got.MetaValor == nil || *got.MetaValor != 600000 {
		t.Fatalf("get inesperado: %+v", got)
	}

	// segunda caixinha arquivada
	c2 := mkCaixinha("cx2", "Antiga", patrimonio.TypeObjetivo)
	c2.Archived = true
	if err := r.Create(ctx, c2); err != nil {
		t.Fatalf("create 2: %v", err)
	}

	active, err := r.List(ctx, false)
	if err != nil || len(active) != 1 {
		t.Fatalf("list ativas: %v len=%d", err, len(active))
	}
	all, err := r.List(ctx, true)
	if err != nil || len(all) != 2 {
		t.Fatalf("list todas: %v len=%d", err, len(all))
	}

	got.Name = "Reserva de Emergência"
	got.UpdatedAt = time.Now().UTC()
	if err := r.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	reread, _ := r.Get(ctx, "cx1")
	if reread.Name != "Reserva de Emergência" {
		t.Fatalf("update não persistiu: %q", reread.Name)
	}

	if err := r.SetMarketValue(ctx, "cx1", 123456, "2026-07-01"); err != nil {
		t.Fatalf("set market value: %v", err)
	}
	reread, _ = r.Get(ctx, "cx1")
	if reread.ValorMercado == nil || *reread.ValorMercado != 123456 {
		t.Fatalf("market value não persistiu: %+v", reread.ValorMercado)
	}

	if err := r.SetArchived(ctx, "cx1", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := r.Delete(ctx, "cx1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, "cx1"); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("esperava ErrCaixinhaNotFound, veio %v", err)
	}
}

func TestSQLite_NotFoundOnMissing(t *testing.T) {
	db := newTestDB(t)
	r := patrimonio.NewSQLiteRepository(db)
	ctx := context.Background()
	if _, err := r.Get(ctx, "nope"); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("get: esperava not found, veio %v", err)
	}
	if err := r.Update(ctx, mkCaixinha("nope", "x", patrimonio.TypeReserva)); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("update: esperava not found, veio %v", err)
	}
	if err := r.Delete(ctx, "nope"); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("delete: esperava not found, veio %v", err)
	}
	if err := r.SetArchived(ctx, "nope", true); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("archive: esperava not found, veio %v", err)
	}
	if err := r.SetMarketValue(ctx, "nope", 1, "2026-07-01"); err != patrimonio.ErrCaixinhaNotFound {
		t.Fatalf("market: esperava not found, veio %v", err)
	}
}
