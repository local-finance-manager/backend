// Package patrimonio implementa a Gestão de Patrimônio: caixinhas (envelopes) de
// saldo persistente construído por aporte/resgate. O módulo é dono da tabela
// `caixinhas`; os movimentos (aporte/resgate) vivem no livro-caixa do módulo
// transaction e são acessados por ports (MovementWriter/MovementReader) — nunca
// por import direto. Ver ARCHITECTURE.md.
package patrimonio

import (
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"
)

// ─── Tipos / Enums ────────────────────────────────────────────────────────────

// CaixinhaType classifica o comportamento/metas de uma caixinha.
type CaixinhaType string

const (
	TypeReserva      CaixinhaType = "reserva"
	TypeObjetivo     CaixinhaType = "objetivo"
	TypeInvestimento CaixinhaType = "investimento"
)

var validTypes = map[CaixinhaType]struct{}{
	TypeReserva: {}, TypeObjetivo: {}, TypeInvestimento: {},
}

// Direções dos movimentos de caixinha (espelham subcategories.caixinha_direction).
const (
	DirectionAporte  = "aporte"
	DirectionResgate = "resgate"
)

// ─── Entidade ─────────────────────────────────────────────────────────────────

// Caixinha é o envelope de patrimônio. Saldo NÃO é persistido: é derivado dos
// movimentos (aportes − resgates), lido via MovementReader.
type Caixinha struct {
	ID               string
	Name             string
	Type             CaixinhaType
	MetaValor        *int64  // reserva/objetivo (centavos, > 0)
	DataAlvo         *string // objetivo (YYYY-MM-DD)
	ValorMercado     *int64  // investimento (centavos, >= 0)
	DataValorMercado *string // investimento (YYYY-MM-DD)
	Color            *string
	Icon             *string
	DisplayOrder     int
	Archived         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Progress devolve o progresso em pontos-base (0..10000) rumo à MetaValor, dado o
// saldo atual. Sem meta (nil ou <= 0) → nil (não se aplica). Teto em 10000 (100%).
func (c Caixinha) Progress(saldo int64) *int {
	if c.MetaValor == nil || *c.MetaValor <= 0 {
		return nil
	}
	bp := int(saldo * 10000 / *c.MetaValor)
	if bp < 0 {
		bp = 0
	}
	if bp > 10000 {
		bp = 10000
	}
	return &bp
}

// GanhoInvestimento devolve o ganho/perda aproximado (valorMercado − saldo aportado)
// para caixinha de investimento com valor de mercado informado. Sem valor → nil.
func (c Caixinha) GanhoInvestimento(saldo int64) *int64 {
	if c.Type != TypeInvestimento || c.ValorMercado == nil {
		return nil
	}
	g := *c.ValorMercado - saldo
	return &g
}

// ─── Inputs ───────────────────────────────────────────────────────────────────

// CreateCaixinhaInput carrega os campos para criar uma caixinha.
type CreateCaixinhaInput struct {
	Name         string
	Type         CaixinhaType
	MetaValor    *int64
	DataAlvo     *string
	ValorMercado *int64
	Color        *string
	Icon         *string
	DisplayOrder int
}

// UpdateCaixinhaInput carrega os campos mutáveis (PUT full-replace).
type UpdateCaixinhaInput struct {
	ID           string
	Name         string
	Type         CaixinhaType
	MetaValor    *int64
	DataAlvo     *string
	ValorMercado *int64
	Color        *string
	Icon         *string
	DisplayOrder int
}

// MarketValueInput atualiza o valor de mercado de uma caixinha de investimento.
type MarketValueInput struct {
	ID           string
	ValorMercado int64
	Data         string // YYYY-MM-DD
}

// MovementInput é o pedido de aporte/resgate.
type MovementInput struct {
	CaixinhaID  string
	Amount      int64
	Date        string // YYYY-MM-DD
	Description *string
}

// ─── Erros de domínio ─────────────────────────────────────────────────────────

var (
	ErrCaixinhaNotFound = domainerr.NewNotFound(
		"caixinha não encontrada", domainerr.WithDisplayable())
	ErrResgateExcedeSaldo = domainerr.NewConflict(
		"o resgate não pode ser maior que o saldo da caixinha", domainerr.WithDisplayable())
	ErrExcluirComSaldo = domainerr.NewConflict(
		"não é possível excluir uma caixinha com saldo; resgate tudo antes ou arquive", domainerr.WithDisplayable())
	ErrValorMercadoTipo = domainerr.NewConflict(
		"valor de mercado só se aplica a caixinhas de investimento", domainerr.WithDisplayable())
)

// ─── Validação ────────────────────────────────────────────────────────────────

const maxNameLen = 150

// ValidateCreate valida um CreateCaixinhaInput acumulando todos os erros.
func ValidateCreate(in CreateCaixinhaInput) error {
	return validateCommon(in.Name, in.Type, in.MetaValor, in.DataAlvo, in.ValorMercado)
}

// ValidateUpdate valida um UpdateCaixinhaInput (mesmas regras de formato).
func ValidateUpdate(in UpdateCaixinhaInput) error {
	acc := validation.NewAccumulator().
		Check(in.ID != "", "id é obrigatório")
	if err := acc.Result(); err != nil {
		return err
	}
	return validateCommon(in.Name, in.Type, in.MetaValor, in.DataAlvo, in.ValorMercado)
}

func validateCommon(name string, typ CaixinhaType, meta *int64, dataAlvo *string, valorMercado *int64) error {
	n := strings.TrimSpace(name)
	_, typeOK := validTypes[typ]

	acc := validation.NewAccumulator().
		Check(n != "", "nome é obrigatório").
		Check(len([]rune(n)) <= maxNameLen, "nome deve ter no máximo 150 caracteres").
		Check(typeOK, "tipo inválido: use reserva, objetivo ou investimento")

	if meta != nil {
		acc.Check(*meta > 0, "meta deve ser maior que zero")
	}
	if valorMercado != nil {
		acc.Check(*valorMercado >= 0, "valor de mercado não pode ser negativo")
	}
	if dataAlvo != nil && *dataAlvo != "" {
		acc.Check(isValidDate(*dataAlvo), "data alvo inválida: use YYYY-MM-DD")
	}
	return acc.Result()
}

// ValidateMovement valida um aporte/resgate.
func ValidateMovement(in MovementInput) error {
	acc := validation.NewAccumulator().
		Check(in.CaixinhaID != "", "caixinha é obrigatória").
		Check(in.Amount > 0, "valor deve ser maior que zero").
		Check(in.Date != "", "data é obrigatória")
	if in.Date != "" {
		acc.Check(isValidDate(in.Date), "data inválida: use YYYY-MM-DD")
	}
	return acc.Result()
}

// ValidateMarketValue valida a atualização de valor de mercado.
func ValidateMarketValue(in MarketValueInput) error {
	acc := validation.NewAccumulator().
		Check(in.ID != "", "id é obrigatório").
		Check(in.ValorMercado >= 0, "valor de mercado não pode ser negativo").
		Check(in.Data != "", "data é obrigatória")
	if in.Data != "" {
		acc.Check(isValidDate(in.Data), "data inválida: use YYYY-MM-DD")
	}
	return acc.Result()
}

// ValidateSaldoInicial valida a definição do saldo inicial (valor >= 0 permite limpar).
func ValidateSaldoInicial(caixinhaID string, valor int64, date string) error {
	acc := validation.NewAccumulator().
		Check(caixinhaID != "", "caixinha é obrigatória").
		Check(valor >= 0, "valor não pode ser negativo").
		Check(date != "", "data é obrigatória")
	if date != "" {
		acc.Check(isValidDate(date), "data inválida: use YYYY-MM-DD")
	}
	return acc.Result()
}

func isValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
