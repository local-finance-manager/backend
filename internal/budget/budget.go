// Package budget implementa a alocação de receitas: o usuário distribui a renda
// do mês em "destinos" (despesa ou investimento, por percentual ou valor fixo) e os
// materializa em lançamentos reais. O domínio aqui é puro (sem I/O): rateio exato de
// centavos, soma alocada/não-alocada e validação do limite de 100%.
package budget

import (
	"sort"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"
)

// percentBaseFull é 100,00% em pontos-base (10000 = 100%).
const percentBaseFull = 10000

const maxNameLen = 150

// ─── Enums ───────────────────────────────────────────────────────────────────

// Kind é o tipo do destino.
type Kind string

const (
	KindDespesa      Kind = "despesa"
	KindInvestimento Kind = "investimento"
)

// Mode é o modo de cálculo do destino.
type Mode string

const (
	ModePercentual Mode = "percentual"
	ModeValorFixo  Mode = "valor_fixo"
)

var validKinds = map[Kind]struct{}{KindDespesa: {}, KindInvestimento: {}}
var validModes = map[Mode]struct{}{ModePercentual: {}, ModeValorFixo: {}}

// ─── Entidade ────────────────────────────────────────────────────────────────

// Destination é um destino do plano mensal de alocação.
type Destination struct {
	ID                  string
	Reference           string
	Name                string
	Kind                Kind
	Mode                Mode
	Percentage          *int   // pontos-base (10000 = 100%); nil se valor_fixo
	FixedAmount         *int64 // centavos; nil se percentual
	PresetSubcategoryID *string
	PresetPaymentMethod *string
	PresetDescription   *string
	CaixinhaID          *string // se setado, materializa como APORTE nesta caixinha
	DisplayOrder        int
	MaterializedTxID    *string
	MaterializedAmount  *int64
	MaterializedAt      *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// IsMaterialized informa se o destino já virou lançamento.
func (d Destination) IsMaterialized() bool { return d.MaterializedTxID != nil }

// Inputs de criação/edição (modo + valores).
type DestinationInput struct {
	Reference           string
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

// ─── Erros de domínio ────────────────────────────────────────────────────────

var (
	ErrDestinationNotFound = domainerr.NewNotFound(
		"destino não encontrado", domainerr.WithDisplayable())
	ErrIncomePending = domainerr.NewConflict(
		"só é possível gerar lançamentos quando todas as receitas do mês estiverem realizadas",
		domainerr.WithDisplayable())
	ErrOverAllocated = domainerr.NewConflict(
		"a soma das alocações não pode exceder a renda do mês", domainerr.WithDisplayable())
	ErrAlreadyMaterialized = domainerr.NewConflict(
		"este destino já virou um lançamento", domainerr.WithDisplayable())
	ErrNotMaterialized = domainerr.NewConflict(
		"este destino ainda não foi materializado", domainerr.WithDisplayable())
	ErrTemplateNotFound = domainerr.NewNotFound(
		"template não encontrado", domainerr.WithDisplayable())
	ErrMissingPreset = domainerr.NewConflict(
		"destino sem subcategoria de preset não pode ser materializado em lote", domainerr.WithDisplayable())
)

// ─── Validação ───────────────────────────────────────────────────────────────

// ValidateDestination valida os campos de um destino (sem checar limite de 100%,
// que depende dos demais destinos do mês — ver ValidateAllocation).
func ValidateDestination(in DestinationInput) error {
	name := strings.TrimSpace(in.Name)
	_, kindOK := validKinds[in.Kind]
	_, modeOK := validModes[in.Mode]

	acc := validation.NewAccumulator().
		Check(name != "", "nome é obrigatório").
		Check(len([]rune(name)) <= maxNameLen, "nome deve ter no máximo 150 caracteres").
		Check(kindOK, "tipo inválido: use despesa ou investimento").
		Check(modeOK, "modo inválido: use percentual ou valor_fixo")

	switch in.Mode {
	case ModePercentual:
		acc.Check(in.Percentage != nil && *in.Percentage >= 1 && *in.Percentage <= percentBaseFull,
			"percentual deve estar entre 0,01% e 100%")
	case ModeValorFixo:
		acc.Check(in.FixedAmount != nil && *in.FixedAmount > 0, "valor fixo deve ser maior que zero")
	}
	return acc.Result()
}

// ─── Cálculo (rateio exato de centavos) ──────────────────────────────────────

// Computed é o valor calculado de um destino.
type Computed struct {
	DestinationID string
	Amount        int64
}

// PlanComputation é o resultado do cálculo de um plano: valor por destino + totais.
type PlanComputation struct {
	Base              int64
	ByDestination     map[string]int64 // destinationID → valor em centavos
	AllocatedAmount   int64
	UnallocatedAmount int64
	AllocatedPercent  int // pontos-base sobre a base
}

// ComputePlan calcula o valor de cada destino sobre a base (renda total do mês).
//
// Os percentuais incidem sobre a BASE INTEIRA (Apêndice C: valor =
// round(percentage/10000 × base)); os destinos de valor fixo são o próprio
// fixedAmount. A soma dos valores percentuais é distribuída centavo a centavo
// (maior-resto) para bater exatamente o total percentual aplicado à base.
func ComputePlan(base int64, dests []Destination) PlanComputation {
	by := make(map[string]int64, len(dests))

	var sumPct int
	percentual := make([]Destination, 0, len(dests))
	for _, d := range dests {
		switch d.Mode {
		case ModeValorFixo:
			amt := int64(0)
			if d.FixedAmount != nil {
				amt = *d.FixedAmount
			}
			by[d.ID] = amt
		case ModePercentual:
			pct := 0
			if d.Percentage != nil {
				pct = *d.Percentage
			}
			sumPct += pct
			percentual = append(percentual, d)
		}
	}

	// total a distribuir entre os percentuais = (sumPct/10000) × base (arredondado).
	totalPercentValue := roundDiv(base*int64(sumPct), percentBaseFull)
	distributePercent(by, percentual, totalPercentValue, sumPct)

	var allocated int64
	for _, d := range dests {
		allocated += by[d.ID]
	}
	unalloc := base - allocated
	if unalloc < 0 {
		unalloc = 0
	}
	return PlanComputation{
		Base:              base,
		ByDestination:     by,
		AllocatedAmount:   allocated,
		UnallocatedAmount: unalloc,
		AllocatedPercent:  pctBP(allocated, base),
	}
}

// distributePercent rateia totalPercentValue entre os destinos percentuais
// proporcionalmente ao percentage de cada um, com método do maior-resto (cent-exato).
func distributePercent(by map[string]int64, percentual []Destination, totalPercentValue int64, sumPct int) {
	if len(percentual) == 0 || sumPct == 0 || totalPercentValue == 0 {
		for _, d := range percentual {
			by[d.ID] = 0
		}
		return
	}
	type rem struct {
		id  string
		rem int64
	}
	rems := make([]rem, 0, len(percentual))
	var assigned int64
	for _, d := range percentual {
		pct := int64(0)
		if d.Percentage != nil {
			pct = int64(*d.Percentage)
		}
		numer := totalPercentValue * pct
		v := numer / int64(sumPct)
		by[d.ID] = v
		assigned += v
		rems = append(rems, rem{id: d.ID, rem: numer % int64(sumPct)})
	}
	leftover := totalPercentValue - assigned
	// distribui os centavos restantes aos maiores restos (tie-break por id estável).
	sort.SliceStable(rems, func(i, j int) bool {
		if rems[i].rem != rems[j].rem {
			return rems[i].rem > rems[j].rem
		}
		return rems[i].id < rems[j].id
	})
	for i := int64(0); i < leftover && int(i) < len(rems); i++ {
		by[rems[i].id]++
	}
}

// ValidateAllocation impõe o limite de 100% (A1): a soma das alocações não pode
// exceder a renda. Como os percentuais incidem sobre a base inteira, o total alocado
// = fixos + (sumPct/10000 × base) e não pode passar da base; além disso, a soma dos
// percentuais sozinha não pode exceder 100%.
func ValidateAllocation(base int64, dests []Destination) error {
	var fixedTotal int64
	var sumPct int
	for _, d := range dests {
		switch d.Mode {
		case ModeValorFixo:
			if d.FixedAmount != nil {
				fixedTotal += *d.FixedAmount
			}
		case ModePercentual:
			if d.Percentage != nil {
				sumPct += *d.Percentage
			}
		}
	}
	if sumPct > percentBaseFull {
		return ErrOverAllocated
	}
	pctValue := roundDiv(base*int64(sumPct), percentBaseFull)
	if fixedTotal+pctValue > base {
		return ErrOverAllocated
	}
	return nil
}

// ─── helpers numéricos ───────────────────────────────────────────────────────

// roundDiv divide a/b arredondando para o inteiro mais próximo (a,b >= 0).
func roundDiv(a, b int64) int64 {
	if b == 0 {
		return 0
	}
	return (a + b/2) / b
}

// pctBP retorna part/whole em pontos-base (10000 = 100%); 0 se whole == 0.
func pctBP(part, whole int64) int {
	if whole == 0 {
		return 0
	}
	return int(part * percentBaseFull / whole)
}
