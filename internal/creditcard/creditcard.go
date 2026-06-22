package creditcard

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
	"github.com/Lucas-Lopes-II/govalidator/validation"
)

// ─── Enums ──────────────────────────────────────────────────────────────────

// Brand é a bandeira do cartão.
type Brand string

const (
	BrandVisa       Brand = "visa"
	BrandMastercard Brand = "mastercard"
	BrandElo        Brand = "elo"
	BrandAmex       Brand = "amex"
	BrandHipercard  Brand = "hipercard"
	BrandOutros     Brand = "outros"
)

var validBrands = map[Brand]struct{}{
	BrandVisa: {}, BrandMastercard: {}, BrandElo: {},
	BrandAmex: {}, BrandHipercard: {}, BrandOutros: {},
}

// InvoiceStatus é o status derivado de uma fatura.
type InvoiceStatus string

const (
	StatusFutura  InvoiceStatus = "futura"
	StatusAberta  InvoiceStatus = "aberta"
	StatusFechada InvoiceStatus = "fechada"
	StatusPaga    InvoiceStatus = "paga"
	StatusVencida InvoiceStatus = "vencida"
)

// UtilizationLevel classifica a faixa de utilização do limite.
type UtilizationLevel string

const (
	LevelSaudavel UtilizationLevel = "saudavel"
	LevelAtencao  UtilizationLevel = "atencao"
	LevelAlto     UtilizationLevel = "alto"
	LevelCritico  UtilizationLevel = "critico"
)

// ─── Entidade ───────────────────────────────────────────────────────────────

// CreditCard é a entidade-raiz. Valores monetários em centavos (int64).
type CreditCard struct {
	ID             string
	Name           string
	Brand          Brand
	LastFourDigits *string
	Issuer         *string
	CreditLimit    int64 // centavos, > 0
	ClosingDay     int   // 1..31
	DueDay         int   // 1..31
	Color          *string
	Icon           *string
	Archived       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DefaultIcon é o ícone aplicado quando o input não traz um.
const DefaultIcon = "credit-card"

// ─── Inputs ─────────────────────────────────────────────────────────────────

// CreateCreditCardInput carrega os dados validados para criação.
type CreateCreditCardInput struct {
	Name           string
	Brand          Brand
	LastFourDigits *string
	Issuer         *string
	CreditLimit    int64
	ClosingDay     int
	DueDay         int
	Color          *string
	Icon           *string
}

// UpdateCreditCardInput carrega os dados editáveis (PUT full-replace).
type UpdateCreditCardInput struct {
	ID             string
	Name           string
	Brand          Brand
	LastFourDigits *string
	Issuer         *string
	CreditLimit    int64
	ClosingDay     int
	DueDay         int
	Color          *string
	Icon           *string
}

// ─── Erros de domínio ───────────────────────────────────────────────────────

var (
	ErrCreditCardNotFound = domainerr.NewNotFound(
		"cartão de crédito não encontrado", domainerr.WithDisplayable())
	ErrCardHasTransactions = domainerr.NewConflict(
		"não é possível excluir um cartão com lançamentos vinculados; arquive-o", domainerr.WithDisplayable())
	// domainerr não expõe 422; usamos Conflict (409) — conflito com o estado do recurso.
	ErrCannotLinkArchivedCard = domainerr.NewConflict(
		"não é possível vincular lançamentos a um cartão arquivado", domainerr.WithDisplayable())
	ErrInvoiceNotClosed = domainerr.NewConflict(
		"a fatura ainda não fechou", domainerr.WithDisplayable())
	ErrInvoiceNotFound = domainerr.NewNotFound(
		"fatura não encontrada", domainerr.WithDisplayable())
	ErrInvoiceAlreadyPaid = domainerr.NewConflict(
		"esta fatura já está marcada como paga", domainerr.WithDisplayable())
)

// ─── Helpers de calendário (privados, puros) ────────────────────────────────

// daysInMonth retorna o número de dias de (year, month).
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// clampDay limita day ao último dia válido de (year, month). Ex.: dia 31 em fev → 28/29.
func clampDay(year int, month time.Month, day int) int {
	if last := daysInMonth(year, month); day > last {
		return last
	}
	return day
}

func dateString(year int, month time.Month, day int) string {
	return fmt.Sprintf("%04d-%02d-%02d", year, int(month), day)
}

func parseReference(ref string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", ref)
	if err != nil {
		return 0, 0, fmt.Errorf("creditcard: reference inválida %q: %w", ref, err)
	}
	return t.Year(), t.Month(), nil
}

func formatReference(year int, month time.Month) string {
	return fmt.Sprintf("%04d-%02d", year, int(month))
}

func nextMonth(year int, month time.Month) (int, time.Month) {
	if month == time.December {
		return year + 1, time.January
	}
	return year, month + 1
}

func prevMonth(year int, month time.Month) (int, time.Month) {
	if month == time.January {
		return year - 1, time.December
	}
	return year, month - 1
}

// ─── Regras de ciclo (RF-CC-04) ─────────────────────────────────────────────

// ClosingDate retorna a data de fechamento (YYYY-MM-DD) da fatura de uma reference
// (YYYY-MM), aplicando clamp de mês curto.
func ClosingDate(reference string, closingDay int) (string, error) {
	y, m, err := parseReference(reference)
	if err != nil {
		return "", err
	}
	return dateString(y, m, clampDay(y, m, closingDay)), nil
}

// DueDate retorna a data de vencimento. Se dueDay <= closingDay, vence no mês SEGUINTE
// ao fechamento; senão, no mesmo mês. Clamp aplicado.
func DueDate(reference string, closingDay, dueDay int) (string, error) {
	y, m, err := parseReference(reference)
	if err != nil {
		return "", err
	}
	dy, dm := y, m
	if dueDay <= closingDay {
		dy, dm = nextMonth(y, m)
	}
	return dateString(dy, dm, clampDay(dy, dm, dueDay)), nil
}

// CycleStart retorna o início do ciclo (dia seguinte ao fechamento ANTERIOR), YYYY-MM-DD.
func CycleStart(reference string, closingDay int) (string, error) {
	y, m, err := parseReference(reference)
	if err != nil {
		return "", err
	}
	py, pm := prevMonth(y, m)
	prevClosing := clampDay(py, pm, closingDay)
	start := time.Date(py, pm, prevClosing, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return start.Format("2006-01-02"), nil
}

// InvoiceReferenceFor retorna a reference (YYYY-MM) da fatura a que uma compra em
// purchaseDate (YYYY-MM-DD) pertence: "primeira data de fechamento >= purchaseDate".
func InvoiceReferenceFor(purchaseDate string, closingDay int) (string, error) {
	t, err := time.Parse("2006-01-02", purchaseDate)
	if err != nil {
		return "", fmt.Errorf("creditcard: data de compra inválida %q: %w", purchaseDate, err)
	}
	y, m, d := t.Year(), t.Month(), t.Day()
	if d <= clampDay(y, m, closingDay) {
		return formatReference(y, m), nil
	}
	ny, nm := nextMonth(y, m)
	return formatReference(ny, nm), nil
}

// BestPurchaseDay: dia seguinte ao fechamento. Para closingDay >= 28 retorna 1, pois
// o "dia seguinte" rola para o início do mês seguinte no pior caso (aproximação estável,
// puramente cosmética — não afeta cálculo financeiro).
func BestPurchaseDay(closingDay int) int {
	if closingDay >= 28 {
		return 1
	}
	return closingDay + 1
}

// DeriveInvoiceStatus deriva o status a partir de hoje, das datas do ciclo e do pagamento.
// Comparação lexicográfica de strings YYYY-MM-DD (formato é zero-padded e ordenável).
func DeriveInvoiceStatus(today, cycleStart, closingDate, dueDate string, hasPayment bool) InvoiceStatus {
	if hasPayment {
		return StatusPaga
	}
	switch {
	case today < cycleStart:
		return StatusFutura
	case today <= closingDate:
		return StatusAberta
	case today <= dueDate:
		return StatusFechada
	default:
		return StatusVencida
	}
}

// ─── Utilização (RF-CC-07) ──────────────────────────────────────────────────

// UtilizationPercent retorna o percentual inteiro de uso do limite (0 se limite <= 0).
// Pode passar de 100 se o uso estourar o limite.
func UtilizationPercent(usedLimit, creditLimit int64) int {
	if creditLimit <= 0 {
		return 0
	}
	return int(usedLimit * 100 / creditLimit)
}

// ClassifyUtilization classifica o percentual: <30 saudavel; 30..69 atencao;
// 70..90 alto; >90 critico.
func ClassifyUtilization(percent int) UtilizationLevel {
	switch {
	case percent < 30:
		return LevelSaudavel
	case percent < 70:
		return LevelAtencao
	case percent <= 90:
		return LevelAlto
	default:
		return LevelCritico
	}
}

// ─── Validação (acumulador — guia §5.1) ─────────────────────────────────────

const (
	maxNameLen   = 80
	maxIssuerLen = 60
)

var (
	colorRegex    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	lastFourRegex = regexp.MustCompile(`^[0-9]{4}$`)
)

// ValidateCreate valida um CreateCreditCardInput acumulando todos os erros.
func ValidateCreate(in CreateCreditCardInput) error {
	return validateCard(in.Name, in.Brand, in.LastFourDigits, in.Issuer,
		in.CreditLimit, in.ClosingDay, in.DueDay, in.Color)
}

// ValidateUpdate aplica as mesmas regras do create.
func ValidateUpdate(in UpdateCreditCardInput) error {
	return validateCard(in.Name, in.Brand, in.LastFourDigits, in.Issuer,
		in.CreditLimit, in.ClosingDay, in.DueDay, in.Color)
}

func validateCard(name string, brand Brand, lastFour, issuer *string,
	creditLimit int64, closingDay, dueDay int, color *string) error {
	trimmed := strings.TrimSpace(name)
	_, brandOK := validBrands[brand]

	acc := validation.NewAccumulator().
		Check(trimmed != "", "nome é obrigatório").
		Check(len([]rune(trimmed)) <= maxNameLen, "nome deve ter no máximo 80 caracteres").
		Check(brandOK, "bandeira inválida").
		Check(creditLimit > 0, "limite do cartão deve ser maior que zero").
		Check(closingDay >= 1 && closingDay <= 31, "dia de fechamento inválido").
		Check(dueDay >= 1 && dueDay <= 31, "dia de vencimento inválido")

	if lastFour != nil {
		acc.Check(lastFourRegex.MatchString(*lastFour), "últimos 4 dígitos devem ser 4 números")
	}
	if issuer != nil {
		acc.Check(len([]rune(*issuer)) <= maxIssuerLen, "emissor deve ter no máximo 60 caracteres")
	}
	if color != nil {
		acc.Check(colorRegex.MatchString(*color), "cor deve estar no formato #RRGGBB")
	}

	return acc.Result()
}
