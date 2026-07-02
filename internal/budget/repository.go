package budget

import (
	"context"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Template é um conjunto reutilizável de destinos (sem mês).
type Template struct {
	ID        string
	Name      string
	Items     []TemplateItem
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TemplateItem é um destino dentro de um template.
type TemplateItem struct {
	ID                  string
	Name                string
	Kind                Kind
	Mode                Mode
	Percentage          *int
	FixedAmount         *int64
	PresetSubcategoryID *string
	PresetPaymentMethod *string
	PresetDescription   *string
	CaixinhaID          *string
	DisplayOrder        int
}

// ─── Repository (owner das tabelas de alocação) ──────────────────────────────

// Repository persiste destinos e templates.
type Repository interface {
	ListDestinations(ctx context.Context, reference string) ([]Destination, error)
	GetDestination(ctx context.Context, id string) (Destination, error)
	CreateDestination(ctx context.Context, d Destination) error
	CreateDestinations(ctx context.Context, ds []Destination) error
	UpdateDestination(ctx context.Context, d Destination) error
	DeleteDestination(ctx context.Context, id string) error
	// SetMaterialized vincula o destino a um lançamento (condicional: só se ainda não
	// materializado). ok=false se já estava materializado (idempotência / corrida).
	SetMaterialized(ctx context.Context, id, txID string, amount int64, at time.Time) (ok bool, err error)
	// ClearMaterialized desvincula (volta a planejado).
	ClearMaterialized(ctx context.Context, id string) error

	ListTemplates(ctx context.Context) ([]Template, error)
	GetTemplate(ctx context.Context, id string) (Template, error)
	CreateTemplate(ctx context.Context, t Template) error
}

// ─── Ports consumidos (injetados no main.go) ─────────────────────────────────

// IncomeReader lê a renda do mês (soma das receitas por competência) e se está toda
// realizada. Implementado pelo módulo transaction.
type IncomeReader interface {
	MonthIncome(ctx context.Context, reference string) (total int64, allRealized bool, items []shared.IncomeItem, err error)
}

// TransactionWriter cria/exclui lançamentos para materializar/desfazer destinos.
// Implementado pelo módulo transaction (passa pelos use cases → deriva tipo, valida
// e aplica o guard de mês fechado do report). Retorna o id do lançamento criado.
type TransactionWriter interface {
	Create(ctx context.Context, in shared.NewTransaction) (string, error)
	Delete(ctx context.Context, id string) error
}

// CaixinhaAporter registra um aporte numa caixinha (quando o destino aponta para uma).
// Implementado por transaction.CaixinhaWriter. Devolve o id do lançamento criado, que é
// guardado como materialização do destino (Undo o exclui via TransactionWriter.Delete).
type CaixinhaAporter interface {
	RegisterAporte(ctx context.Context, caixinhaID string, amount int64, date string, desc *string) (string, error)
}
