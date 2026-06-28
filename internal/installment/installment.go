// Package installment modela compras parceladas no cartão de crédito. Cada parcela é
// um lançamento normal na tabela transactions (com credit_card_id, competência própria,
// status), e o grupo (installment_groups) guarda o metadado imutável da compra. O domínio
// aqui é puro (sem I/O): rateio de centavos, cronograma de competências, juros, status.
package installment

import (
	"fmt"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"
)

// ─── Enums e constantes ─────────────────────────────────────────────────────

// InputMode indica como o usuário informou os valores.
type InputMode string

const (
	ByTotal       InputMode = "by_total"
	ByInstallment InputMode = "by_installment"
)

// GroupStatus é o status derivado de um grupo de parcelamento.
type GroupStatus string

const (
	GroupActive    GroupStatus = "ativo"
	GroupSettled   GroupStatus = "quitado"
	GroupCancelled GroupStatus = "cancelado"
)

// statuses das parcelas (espelham transaction.TransactionStatus como string neutra).
const (
	parcelaPending   = "pendente"
	parcelaRealized  = "realizado"
	parcelaCancelled = "cancelado"
)

const (
	minInstallments = 2
	maxInstallments = 72
	maxTitleLen     = 150
	maxDescLen      = 1000

	expenseType = "despesa" // type exigido da subcategoria (RF-PARC-01)
)

// ─── Entidade e projeções ───────────────────────────────────────────────────

// InstallmentGroup é o metadado imutável da compra parcelada.
type InstallmentGroup struct {
	ID                string
	CreditCardID      string
	SubcategoryID     string
	Title             string
	Description       *string
	TotalAmount       int64
	PrincipalAmount   *int64
	InstallmentsCount int
	PurchaseDate      string // YYYY-MM-DD
	FirstReference    string // YYYY-MM
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Parcela é uma parcela a ser inserida (o repositório compõe o restante a partir do grupo).
type Parcela struct {
	ID             string
	Number         int    // k (1-based)
	Amount         int64  // centavos
	CompetenceDate string // YYYY-MM-DD
}

// Installment é uma parcela lida do banco (para detalhe do grupo).
type Installment struct {
	TransactionID  string
	Number         int
	Amount         int64
	CompetenceDate string
	Reference      string // YYYY-MM, resolvido no serviço para exibição
	Status         string // pendente|realizado|cancelado
}

// PlannedInstallment é uma parcela calculada no preview (antes de persistir).
type PlannedInstallment struct {
	Number         int
	Amount         int64
	CompetenceDate string
	Reference      string
}

// Plan é o resultado do preview (sem persistência).
type Plan struct {
	TotalAmount       int64
	InstallmentsCount int
	InterestAmount    int64
	Installments      []PlannedInstallment
}

// GroupDetail é o grupo com suas parcelas e indicadores derivados.
type GroupDetail struct {
	InstallmentGroup
	InterestAmount  int64
	PaidCount       int
	RemainingCount  int
	RemainingAmount int64
	Status          GroupStatus
	Installments    []Installment
}

// GroupSummary é a linha da listagem (grupo + agregados das parcelas).
type GroupSummary struct {
	Group           InstallmentGroup
	PaidCount       int
	RemainingCount  int
	CancelledCount  int
	RemainingAmount int64
	Status          GroupStatus
}

// Filter são os filtros da listagem de grupos.
type Filter struct {
	CreditCardID *string
	Status       *GroupStatus
}

// ─── Inputs de mutação ──────────────────────────────────────────────────────

// CreateInput carrega os dados para criar (ou simular) uma compra parcelada.
type CreateInput struct {
	CreditCardID      string
	SubcategoryID     string
	Title             string
	Description       *string
	InstallmentsCount int
	InputMode         InputMode
	TotalAmount       int64  // by_total
	InstallmentAmount int64  // by_installment
	PrincipalAmount   *int64 // opcional (juros)
	PurchaseDate      string // YYYY-MM-DD
}

// UpdateSeriesInput carrega os metadados editáveis da série (RF-PARC-07).
type UpdateSeriesInput struct {
	ID            string
	Title         string
	Description   *string
	SubcategoryID string
}

// ─── Erros de domínio ───────────────────────────────────────────────────────

var (
	ErrInstallmentGroupNotFound = domainerr.NewNotFound(
		"compra parcelada não encontrada", domainerr.WithDisplayable())
	ErrOnlyExpensesInstallable = domainerr.NewConflict(
		"só é possível parcelar despesas", domainerr.WithDisplayable())
	ErrImmutableSeriesField = domainerr.NewBadRequest(
		"para alterar valor, parcelas ou data, recrie a compra parcelada", domainerr.WithDisplayable())
	ErrInstallmentHasPaidParcelas = domainerr.NewConflict(
		"não é possível excluir: há parcelas pagas. Cancele as parcelas restantes (as pagas são preservadas).",
		domainerr.WithDisplayable())
)

// ─── Rateio exato de centavos (RNF-PARC-02) ─────────────────────────────────

// ComputeAmounts distribui total em n parcelas; as `resto` primeiras recebem base+1.
// Invariante: soma(ComputeAmounts(total, n)) == total, para qualquer total e n.
func ComputeAmounts(total int64, n int) []int64 {
	base := total / int64(n)
	resto := total % int64(n)
	out := make([]int64, n)
	for i := range out {
		out[i] = base
		if int64(i) < resto {
			out[i]++
		}
	}
	return out
}

// ResolvePlan deriva (total, valores das parcelas) a partir do modo de entrada.
func ResolvePlan(mode InputMode, totalAmount, installmentAmount int64, n int) (int64, []int64, error) {
	switch mode {
	case ByTotal:
		return totalAmount, ComputeAmounts(totalAmount, n), nil
	case ByInstallment:
		total := installmentAmount * int64(n)
		amounts := make([]int64, n)
		for i := range amounts {
			amounts[i] = installmentAmount
		}
		return total, amounts, nil
	default:
		return 0, nil, domainerr.NewBadRequest("modo de entrada inválido", domainerr.WithDisplayable())
	}
}

// Interest calcula o juro absoluto (total − principal) quando há principal informado.
func Interest(total int64, principal *int64) int64 {
	if principal != nil && *principal > 0 && total > *principal {
		return total - *principal
	}
	return 0
}

// ─── Cronograma de competências (RNF-PARC-01/04) ────────────────────────────

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// addMonthsClamp soma `months` a uma data YYYY-MM-DD, clampando o dia ao último dia
// do mês de destino (ex.: 31/jan +1 mês → 28/29-fev).
func addMonthsClamp(date string, months int) (string, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("installment: data inválida %q: %w", date, err)
	}
	y, m, d := t.Date()
	idx := int(m) - 1 + months
	ny := y + idx/12
	nm := time.Month(idx%12) + 1
	if last := daysInMonth(ny, nm); d > last {
		d = last
	}
	return fmt.Sprintf("%04d-%02d-%02d", ny, int(nm), d), nil
}

// CompetenceSchedule devolve as N competências: parcela k → purchaseDate + (k-1) meses.
func CompetenceSchedule(purchaseDate string, n int) ([]string, error) {
	out := make([]string, n)
	for k := 0; k < n; k++ {
		d, err := addMonthsClamp(purchaseDate, k)
		if err != nil {
			return nil, err
		}
		out[k] = d
	}
	return out, nil
}

// ─── Status do grupo (RF-PARC-06) ───────────────────────────────────────────

// StatusCounts são as contagens de parcelas por status num grupo.
type StatusCounts struct {
	Pending   int
	Realized  int
	Cancelled int
}

// DeriveGroupStatus deriva o status do grupo a partir das contagens.
func DeriveGroupStatus(c StatusCounts) GroupStatus {
	switch {
	case c.Pending > 0:
		return GroupActive
	case c.Cancelled > 0:
		return GroupCancelled
	default:
		return GroupSettled
	}
}

// ─── Validação (acumulador — guia §5.1) ─────────────────────────────────────

// ValidateCreate valida o input puro (regras de I/O — despesa, cartão — ficam no serviço).
func ValidateCreate(in CreateInput) error {
	title := strings.TrimSpace(in.Title)
	acc := validation.NewAccumulator().
		Check(title != "", "título é obrigatório").
		Check(len([]rune(title)) <= maxTitleLen, "título deve ter no máximo 150 caracteres").
		Check(in.CreditCardID != "", "cartão é obrigatório").
		Check(in.SubcategoryID != "", "subcategoria é obrigatória").
		Check(in.InstallmentsCount >= minInstallments && in.InstallmentsCount <= maxInstallments,
			"número de parcelas deve estar entre 2 e 72").
		Check(in.PurchaseDate != "", "data da compra é obrigatória")

	if in.PurchaseDate != "" {
		acc.Check(isValidDate(in.PurchaseDate), "data da compra inválida: use YYYY-MM-DD")
	}
	switch in.InputMode {
	case ByTotal:
		acc.Check(in.TotalAmount > 0, "valor total deve ser maior que zero")
	case ByInstallment:
		acc.Check(in.InstallmentAmount > 0, "valor da parcela deve ser maior que zero")
	default:
		acc.Check(false, "modo de entrada inválido: use by_total ou by_installment")
	}
	if in.Description != nil {
		acc.Check(len([]rune(*in.Description)) <= maxDescLen, "descrição deve ter no máximo 1.000 caracteres")
	}
	if in.PrincipalAmount != nil {
		acc.Check(*in.PrincipalAmount > 0, "valor à vista deve ser maior que zero")
	}
	return acc.Result()
}

// ValidateUpdateSeries valida a edição de metadados da série.
func ValidateUpdateSeries(in UpdateSeriesInput) error {
	title := strings.TrimSpace(in.Title)
	acc := validation.NewAccumulator().
		Check(title != "", "título é obrigatório").
		Check(len([]rune(title)) <= maxTitleLen, "título deve ter no máximo 150 caracteres").
		Check(in.SubcategoryID != "", "subcategoria é obrigatória")
	if in.Description != nil {
		acc.Check(len([]rune(*in.Description)) <= maxDescLen, "descrição deve ter no máximo 1.000 caracteres")
	}
	return acc.Result()
}

func isValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
