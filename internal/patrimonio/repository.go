package patrimonio

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Repository é o contrato de persistência das caixinhas (owner da tabela `caixinhas`).
type Repository interface {
	Create(ctx context.Context, c Caixinha) error
	Get(ctx context.Context, id string) (Caixinha, error)
	List(ctx context.Context, includeArchived bool) ([]Caixinha, error)
	Update(ctx context.Context, c Caixinha) error
	SetArchived(ctx context.Context, id string, archived bool) error
	SetMarketValue(ctx context.Context, id string, valor int64, data string) error
	Delete(ctx context.Context, id string) error
}

// ─── Ports consumidos (implementados pelo módulo transaction, injetados no main.go) ───

// MovementWriter cria/exclui os lançamentos neutros de aporte/resgate no livro-caixa.
type MovementWriter interface {
	Register(ctx context.Context, in shared.NewCaixinhaMovement) (txID string, err error)
	Delete(ctx context.Context, txID string) error
}

// MovementReader lê o extrato e os saldos derivados dos movimentos de caixinha.
type MovementReader interface {
	ListByCaixinha(ctx context.Context, caixinhaID string, p shared.Pagination) ([]shared.CaixinhaMovement, int, error)
	BalanceByCaixinha(ctx context.Context, caixinhaID string) (int64, error)
	BalancesAll(ctx context.Context) (map[string]int64, error)
	OpeningMovementIDs(ctx context.Context, caixinhaID string) ([]string, error)
}

// DisponivelReader devolve o saldo disponível de caixa corrente (para o Overview).
type DisponivelReader interface {
	DisponivelAtual(ctx context.Context) (int64, error)
}
