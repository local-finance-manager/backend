package creditcard

import (
	"sort"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// statusCancelado espelha transaction.StatusCancelado (string neutra no DTO).
// Compras canceladas não compõem fatura, total nem usedLimit.
const statusCancelado = "cancelado"

// ─── Tipos de fatura ────────────────────────────────────────────────────────

// Invoice é a projeção calculada de uma fatura (não armazenada — D4).
type Invoice struct {
	Reference         string
	CycleStart        string
	ClosingDate       string
	DueDate           string
	Status            InvoiceStatus
	Total             int64
	Count             int
	Payment           *InvoicePayment
	CategoryBreakdown []CategoryBreakdown
}

// CategoryBreakdown é a distribuição de um recorte por categoria.
type CategoryBreakdown struct {
	CategoryID   string
	CategoryName string
	Color        string
	Total        int64
	Percent      int
}

// InvoicePayment é o registro persistido do pagamento de uma fatura.
type InvoicePayment struct {
	Reference     string
	PaymentDate   string
	TransactionID *string
	CreatedAt     time.Time
}

// MonthlyCardSummary é o resumo mensal por cartão (por competência).
type MonthlyCardSummary struct {
	CreditCardID      string
	Year, Month       int
	Total             int64
	Count             int
	AverageTicket     int64
	CategoryBreakdown []CategoryBreakdown
}

// ─── Agregações puras ───────────────────────────────────────────────────────

// counts reporta se um lançamento compõe a fatura (exclui cancelados).
func counts(t shared.CardTransaction) bool {
	return t.Status != statusCancelado
}

// sumAmount soma os valores das compras que contam (exclui cancelados).
func sumAmount(txns []shared.CardTransaction) int64 {
	var total int64
	for _, t := range txns {
		if counts(t) {
			total += t.Amount
		}
	}
	return total
}

// countCounted conta os lançamentos que compõem a fatura.
func countCounted(txns []shared.CardTransaction) int {
	n := 0
	for _, t := range txns {
		if counts(t) {
			n++
		}
	}
	return n
}

// breakdownByCategory agrega por categoria e calcula percentuais sobre total.
// Ordena por total desc (tie-break por categoryID) para saída determinística.
func breakdownByCategory(txns []shared.CardTransaction, total int64) []CategoryBreakdown {
	type acc struct {
		name  string
		color string
		total int64
	}
	byCat := map[string]*acc{}
	order := []string{}
	for _, t := range txns {
		if !counts(t) {
			continue
		}
		a, ok := byCat[t.CategoryID]
		if !ok {
			a = &acc{name: t.CategoryName, color: t.CategoryColor}
			byCat[t.CategoryID] = a
			order = append(order, t.CategoryID)
		}
		a.total += t.Amount
	}

	out := make([]CategoryBreakdown, 0, len(order))
	for _, id := range order {
		a := byCat[id]
		percent := 0
		if total > 0 {
			percent = int(a.total * 100 / total)
		}
		out = append(out, CategoryBreakdown{
			CategoryID:   id,
			CategoryName: a.name,
			Color:        a.color,
			Total:        a.total,
			Percent:      percent,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].CategoryID < out[j].CategoryID
	})
	return out
}

// bucketByReference agrupa lançamentos pela reference da fatura (usa InvoiceReferenceFor),
// ignorando cancelados.
func bucketByReference(txns []shared.CardTransaction, closingDay int) (map[string][]shared.CardTransaction, error) {
	buckets := map[string][]shared.CardTransaction{}
	for _, t := range txns {
		if !counts(t) {
			continue
		}
		ref, err := InvoiceReferenceFor(t.CompetenceDate, closingDay)
		if err != nil {
			return nil, err
		}
		buckets[ref] = append(buckets[ref], t)
	}
	return buckets, nil
}

// BuildInvoice monta a Invoice de uma reference, dado seu bucket, a config do cartão,
// "hoje" e o pagamento (ou nil). txns pode ser vazio (fatura zerada).
func BuildInvoice(reference string, txns []shared.CardTransaction, card CreditCard, today string, payment *InvoicePayment) (Invoice, error) {
	cycleStart, err := CycleStart(reference, card.ClosingDay)
	if err != nil {
		return Invoice{}, err
	}
	closingDate, err := ClosingDate(reference, card.ClosingDay)
	if err != nil {
		return Invoice{}, err
	}
	dueDate, err := DueDate(reference, card.ClosingDay, card.DueDay)
	if err != nil {
		return Invoice{}, err
	}

	total := sumAmount(txns)
	return Invoice{
		Reference:         reference,
		CycleStart:        cycleStart,
		ClosingDate:       closingDate,
		DueDate:           dueDate,
		Status:            DeriveInvoiceStatus(today, cycleStart, closingDate, dueDate, payment != nil),
		Total:             total,
		Count:             countCounted(txns),
		Payment:           payment,
		CategoryBreakdown: breakdownByCategory(txns, total),
	}, nil
}

// UsedLimit soma os totais das faturas em {aberta, fechada, vencida} (D11/RF-CC-07).
// Faturas pagas e futuras não comprometem limite.
func UsedLimit(buckets map[string][]shared.CardTransaction, card CreditCard, today string, payments map[string]*InvoicePayment) (int64, error) {
	var used int64
	for ref, txns := range buckets {
		cycleStart, err := CycleStart(ref, card.ClosingDay)
		if err != nil {
			return 0, err
		}
		closingDate, err := ClosingDate(ref, card.ClosingDay)
		if err != nil {
			return 0, err
		}
		dueDate, err := DueDate(ref, card.ClosingDay, card.DueDay)
		if err != nil {
			return 0, err
		}
		_, hasPayment := payments[ref]
		// StatusFutura entra no usedLimit (RF-PARC-10): parcelas futuras de compras
		// parceladas comprometem o limite na hora da compra. usedLimit = tudo não-pago.
		switch DeriveInvoiceStatus(today, cycleStart, closingDate, dueDate, hasPayment) {
		case StatusAberta, StatusFechada, StatusVencida, StatusFutura:
			used += sumAmount(txns)
		}
	}
	return used, nil
}

// BuildMonthlySummary monta o resumo mensal por competência a partir dos lançamentos
// do mês (já filtrados pelo use case via reader).
func BuildMonthlySummary(cardID string, year, month int, txns []shared.CardTransaction) MonthlyCardSummary {
	total := sumAmount(txns)
	count := countCounted(txns)
	var avg int64
	if count > 0 {
		avg = total / int64(count)
	}
	return MonthlyCardSummary{
		CreditCardID:      cardID,
		Year:              year,
		Month:             month,
		Total:             total,
		Count:             count,
		AverageTicket:     avg,
		CategoryBreakdown: breakdownByCategory(txns, total),
	}
}
