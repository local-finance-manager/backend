package creditcard_test

import (
	"testing"

	"github.com/local-finance-manager/backend/internal/creditcard"
	"github.com/local-finance-manager/backend/internal/shared"
)

func mkTxn(id string, amount int64, competence, catID, catName, color string) shared.CardTransaction {
	return shared.CardTransaction{
		ID:             id,
		Amount:         amount,
		CompetenceDate: competence,
		Status:         "realizado",
		CategoryID:     catID,
		CategoryName:   catName,
		CategoryColor:  color,
		CreditCardID:   "card-1",
	}
}

func card(closingDay, dueDay int) creditcard.CreditCard {
	return creditcard.CreditCard{ID: "card-1", ClosingDay: closingDay, DueDay: dueDay, CreditLimit: 500000}
}

// ─── BuildInvoice ───────────────────────────────────────────────────────────

func TestBuildInvoice_TotalsAndBreakdown(t *testing.T) {
	txns := []shared.CardTransaction{
		mkTxn("a", 120000, "2026-06-10", "cat-food", "Alimentação", "#27AE60"),
		mkTxn("b", 90000, "2026-06-15", "cat-transp", "Transporte", "#2980B9"),
		mkTxn("c", 10000, "2026-06-20", "cat-food", "Alimentação", "#27AE60"),
	}
	inv, err := creditcard.BuildInvoice("2026-07", txns, card(3, 10), "2026-06-25", nil)
	if err != nil {
		t.Fatalf("BuildInvoice: %v", err)
	}
	if inv.Total != 220000 {
		t.Errorf("total: got %d, want 220000", inv.Total)
	}
	if inv.Count != 3 {
		t.Errorf("count: got %d, want 3", inv.Count)
	}
	if inv.Status != creditcard.StatusAberta {
		t.Errorf("status: got %q, want aberta", inv.Status)
	}
	if inv.CycleStart != "2026-06-04" || inv.ClosingDate != "2026-07-03" || inv.DueDate != "2026-07-10" {
		t.Errorf("datas do ciclo erradas: %+v", inv)
	}
	if len(inv.CategoryBreakdown) != 2 {
		t.Fatalf("breakdown len: got %d, want 2", len(inv.CategoryBreakdown))
	}
	// ordenado por total desc → Alimentação (130000) primeiro
	if inv.CategoryBreakdown[0].CategoryID != "cat-food" || inv.CategoryBreakdown[0].Total != 130000 {
		t.Errorf("breakdown[0] inesperado: %+v", inv.CategoryBreakdown[0])
	}
	if inv.CategoryBreakdown[0].Percent != 59 { // 130000/220000 = 59%
		t.Errorf("percent: got %d, want 59", inv.CategoryBreakdown[0].Percent)
	}
}

func TestBuildInvoice_IgnoresCancelled(t *testing.T) {
	txns := []shared.CardTransaction{
		mkTxn("a", 100000, "2026-06-10", "cat-1", "Cat 1", "#fff"),
		func() shared.CardTransaction {
			c := mkTxn("b", 999999, "2026-06-11", "cat-1", "Cat 1", "#fff")
			c.Status = "cancelado"
			return c
		}(),
	}
	inv, err := creditcard.BuildInvoice("2026-07", txns, card(3, 10), "2026-06-25", nil)
	if err != nil {
		t.Fatalf("BuildInvoice: %v", err)
	}
	if inv.Total != 100000 {
		t.Errorf("total: got %d, want 100000 (cancelado deve ser ignorado)", inv.Total)
	}
	if inv.Count != 1 {
		t.Errorf("count: got %d, want 1", inv.Count)
	}
}

func TestBuildInvoice_Empty(t *testing.T) {
	inv, err := creditcard.BuildInvoice("2026-07", nil, card(3, 10), "2026-06-25", nil)
	if err != nil {
		t.Fatalf("BuildInvoice: %v", err)
	}
	if inv.Total != 0 || inv.Count != 0 {
		t.Errorf("fatura vazia deveria ter total/count 0: %+v", inv)
	}
	if inv.Status != creditcard.StatusAberta {
		t.Errorf("status: got %q, want aberta", inv.Status)
	}
}

func TestBuildInvoice_Paid(t *testing.T) {
	pay := &creditcard.InvoicePayment{Reference: "2026-06", PaymentDate: "2026-06-10"}
	inv, err := creditcard.BuildInvoice("2026-06", []shared.CardTransaction{
		mkTxn("a", 50000, "2026-05-10", "cat-1", "Cat 1", "#fff"),
	}, card(3, 10), "2026-08-01", pay)
	if err != nil {
		t.Fatalf("BuildInvoice: %v", err)
	}
	if inv.Status != creditcard.StatusPaga {
		t.Errorf("status: got %q, want paga", inv.Status)
	}
	if inv.Payment == nil {
		t.Error("payment deveria estar preenchido")
	}
}

// ─── bucket + UsedLimit ─────────────────────────────────────────────────────

func TestUsedLimit(t *testing.T) {
	c := card(3, 10)
	// today fixo
	const today = "2026-07-20"
	// Fatura 2026-06 (fechada em 03/06, vence 10/06) → vencida em 20/07, sem pagamento → conta
	// Fatura 2026-07 (fecha 03/07, vence 10/07) → vencida em 20/07 → conta
	// Fatura 2026-08 (aberta: ciclo 04/07..03/08, today 20/07 dentro) → conta
	// Fatura paga não conta.
	txns := []shared.CardTransaction{
		mkTxn("jun", 10000, "2026-05-20", "c", "C", "#f"), // ref 2026-06
		mkTxn("jul", 20000, "2026-06-20", "c", "C", "#f"), // ref 2026-07
		mkTxn("ago", 30000, "2026-07-20", "c", "C", "#f"), // ref 2026-08 (aberta)
	}
	buckets, err := creditcardBuckets(txns, c.ClosingDay)
	if err != nil {
		t.Fatal(err)
	}
	// sem pagamentos → tudo conta = 60000
	used, err := creditcard.UsedLimit(buckets, c, today, map[string]*creditcard.InvoicePayment{})
	if err != nil {
		t.Fatalf("UsedLimit: %v", err)
	}
	if used != 60000 {
		t.Errorf("used (sem pagamentos): got %d, want 60000", used)
	}
	// pagando a fatura de junho → 60000 - 10000 = 50000
	payments := map[string]*creditcard.InvoicePayment{"2026-06": {Reference: "2026-06", PaymentDate: "2026-06-10"}}
	used, err = creditcard.UsedLimit(buckets, c, today, payments)
	if err != nil {
		t.Fatalf("UsedLimit: %v", err)
	}
	if used != 50000 {
		t.Errorf("used (junho paga): got %d, want 50000", used)
	}
}

func TestUsedLimit_FutureCounted(t *testing.T) {
	c := card(3, 10)
	// today antes de qualquer ciclo → fatura futura. Desde RF-PARC-10, faturas futuras
	// (ex.: parcelas de uma compra parcelada) TAMBÉM comprometem o limite.
	txns := []shared.CardTransaction{
		mkTxn("a", 50000, "2026-12-20", "c", "C", "#f"),
	}
	buckets, err := creditcardBuckets(txns, c.ClosingDay)
	if err != nil {
		t.Fatal(err)
	}
	used, err := creditcard.UsedLimit(buckets, c, "2026-06-01", map[string]*creditcard.InvoicePayment{})
	if err != nil {
		t.Fatalf("UsedLimit: %v", err)
	}
	if used != 50000 {
		t.Errorf("fatura futura deve comprometer limite (RF-PARC-10): got %d, want 50000", used)
	}
}

func TestUsedLimit_PaidFutureNotCounted(t *testing.T) {
	c := card(3, 10)
	// fatura futura mas paga → não conta (paga nunca compromete)
	txns := []shared.CardTransaction{mkTxn("a", 50000, "2026-12-20", "c", "C", "#f")}
	buckets, _ := creditcardBuckets(txns, c.ClosingDay)
	ref, _ := creditcard.InvoiceReferenceFor("2026-12-20", c.ClosingDay)
	payments := map[string]*creditcard.InvoicePayment{ref: {Reference: ref, PaymentDate: "2026-12-10"}}
	used, err := creditcard.UsedLimit(buckets, c, "2026-06-01", payments)
	if err != nil {
		t.Fatalf("UsedLimit: %v", err)
	}
	if used != 0 {
		t.Errorf("futura paga não deveria comprometer limite: got %d, want 0", used)
	}
}

// ─── Resumo mensal ──────────────────────────────────────────────────────────

func TestBuildMonthlySummary(t *testing.T) {
	txns := []shared.CardTransaction{
		mkTxn("a", 100000, "2026-06-05", "cat-1", "Cat 1", "#a"),
		mkTxn("b", 50000, "2026-06-15", "cat-2", "Cat 2", "#b"),
		func() shared.CardTransaction {
			c := mkTxn("c", 999999, "2026-06-20", "cat-1", "Cat 1", "#a")
			c.Status = "cancelado"
			return c
		}(),
	}
	s := creditcard.BuildMonthlySummary("card-1", 2026, 6, txns)
	if s.Total != 150000 {
		t.Errorf("total: got %d, want 150000", s.Total)
	}
	if s.Count != 2 {
		t.Errorf("count: got %d, want 2", s.Count)
	}
	if s.AverageTicket != 75000 {
		t.Errorf("avg: got %d, want 75000", s.AverageTicket)
	}
	if len(s.CategoryBreakdown) != 2 {
		t.Errorf("breakdown len: got %d, want 2", len(s.CategoryBreakdown))
	}
}

func TestBuildMonthlySummary_Empty(t *testing.T) {
	s := creditcard.BuildMonthlySummary("card-1", 2026, 6, nil)
	if s.Total != 0 || s.Count != 0 || s.AverageTicket != 0 {
		t.Errorf("resumo vazio deveria ser zerado: %+v", s)
	}
}

// helper que reusa a lógica pública de referência para montar buckets no teste
func creditcardBuckets(txns []shared.CardTransaction, closingDay int) (map[string][]shared.CardTransaction, error) {
	buckets := map[string][]shared.CardTransaction{}
	for _, t := range txns {
		ref, err := creditcard.InvoiceReferenceFor(t.CompetenceDate, closingDay)
		if err != nil {
			return nil, err
		}
		buckets[ref] = append(buckets[ref], t)
	}
	return buckets, nil
}
