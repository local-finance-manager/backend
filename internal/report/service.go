package report

import (
	"context"
	"time"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Deps são as dependências do serviço (injetadas no main.go).
type Deps struct {
	Repo     Repository
	Realized RealizedAggregator
	Pending  PendingAggregator
	Tree     CategoryTreeReader
	Payments PaymentBreakdownReader
	Cash     CashAggregator
}

// Service orquestra fechamento, recálculo, lock e leitura dos relatórios.
type Service struct {
	repo     Repository
	realized RealizedAggregator
	pending  PendingAggregator
	tree     CategoryTreeReader
	payments PaymentBreakdownReader
	cash     CashAggregator
	now      func() time.Time
}

// NewService cria o serviço.
func NewService(d Deps) *Service {
	return &Service{
		repo: d.Repo, realized: d.Realized, pending: d.Pending,
		tree: d.Tree, payments: d.Payments, cash: d.Cash, now: time.Now,
	}
}

// RegimeCaixa é a lente padrão (por data de pagamento); RegimeCompetencia usa snapshot.
const (
	RegimeCaixa       = "caixa"
	RegimeCompetencia = "competencia"
)

// normalizeRegime devolve o regime válido (default: caixa).
func normalizeRegime(regime string) string {
	if regime == RegimeCompetencia {
		return RegimeCompetencia
	}
	return RegimeCaixa
}

func (s *Service) today() string { return s.now().UTC().Format("2006-01-02") }

// ─── Fechamento / Recálculo (RF-REL-03..09) ──────────────────────────────────

// Close congela o mês em snapshot (apenas realizado). Só após o último dia do mês.
func (s *Service) Close(ctx context.Context, reference string) (Closing, error) {
	if _, _, err := ParseReference(reference); err != nil {
		return Closing{}, err
	}
	ended, err := MonthEnded(reference, s.today())
	if err != nil {
		return Closing{}, err
	}
	if !ended {
		return Closing{}, ErrMonthNotEnded
	}
	if _, exists, err := s.repo.GetClosing(ctx, reference); err != nil {
		return Closing{}, err
	} else if exists {
		return Closing{}, ErrAlreadyClosed
	}
	return s.writeClosing(ctx, reference, false)
}

// Recalculate regenera o snapshot de um mês JÁ fechado (idempotente). Mês aberto
// não tem snapshot → no-op silencioso.
func (s *Service) Recalculate(ctx context.Context, reference string) error {
	existing, exists, err := s.repo.GetClosing(ctx, reference)
	if err != nil {
		return err
	}
	if !exists {
		return nil // mês aberto: nada a recalcular
	}
	_, err = s.writeClosingPreserving(ctx, reference, existing)
	return err
}

// writeClosing computa os agregados realizados do mês e grava o fechamento.
func (s *Service) writeClosing(ctx context.Context, reference string, _ bool) (Closing, error) {
	aggs, totals, err := s.realized.AggregateMonth(ctx, reference)
	if err != nil {
		return Closing{}, domainerr.NewInternal("erro ao agregar lançamentos do mês")
	}
	lastDay, _ := MonthLastDay(reference)
	hardLock, _ := HardLockDate(reference)
	now := s.now().UTC()
	c := Closing{
		Reference:    reference,
		ClosedAt:     now,
		MonthLastDay: lastDay,
		HardLockAt:   hardLock,
		Totals:       totals,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.SaveClosing(ctx, c, aggs); err != nil {
		return Closing{}, domainerr.NewInternal("erro ao salvar fechamento")
	}
	return c, nil
}

// writeClosingPreserving recalcula mantendo closedAt/createdAt/hardLock do fechamento.
func (s *Service) writeClosingPreserving(ctx context.Context, reference string, prev Closing) (Closing, error) {
	aggs, totals, err := s.realized.AggregateMonth(ctx, reference)
	if err != nil {
		return Closing{}, domainerr.NewInternal("erro ao agregar lançamentos do mês")
	}
	now := s.now().UTC()
	c := prev
	c.Totals = totals
	c.RecalculatedAt = &now
	c.UpdatedAt = now
	if err := s.repo.SaveClosing(ctx, c, aggs); err != nil {
		return Closing{}, domainerr.NewInternal("erro ao recalcular fechamento")
	}
	return c, nil
}

// ─── Lock (consumido pelo módulo transaction via guard) ──────────────────────

// LockState retorna o estado de bloqueio do mês da competência informada.
func (s *Service) LockState(ctx context.Context, competenceDate string) (LockState, error) {
	ref, err := ReferenceOf(competenceDate)
	if err != nil {
		return "", err
	}
	c, exists, err := s.repo.GetClosing(ctx, ref)
	if err != nil {
		return "", err
	}
	if !exists {
		return StateOpen, nil
	}
	return DeriveLockState(true, c.HardLockAt, s.today()), nil
}

// EnsureEditable rejeita alterações em mês fechado-bloqueado (≥90 dias). Demais
// estados são permitidos (ajustável recalcula via AfterChange).
func (s *Service) EnsureEditable(ctx context.Context, competenceDate string) error {
	st, err := s.LockState(ctx, competenceDate)
	if err != nil {
		return err
	}
	if st == StateBlocked {
		return ErrMonthBlocked
	}
	return nil
}

// AfterChange recalcula o snapshot dos meses fechados-ajustáveis tocados por uma
// alteração de lançamento (origem e/ou destino quando a competência muda).
func (s *Service) AfterChange(ctx context.Context, competenceDates ...string) error {
	seen := map[string]bool{}
	for _, d := range competenceDates {
		if d == "" {
			continue
		}
		ref, err := ReferenceOf(d)
		if err != nil {
			continue
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		st, err := s.LockState(ctx, d)
		if err != nil {
			return err
		}
		if st == StateAdjustable {
			if err := s.Recalculate(ctx, ref); err != nil {
				return err
			}
		}
	}
	return nil
}

// ─── Leitura: listagem de fechamentos ────────────────────────────────────────

// ClosingView é o estado de um mês fechado para a UI.
type ClosingView struct {
	Reference  string    `json:"reference"`
	Status     LockState `json:"status"`
	ClosedAt   string    `json:"closedAt"`
	HardLockAt string    `json:"hardLockAt"`
}

// ListClosings lista os meses fechados e seus estados atuais.
func (s *Service) ListClosings(ctx context.Context) ([]ClosingView, error) {
	cs, err := s.repo.ListClosings(ctx)
	if err != nil {
		return nil, domainerr.NewInternal("erro ao listar fechamentos")
	}
	today := s.today()
	out := make([]ClosingView, len(cs))
	for i, c := range cs {
		out[i] = ClosingView{
			Reference:  c.Reference,
			Status:     DeriveLockState(true, c.HardLockAt, today),
			ClosedAt:   c.ClosedAt.UTC().Format(time.RFC3339),
			HardLockAt: c.HardLockAt,
		}
	}
	return out, nil
}

// ─── Leitura: relatório mensal ───────────────────────────────────────────────

// Monthly monta o relatório de um mês, no regime (caixa/competência) e modo
// (realizado/projetivo) escolhidos. Padrão: caixa + realizado.
func (s *Service) Monthly(ctx context.Context, reference, mode, regime string) (Report, error) {
	if _, _, err := ParseReference(reference); err != nil {
		return Report{}, err
	}
	regime = normalizeRegime(regime)
	lk, err := s.lookup(ctx)
	if err != nil {
		return Report{}, err
	}
	first, last, _ := monthDayBounds(reference)
	status := s.monthLockState(ctx, reference)

	var aggs []shared.SubcategoryAggregate
	var totals shared.MonthlyTotals
	var payMap map[string]int64

	if regime == RegimeCompetencia {
		var derr error
		aggs, totals, _, derr = s.monthData(ctx, reference)
		if derr != nil {
			return Report{}, derr
		}
		payMap, _ = s.payments.PaymentBreakdownMonth(ctx, reference)
	} else {
		var aerr error
		aggs, totals, aerr = s.cash.AggregateCashPeriod(ctx, first, last)
		if aerr != nil {
			return Report{}, domainerr.NewInternal("erro ao agregar o caixa do mês")
		}
		payMap, _ = s.cash.PaymentBreakdownCash(ctx, first, last)
	}

	percentCred := pct(payMap["cartao_credito"], totals.Despesas)
	rep := Report{
		Scope:          "monthly",
		Reference:      reference,
		Regime:         regime,
		Mode:           "realizado",
		Status:         status,
		KPIs:           BuildKPIs(totals, aggs, percentCred),
		Analitico:      BuildAnalitico(aggs, lk),
		PaymentMethods: paymentSlices(payMap),
	}

	prevRef, _ := PrevReference(reference)
	yoyRef, _ := SameMonthPrevYear(reference)
	var prevTotals, yoyTotals shared.MonthlyTotals
	var prevPartial, yoyPartial bool
	var prevAggs []shared.SubcategoryAggregate
	if regime == RegimeCompetencia {
		var prevClosed, yoyClosed bool
		prevTotals, prevClosed, prevAggs = s.refTotals(ctx, prevRef)
		yoyTotals, yoyClosed, _ = s.refTotals(ctx, yoyRef)
		prevPartial, yoyPartial = !prevClosed, !yoyClosed
	} else {
		// caixa é ao vivo — nunca "parcial" por fechamento
		prevTotals, prevAggs = s.cashMonthTotals(ctx, prevRef)
		yoyTotals, _ = s.cashMonthTotals(ctx, yoyRef)
	}
	rep.Comparativos = Comparativos{
		PeriodoAnterior:         BuildComparison(prevRef, prevPartial, totals, prevTotals),
		MesmoPeriodoAnoAnterior: BuildComparison(yoyRef, yoyPartial, totals, yoyTotals),
	}
	rep.Insights = BuildInsights(rep.Analitico, rep.KPIs, rep.Comparativos.PeriodoAnterior, despesaByCat(prevAggs))

	if mode == "projetivo" {
		var pTotals shared.MonthlyTotals
		var perr error
		if regime == RegimeCompetencia {
			_, pTotals, perr = s.pending.AggregatePendingMonth(ctx, reference)
		} else {
			_, pTotals, perr = s.cash.AggregateCashPending(ctx, first, last)
		}
		if perr == nil {
			rep.Mode = "projetivo"
			rep.Projetado = &Projetado{
				TotalDespesas: pTotals.Despesas,
				TotalReceitas: pTotals.Receitas,
				SaldoPeriodo:  pTotals.Receitas - pTotals.Despesas,
			}
		}
	}
	return rep, nil
}

// monthDayBounds devolve o 1º e o último dia (YYYY-MM-DD) do mês de uma referência.
func monthDayBounds(reference string) (string, string, error) {
	last, err := MonthLastDay(reference)
	if err != nil {
		return "", "", err
	}
	return reference + "-01", last, nil
}

// monthLockState devolve o estado de bloqueio do mês (independe do regime).
func (s *Service) monthLockState(ctx context.Context, reference string) LockState {
	c, exists, err := s.repo.GetClosing(ctx, reference)
	if err != nil || !exists {
		return StateOpen
	}
	return DeriveLockState(true, c.HardLockAt, s.today())
}

// cashMonthTotals agrega o caixa (por data de pagamento) de um mês (comparativo caixa).
func (s *Service) cashMonthTotals(ctx context.Context, reference string) (shared.MonthlyTotals, []shared.SubcategoryAggregate) {
	first, last, err := monthDayBounds(reference)
	if err != nil {
		return shared.MonthlyTotals{}, nil
	}
	aggs, totals, aerr := s.cash.AggregateCashPeriod(ctx, first, last)
	if aerr != nil {
		return shared.MonthlyTotals{}, nil
	}
	return totals, aggs
}

// monthData retorna os agregados/totais de um mês: do snapshot se fechado, ao vivo
// (realizado) se aberto. Também devolve o status do mês.
func (s *Service) monthData(ctx context.Context, reference string) ([]shared.SubcategoryAggregate, shared.MonthlyTotals, LockState, error) {
	c, exists, err := s.repo.GetClosing(ctx, reference)
	if err != nil {
		return nil, shared.MonthlyTotals{}, "", err
	}
	if exists {
		aggs, serr := s.repo.Snapshot(ctx, reference)
		if serr != nil {
			return nil, shared.MonthlyTotals{}, "", serr
		}
		return aggs, c.Totals, DeriveLockState(true, c.HardLockAt, s.today()), nil
	}
	aggs, totals, aerr := s.realized.AggregateMonth(ctx, reference)
	if aerr != nil {
		return nil, shared.MonthlyTotals{}, "", domainerr.NewInternal("erro ao agregar lançamentos do mês")
	}
	return aggs, totals, StateOpen, nil
}

// refTotals devolve os totais de um mês de referência (para comparativo): do
// snapshot se fechado, ao vivo se aberto (marcado como parcial). Também os aggs.
func (s *Service) refTotals(ctx context.Context, reference string) (shared.MonthlyTotals, bool, []shared.SubcategoryAggregate) {
	c, exists, err := s.repo.GetClosing(ctx, reference)
	if err == nil && exists {
		aggs, _ := s.repo.Snapshot(ctx, reference)
		return c.Totals, true, aggs
	}
	aggs, totals, aerr := s.realized.AggregateMonth(ctx, reference)
	if aerr != nil {
		return shared.MonthlyTotals{}, false, nil
	}
	return totals, false, aggs
}

// ─── Leitura: períodos longos ────────────────────────────────────────────────

// Quarterly monta o relatório trimestral no regime escolhido (padrão caixa).
func (s *Service) Quarterly(ctx context.Context, year, quarter int, regime string) (Report, error) {
	if quarter < 1 || quarter > 4 {
		return Report{}, domainerr.NewBadRequest("trimestre inválido (1..4)", domainerr.WithDisplayable())
	}
	rep, err := s.longPeriod(ctx, MonthsInQuarter(year, quarter), MonthsInQuarter(year-1, quarter), prevQuarterMonths(year, quarter), regime)
	if err != nil {
		return Report{}, err
	}
	rep.Scope, rep.Year, rep.Quarter = "quarterly", year, quarter
	return rep, nil
}

// Semiannual monta o relatório semestral no regime escolhido (padrão caixa).
func (s *Service) Semiannual(ctx context.Context, year, half int, regime string) (Report, error) {
	if half < 1 || half > 2 {
		return Report{}, domainerr.NewBadRequest("semestre inválido (1..2)", domainerr.WithDisplayable())
	}
	prevMonths := MonthsInSemester(year, 1)
	if half == 1 {
		prevMonths = MonthsInSemester(year-1, 2)
	}
	rep, err := s.longPeriod(ctx, MonthsInSemester(year, half), MonthsInSemester(year-1, half), prevMonths, regime)
	if err != nil {
		return Report{}, err
	}
	rep.Scope, rep.Year, rep.Half = "semiannual", year, half
	return rep, nil
}

// Annual monta o relatório anual no regime escolhido (padrão caixa).
func (s *Service) Annual(ctx context.Context, year int, regime string) (Report, error) {
	rep, err := s.longPeriod(ctx, MonthsInYear(year), MonthsInYear(year-1), MonthsInYear(year-1), regime)
	if err != nil {
		return Report{}, err
	}
	rep.Scope, rep.Year = "annual", year
	return rep, nil
}

func prevQuarterMonths(year, quarter int) []string {
	if quarter == 1 {
		return MonthsInQuarter(year-1, 4)
	}
	return MonthsInQuarter(year, quarter-1)
}

// longPeriod soma snapshots dos meses fechados de `months`, lista os não incluídos,
// monta analítico/KPIs/comparativos (vs. período anterior e vs. mesmo período ano
// anterior) e o gráfico mês a mês.
func (s *Service) longPeriod(ctx context.Context, months, yoyMonths, prevMonths []string, regime string) (Report, error) {
	regime = normalizeRegime(regime)
	lk, err := s.lookup(ctx)
	if err != nil {
		return Report{}, err
	}
	if regime == RegimeCaixa {
		return s.longPeriodCash(ctx, lk, months, yoyMonths, prevMonths)
	}

	closings, err := s.repo.ClosingsForRefs(ctx, months)
	if err != nil {
		return Report{}, domainerr.NewInternal("erro ao ler fechamentos do período")
	}

	included, missing := []string{}, []string{}
	for _, m := range months {
		if _, ok := closings[m]; ok {
			included = append(included, m)
		} else {
			missing = append(missing, m)
		}
	}

	aggs, err := s.repo.SnapshotForRefs(ctx, included)
	if err != nil {
		return Report{}, domainerr.NewInternal("erro ao somar snapshots do período")
	}

	totals := sumClosings(closings, included)

	// pontos mês a mês (saldo acumulado = saldoFinal de cada mês incluído)
	monthly := make([]MonthlyPoint, 0, len(included))
	for _, m := range included {
		c := closings[m]
		monthly = append(monthly, MonthlyPoint{
			Reference:           m,
			TotalDespesas:       c.Totals.Despesas,
			TotalReceitas:       c.Totals.Receitas,
			TotalTransferencias: c.Totals.Transferencias,
			SaldoAcumulado:      c.Totals.SaldoFinal,
		})
	}

	prevTotals, prevClosedAll, prevAggs := s.periodTotals(ctx, prevMonths)
	yoyTotals, yoyClosedAll, _ := s.periodTotals(ctx, yoyMonths)

	rep := Report{
		Regime:         RegimeCompetencia,
		KPIs:           BuildKPIs(totals, aggs, 0),
		Analitico:      BuildAnalitico(aggs, lk),
		IncludedMonths: included,
		MissingMonths:  missing,
		Monthly:        monthly,
		Comparativos: Comparativos{
			PeriodoAnterior:         BuildComparison(prevMonths[0]+".."+prevMonths[len(prevMonths)-1], !prevClosedAll, totals, prevTotals),
			MesmoPeriodoAnoAnterior: BuildComparison(yoyMonths[0]+".."+yoyMonths[len(yoyMonths)-1], !yoyClosedAll, totals, yoyTotals),
		},
	}
	rep.Insights = BuildInsights(rep.Analitico, rep.KPIs, rep.Comparativos.PeriodoAnterior, despesaByCat(prevAggs))
	return rep, nil
}

// longPeriodCash monta o relatório de período longo no regime de CAIXA: apura ao vivo por
// data de pagamento sobre todo o intervalo (não depende de meses fechados). IncludedMonths
// = todos; MissingMonths = vazio. Mês a mês e comparativos também por caixa.
func (s *Service) longPeriodCash(ctx context.Context, lk *CategoryLookup, months, yoyMonths, prevMonths []string) (Report, error) {
	from, _, _ := monthDayBounds(months[0])
	_, to, _ := monthDayBounds(months[len(months)-1])
	aggs, totals, err := s.cash.AggregateCashPeriod(ctx, from, to)
	if err != nil {
		return Report{}, domainerr.NewInternal("erro ao agregar o caixa do período")
	}

	monthly := make([]MonthlyPoint, 0, len(months))
	for _, m := range months {
		mt, _ := s.cashMonthTotals(ctx, m)
		monthly = append(monthly, MonthlyPoint{
			Reference:           m,
			TotalDespesas:       mt.Despesas,
			TotalReceitas:       mt.Receitas,
			TotalTransferencias: mt.Transferencias,
			SaldoAcumulado:      mt.SaldoFinal,
		})
	}

	prevTotals, prevAggs := s.cashPeriodTotals(ctx, prevMonths)
	yoyTotals, _ := s.cashPeriodTotals(ctx, yoyMonths)

	rep := Report{
		Regime:         RegimeCaixa,
		KPIs:           BuildKPIs(totals, aggs, 0),
		Analitico:      BuildAnalitico(aggs, lk),
		IncludedMonths: months,
		MissingMonths:  []string{},
		Monthly:        monthly,
		Comparativos: Comparativos{
			PeriodoAnterior:         BuildComparison(prevMonths[0]+".."+prevMonths[len(prevMonths)-1], false, totals, prevTotals),
			MesmoPeriodoAnoAnterior: BuildComparison(yoyMonths[0]+".."+yoyMonths[len(yoyMonths)-1], false, totals, yoyTotals),
		},
	}
	rep.Insights = BuildInsights(rep.Analitico, rep.KPIs, rep.Comparativos.PeriodoAnterior, despesaByCat(prevAggs))
	return rep, nil
}

// cashPeriodTotals agrega o caixa sobre o intervalo coberto por `months`.
func (s *Service) cashPeriodTotals(ctx context.Context, months []string) (shared.MonthlyTotals, []shared.SubcategoryAggregate) {
	if len(months) == 0 {
		return shared.MonthlyTotals{}, nil
	}
	from, _, _ := monthDayBounds(months[0])
	_, to, _ := monthDayBounds(months[len(months)-1])
	aggs, totals, err := s.cash.AggregateCashPeriod(ctx, from, to)
	if err != nil {
		return shared.MonthlyTotals{}, nil
	}
	return totals, aggs
}

// periodTotals soma os totais dos meses FECHADOS de um conjunto de referências;
// allClosed=false se algum mês do conjunto não estiver fechado (comparativo parcial).
func (s *Service) periodTotals(ctx context.Context, months []string) (shared.MonthlyTotals, bool, []shared.SubcategoryAggregate) {
	closings, err := s.repo.ClosingsForRefs(ctx, months)
	if err != nil {
		return shared.MonthlyTotals{}, false, nil
	}
	included := []string{}
	for _, m := range months {
		if _, ok := closings[m]; ok {
			included = append(included, m)
		}
	}
	aggs, _ := s.repo.SnapshotForRefs(ctx, included)
	return sumClosings(closings, included), len(included) == len(months), aggs
}

// sumClosings soma os totais dos meses incluídos; saldoInicial = do 1º mês,
// saldoFinal = do último (saldo acumulado abrange o período).
func sumClosings(closings map[string]Closing, included []string) shared.MonthlyTotals {
	var t shared.MonthlyTotals
	for i, m := range included {
		c := closings[m]
		t.Receitas += c.Totals.Receitas
		t.Despesas += c.Totals.Despesas
		t.Transferencias += c.Totals.Transferencias
		t.TxCount += c.Totals.TxCount
		if i == 0 {
			t.SaldoInicial = c.Totals.SaldoInicial
		}
		t.SaldoFinal = c.Totals.SaldoFinal
	}
	t.SaldoPeriodo = t.Receitas - t.Despesas
	return t
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (s *Service) lookup(ctx context.Context) (*CategoryLookup, error) {
	tree, err := s.tree.Tree(ctx)
	if err != nil {
		return nil, domainerr.NewInternal("erro ao ler categorias")
	}
	return NewCategoryLookup(tree), nil
}

func despesaByCat(aggs []shared.SubcategoryAggregate) map[string]int64 {
	if aggs == nil {
		return nil
	}
	m := map[string]int64{}
	for _, a := range aggs {
		if a.Type == "despesa" {
			m[a.CategoryID] += a.Total
		}
	}
	return m
}

func paymentSlices(m map[string]int64) []PaymentSlice {
	out := make([]PaymentSlice, 0, len(m))
	for method, total := range m {
		if total != 0 {
			out = append(out, PaymentSlice{Method: method, Total: total})
		}
	}
	return out
}
