package transaction_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	_ "modernc.org/sqlite"

	"github.com/local-finance-manager/backend/internal/shared"
	"github.com/local-finance-manager/backend/internal/transaction"
)

func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()
	return db
}

func TestCaixinhaReader_ErrorPaths(t *testing.T) {
	r := transaction.NewCaixinhaReader(closedDB(t))
	ctx := context.Background()
	if _, _, err := r.ListByCaixinha(ctx, "cx1", shared.DefaultPagination()); err == nil {
		t.Fatal("ListByCaixinha: esperava erro com DB fechado")
	}
	if _, err := r.BalanceByCaixinha(ctx, "cx1"); err == nil {
		t.Fatal("BalanceByCaixinha: esperava erro")
	}
	if _, err := r.BalancesAll(ctx); err == nil {
		t.Fatal("BalancesAll: esperava erro")
	}
	if _, err := r.DisponivelAtual(ctx); err == nil {
		t.Fatal("DisponivelAtual: esperava erro")
	}
}

func TestCaixinhaWriter_RegisterError(t *testing.T) {
	db := newTestDB(t)
	repo := transaction.NewSQLiteRepository(db)
	// facade que falha em GetSubcategoryType → create falha → Register propaga o erro.
	create := transaction.NewCreateTransaction(repo, fakeFacadeErr{err: domainerr.NewNotFound("subcategoria não encontrada")}, fakeChecker{}, nil)
	del := transaction.NewDeleteTransaction(repo, nil)
	w := transaction.NewCaixinhaWriter(create, del)

	if _, err := w.Register(context.Background(), shared.NewCaixinhaMovement{
		CaixinhaID: "cx1", Direction: "resgate", Amount: 1, Date: "2026-07-01",
	}); err == nil {
		t.Fatal("Register: esperava erro quando o create falha")
	}
}
