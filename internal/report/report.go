// Package report materializa relatórios financeiros via fechamento mensal (snapshot).
// O domínio aqui é puro (sem I/O): matemática de referências/datas, estados de
// bloqueio do mês, rollup de agregados, comparativos, KPIs e insights.
package report

import (
	"fmt"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
)

// ─── Constantes de domínio ───────────────────────────────────────────────────

// hardLockDays é a janela de imutabilidade: a partir de +90 dias do último dia do
// mês, o mês fechado não aceita mais alterações (RF-REL-10).
const hardLockDays = 90

// ─── Estados do mês ──────────────────────────────────────────────────────────

// LockState é o estado de bloqueio de um mês (derivado, não armazenado).
type LockState string

const (
	StateOpen       LockState = "aberto"             // sem linha de fechamento
	StateAdjustable LockState = "fechado_ajustavel"  // fechado, hoje <= hard_lock_at
	StateBlocked    LockState = "fechado_bloqueado"  // fechado, hoje > hard_lock_at
)

// ─── Erros de domínio ────────────────────────────────────────────────────────

var (
	ErrMonthNotEnded = domainerr.NewConflict(
		"só é possível fechar após o último dia do mês", domainerr.WithDisplayable())
	ErrAlreadyClosed = domainerr.NewConflict(
		"mês já está fechado", domainerr.WithDisplayable())
	ErrNotClosed = domainerr.NewNotFound(
		"mês não está fechado", domainerr.WithDisplayable())
	ErrInvalidReference = domainerr.NewBadRequest(
		"referência inválida: use YYYY-MM", domainerr.WithDisplayable())
	// ErrMonthBlocked é consumido pelo módulo transaction ao tentar alterar um
	// lançamento de mês fechado-bloqueado (RF-REL-10). A spec sugere 422, mas a lib
	// domainerr só expõe 409 (Conflict) — conflito com o estado do recurso é coerente.
	ErrMonthBlocked = domainerr.NewConflict(
		"mês fechado e bloqueado para alterações após 90 dias", domainerr.WithDisplayable())
)

// ─── Referência (YYYY-MM) e matemática de datas (puras) ──────────────────────

// ParseReference valida e quebra uma referência YYYY-MM em ano/mês.
func ParseReference(ref string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", ref)
	if err != nil {
		return 0, 0, ErrInvalidReference
	}
	return t.Year(), t.Month(), nil
}

// FormatReference monta a referência YYYY-MM.
func FormatReference(year int, month time.Month) string {
	return fmt.Sprintf("%04d-%02d", year, int(month))
}

// ReferenceOf retorna a referência YYYY-MM de uma data YYYY-MM-DD.
func ReferenceOf(date string) (string, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", domainerr.NewBadRequest("data inválida: use YYYY-MM-DD", domainerr.WithDisplayable())
	}
	return FormatReference(t.Year(), t.Month()), nil
}

// MonthLastDay retorna o último dia do mês (YYYY-MM-DD), tratando meses curtos.
func MonthLastDay(ref string) (string, error) {
	y, m, err := ParseReference(ref)
	if err != nil {
		return "", err
	}
	// dia 0 do mês seguinte = último dia do mês atual.
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
	return last.Format("2006-01-02"), nil
}

// HardLockDate retorna a data (YYYY-MM-DD) a partir da qual o mês fica imutável:
// último dia do mês + 90 dias.
func HardLockDate(ref string) (string, error) {
	last, err := MonthLastDay(ref)
	if err != nil {
		return "", err
	}
	t, _ := time.Parse("2006-01-02", last)
	return t.AddDate(0, 0, hardLockDays).Format("2006-01-02"), nil
}

// MonthEnded informa se o último dia do mês já passou (em relação a `today` YYYY-MM-DD).
// Permite fechar a partir do dia seguinte ao último dia do mês.
func MonthEnded(ref, today string) (bool, error) {
	last, err := MonthLastDay(ref)
	if err != nil {
		return false, err
	}
	return today > last, nil // comparação lexicográfica de YYYY-MM-DD
}

// DeriveLockState deriva o estado a partir da existência de fechamento e de hoje.
// closed=false → aberto; senão compara hoje com hardLockAt (YYYY-MM-DD lexicográfico).
func DeriveLockState(closed bool, hardLockAt, today string) LockState {
	if !closed {
		return StateOpen
	}
	if today > hardLockAt {
		return StateBlocked
	}
	return StateAdjustable
}

// ─── Navegação de referências ────────────────────────────────────────────────

// PrevReference retorna o mês anterior (YYYY-MM).
func PrevReference(ref string) (string, error) {
	y, m, err := ParseReference(ref)
	if err != nil {
		return "", err
	}
	t := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	return FormatReference(t.Year(), t.Month()), nil
}

// SameMonthPrevYear retorna o mesmo mês do ano anterior.
func SameMonthPrevYear(ref string) (string, error) {
	y, m, err := ParseReference(ref)
	if err != nil {
		return "", err
	}
	return FormatReference(y-1, m), nil
}

// MonthsInQuarter retorna as 3 referências de um trimestre civil (q ∈ 1..4).
func MonthsInQuarter(year, q int) []string {
	start := (q-1)*3 + 1
	out := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, FormatReference(year, time.Month(start+i)))
	}
	return out
}

// MonthsInSemester retorna as 6 referências de um semestre (h ∈ 1..2).
func MonthsInSemester(year, h int) []string {
	start := (h-1)*6 + 1
	out := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		out = append(out, FormatReference(year, time.Month(start+i)))
	}
	return out
}

// MonthsInYear retorna as 12 referências de um ano.
func MonthsInYear(year int) []string {
	out := make([]string, 0, 12)
	for m := 1; m <= 12; m++ {
		out = append(out, FormatReference(year, time.Month(m)))
	}
	return out
}
