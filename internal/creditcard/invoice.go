package creditcard

import (
	"sort"
	"time"

	"github.com/local-finance-manager/backend/internal/shared"
)

// Espelham os status de transaction (strings neutras no DTO shared.CardTransaction).
// Compras canceladas não compõem fatura, total nem usedLimit.
const (
	statusPendente  = "pendente"
	statusRealizado = "realizado"
	statusCancelado = "cancelado"
)

// ─── Tipos de fatura ────────────────────────────────────────────────────────

// PaymentStatus resume o quanto da fatura já foi pago (independe do ciclo).
type PaymentStatus string

const (
	PaymentNenhum  PaymentStatus = "nenhum"  // nada pago
	PaymentParcial PaymentStatus = "parcial" // 0 < pago < total
	PaymentPaga    PaymentStatus = "paga"    // pago >= total
)

// Invoice é a projeção calculada de uma fatura (não armazenada — D4).
type Invoice struct {
	Reference         string
	CycleStart        string
	ClosingDate       string
	DueDate           string
	Status            InvoiceStatus
	Total             int64
	PaidAmount        int64 // soma dos pagamentos (ledger)
	OutstandingAmount int64 // saldo devedor = max(0, Total − PaidAmount)
	PaymentStatus     PaymentStatus
	Count             int
	Payments          []InvoicePayment // ledger de pagamentos (0..N)
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

// InvoicePayment é uma entrada do ledger de pagamentos de uma fatura (1 por pagamento).
type InvoicePayment struct {
	ID            string
	Reference     string
	Amount        int64 // centavos
	PaymentDate   string
	TransactionID *string
	CreatedAt     time.Time
}

// sumPayments soma os valores dos pagamentos do ledger.
func sumPayments(payments []InvoicePayment) int64 {
	var paid int64
	for _, p := range payments {
		paid += p.Amount
	}
	return paid
}

// derivePaymentStatus classifica o quanto da fatura foi pago.
func derivePaymentStatus(total, paid int64) PaymentStatus {
	switch {
	case total > 0 && paid >= total:
		return PaymentPaga
	case paid > 0:
		return PaymentParcial
	default:
		return PaymentNenhum
	}
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
// "hoje" e os pagamentos do ledger (0..N). txns pode ser vazio (fatura zerada).
func BuildInvoice(reference string, txns []shared.CardTransaction, card CreditCard, today string, payments []InvoicePayment) (Invoice, error) {
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
	paid := sumPayments(payments)
	outstanding := total - paid
	if outstanding < 0 {
		outstanding = 0
	}
	payStatus := derivePaymentStatus(total, paid)
	if payments == nil {
		payments = []InvoicePayment{}
	}
	return Invoice{
		Reference:         reference,
		CycleStart:        cycleStart,
		ClosingDate:       closingDate,
		DueDate:           dueDate,
		Status:            DeriveInvoiceStatus(today, cycleStart, closingDate, dueDate, payStatus == PaymentPaga),
		Total:             total,
		PaidAmount:        paid,
		OutstandingAmount: outstanding,
		PaymentStatus:     payStatus,
		Count:             countCounted(txns),
		Payments:          payments,
		CategoryBreakdown: breakdownByCategory(txns, total),
	}, nil
}

// UsedLimit soma o SALDO DEVEDOR (total − pago) das faturas não quitadas (D11/RF-CC-07).
// Pagamentos parciais liberam limite pelo valor pago; faturas pagas não comprometem nada.
// Futura/aberta/fechada/vencida comprometem o saldo ainda devido (RF-PARC-10).
func UsedLimit(buckets map[string][]shared.CardTransaction, card CreditCard, today string, paymentsByRef map[string][]InvoicePayment) (int64, error) {
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
		total := sumAmount(txns)
		paid := sumPayments(paymentsByRef[ref])
		fullyPaid := total > 0 && paid >= total
		switch DeriveInvoiceStatus(today, cycleStart, closingDate, dueDate, fullyPaid) {
		case StatusAberta, StatusFechada, StatusVencida, StatusFutura:
			outstanding := total - paid
			if outstanding > 0 {
				used += outstanding
			}
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
