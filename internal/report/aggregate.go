package report

import (
	"sort"

	"github.com/local-finance-manager/backend/internal/shared"
)

// ─── Tipos de resposta (camelCase — contrato Apêndice B) ─────────────────────

// KPIs do período (centavos; percentuais inteiros derivados para exibição).
type KPIs struct {
	TotalReceitas       int64 `json:"totalReceitas"`
	TotalDespesas       int64 `json:"totalDespesas"`
	TotalTransferencias int64 `json:"totalTransferencias"`
	SaldoPeriodo        int64 `json:"saldoPeriodo"`
	SaldoInicial        int64 `json:"saldoInicial"`
	SaldoFinal          int64 `json:"saldoFinal"`
	TaxaPoupanca        int   `json:"taxaPoupanca"`
	TicketMedio         int64 `json:"ticketMedio"`
	TxCount             int   `json:"txCount"`
	PercentNoCredito    int   `json:"percentNoCredito"`
}

// SubAnalitico é uma subcategoria no analítico.
type SubAnalitico struct {
	SubcategoryID string `json:"subcategoryId"`
	Name          string `json:"name"`
	Total         int64  `json:"total"`
	Percent       int    `json:"percent"`
}

// CatAnalitico é uma categoria no analítico (com drill-down em subcategorias).
type CatAnalitico struct {
	CategoryID    string         `json:"categoryId"`
	CategoryName  string         `json:"categoryName"`
	Color         string         `json:"color"`
	Total         int64          `json:"total"`
	Percent       int            `json:"percent"`
	Subcategorias []SubAnalitico `json:"subcategorias"`
}

// Analitico separa o detalhamento por tipo.
type Analitico struct {
	Despesas       []CatAnalitico `json:"despesas"`
	Receitas       []CatAnalitico `json:"receitas"`
	Transferencias []CatAnalitico `json:"transferencias"`
}

// Comparison é um lado de comparativo (vs. período anterior ou vs. ano anterior).
type Comparison struct {
	Reference            string `json:"reference"`
	Partial              bool   `json:"partial"`
	TotalDespesas        int64  `json:"totalDespesas"`
	TotalReceitas        int64  `json:"totalReceitas"`
	DeltaAbsDespesas     int64  `json:"deltaAbsDespesas"`
	DeltaPercentDespesas int    `json:"deltaPercentDespesas"`
	DeltaAbsReceitas     int64  `json:"deltaAbsReceitas"`
	DeltaPercentReceitas int    `json:"deltaPercentReceitas"`
}

// Comparativos reúne as duas comparações (RF-REL-12).
type Comparativos struct {
	PeriodoAnterior         *Comparison `json:"periodoAnterior"`
	MesmoPeriodoAnoAnterior *Comparison `json:"mesmoPeriodoAnoAnterior"`
}

// PaymentSlice é uma fatia da distribuição por forma de pagamento (mensal).
type PaymentSlice struct {
	Method string `json:"method"`
	Total  int64  `json:"total"`
}

// MonthlyPoint é um ponto do gráfico mês a mês (períodos longos — RF-REL-14).
type MonthlyPoint struct {
	Reference           string `json:"reference"`
	TotalDespesas       int64  `json:"totalDespesas"`
	TotalReceitas       int64  `json:"totalReceitas"`
	TotalTransferencias int64  `json:"totalTransferencias"`
	SaldoAcumulado      int64  `json:"saldoAcumulado"`
}

// Projetado é a parte pendente do modo projetivo (mensal).
type Projetado struct {
	TotalDespesas int64 `json:"totalDespesas"`
	TotalReceitas int64 `json:"totalReceitas"`
	SaldoPeriodo  int64 `json:"saldoPeriodo"`
}

// Report é a resposta unificada de relatório (campos opcionais por escopo/modo).
type Report struct {
	Scope        string       `json:"scope"`
	Reference    string       `json:"reference,omitempty"`
	Year         int          `json:"year,omitempty"`
	Quarter      int          `json:"quarter,omitempty"`
	Half         int          `json:"half,omitempty"`
	Regime       string       `json:"regime,omitempty"` // "caixa" (padrão) | "competencia"
	Mode         string       `json:"mode,omitempty"`
	Status       LockState    `json:"status,omitempty"`
	KPIs         KPIs         `json:"kpis"`
	Analitico    Analitico    `json:"analitico"`
	Comparativos Comparativos `json:"comparativos"`
	Insights     []string     `json:"insights"`

	PaymentMethods []PaymentSlice `json:"paymentMethods,omitempty"` // mensal

	IncludedMonths []string       `json:"includedMonths,omitempty"` // longos
	MissingMonths  []string       `json:"missingMonths,omitempty"`
	Monthly        []MonthlyPoint `json:"monthly,omitempty"`

	Projetado *Projetado `json:"projetado,omitempty"` // mensal projetivo
}

// ─── Helpers numéricos ───────────────────────────────────────────────────────

// pct retorna part/whole em percentual inteiro (0 se whole == 0).
func pct(part, whole int64) int {
	if whole == 0 {
		return 0
	}
	return int(part * 100 / whole)
}

// ─── Rollup: agregados → analítico ───────────────────────────────────────────

// CategoryLookup resolve nomes/cores de categorias e nomes de subcategorias.
type CategoryLookup struct {
	catName  map[string]string
	catColor map[string]string
	subName  map[string]string
}

// NewCategoryLookup constrói o lookup a partir da árvore de categorias.
func NewCategoryLookup(tree []shared.CategoryNode) *CategoryLookup {
	l := &CategoryLookup{
		catName:  map[string]string{},
		catColor: map[string]string{},
		subName:  map[string]string{},
	}
	for _, c := range tree {
		l.catName[c.CategoryID] = c.CategoryName
		l.catColor[c.CategoryID] = c.CategoryColor
		for _, s := range c.Subcategories {
			l.subName[s.ID] = s.Name
		}
	}
	return l
}

// BuildAnalitico agrupa os agregados por tipo → categoria → subcategoria,
// calculando o % de cada item sobre o total do seu tipo. Ordena por total desc.
func BuildAnalitico(aggs []shared.SubcategoryAggregate, lk *CategoryLookup) Analitico {
	return Analitico{
		Despesas:       buildByType(aggs, "despesa", lk),
		Receitas:       buildByType(aggs, "receita", lk),
		Transferencias: buildByType(aggs, "transferencia", lk),
	}
}

func buildByType(aggs []shared.SubcategoryAggregate, typ string, lk *CategoryLookup) []CatAnalitico {
	type catAcc struct {
		total int64
		subs  []SubAnalitico
	}
	byCat := map[string]*catAcc{}
	order := []string{}
	var typeTotal int64

	for _, a := range aggs {
		if a.Type != typ || a.Total == 0 {
			continue
		}
		typeTotal += a.Total
		c, ok := byCat[a.CategoryID]
		if !ok {
			c = &catAcc{}
			byCat[a.CategoryID] = c
			order = append(order, a.CategoryID)
		}
		c.total += a.Total
		c.subs = append(c.subs, SubAnalitico{
			SubcategoryID: a.SubcategoryID,
			Name:          lk.subName[a.SubcategoryID],
			Total:         a.Total,
		})
	}

	out := make([]CatAnalitico, 0, len(order))
	for _, cid := range order {
		c := byCat[cid]
		for i := range c.subs {
			c.subs[i].Percent = pct(c.subs[i].Total, c.total)
		}
		sort.Slice(c.subs, func(i, j int) bool { return c.subs[i].Total > c.subs[j].Total })
		out = append(out, CatAnalitico{
			CategoryID:    cid,
			CategoryName:  lk.catName[cid],
			Color:         lk.catColor[cid],
			Total:         c.total,
			Percent:       pct(c.total, typeTotal),
			Subcategorias: c.subs,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

// ─── KPIs ────────────────────────────────────────────────────────────────────

// BuildKPIs deriva os KPIs a partir dos totais do período e dos agregados (para
// contagem por tipo / ticket médio). percentNoCredito vem separado (mensal).
func BuildKPIs(t shared.MonthlyTotals, aggs []shared.SubcategoryAggregate, percentNoCredito int) KPIs {
	var despesaCount int
	for _, a := range aggs {
		if a.Type == "despesa" {
			despesaCount += a.TxCount
		}
	}
	ticket := int64(0)
	if despesaCount > 0 {
		ticket = t.Despesas / int64(despesaCount)
	}
	return KPIs{
		TotalReceitas:       t.Receitas,
		TotalDespesas:       t.Despesas,
		TotalTransferencias: t.Transferencias,
		SaldoPeriodo:        t.SaldoPeriodo,
		SaldoInicial:        t.SaldoInicial,
		SaldoFinal:          t.SaldoFinal,
		TaxaPoupanca:        pct(t.Receitas-t.Despesas, t.Receitas),
		TicketMedio:         ticket,
		TxCount:             t.TxCount,
		PercentNoCredito:    percentNoCredito,
	}
}

// ─── Comparativos ────────────────────────────────────────────────────────────

// BuildComparison monta um lado do comparativo: valores de referência + deltas.
func BuildComparison(reference string, partial bool, cur, ref shared.MonthlyTotals) *Comparison {
	return &Comparison{
		Reference:            reference,
		Partial:              partial,
		TotalDespesas:        ref.Despesas,
		TotalReceitas:        ref.Receitas,
		DeltaAbsDespesas:     cur.Despesas - ref.Despesas,
		DeltaPercentDespesas: deltaPct(cur.Despesas, ref.Despesas),
		DeltaAbsReceitas:     cur.Receitas - ref.Receitas,
		DeltaPercentReceitas: deltaPct(cur.Receitas, ref.Receitas),
	}
}

// deltaPct retorna a variação percentual de ref→cur (0 se ref == 0).
func deltaPct(cur, ref int64) int {
	if ref == 0 {
		return 0
	}
	return int((cur - ref) * 100 / ref)
}

// ─── Insights (regras fixas por limiar — RF-REL-19) ──────────────────────────

const insightThreshold = 30 // % de variação para gerar insight de categoria

// BuildInsights gera alertas textuais simples a partir do analítico e comparativos.
func BuildInsights(analitico Analitico, kpis KPIs, prev *Comparison, prevByCat map[string]int64) []string {
	out := []string{}

	// maior gasto do período
	if len(analitico.Despesas) > 0 {
		top := analitico.Despesas[0]
		out = append(out, "Maior gasto: "+top.CategoryName+" ("+pctStr(top.Percent)+" das despesas)")
	}

	// variações relevantes por categoria de despesa vs. período anterior
	if prevByCat != nil {
		for _, c := range analitico.Despesas {
			ref := prevByCat[c.CategoryID]
			d := deltaPct(c.Total, ref)
			if ref > 0 && (d >= insightThreshold || d <= -insightThreshold) {
				dir := "subiu"
				if d < 0 {
					dir = "caiu"
				}
				out = append(out, c.CategoryName+" "+dir+" "+pctStr(abs(d))+" vs. período anterior")
			}
		}
	}

	// taxa de poupança
	if kpis.TotalReceitas > 0 {
		if kpis.TaxaPoupanca >= 20 {
			out = append(out, "Boa taxa de poupança no período: "+pctStr(kpis.TaxaPoupanca))
		} else if kpis.TaxaPoupanca < 0 {
			out = append(out, "Atenção: você gastou mais do que recebeu no período")
		}
	}

	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func pctStr(p int) string {
	return itoa(p) + "%"
}

// itoa evita importar strconv só para isto no domínio.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
