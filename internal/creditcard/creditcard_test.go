package creditcard_test

import (
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/creditcard"
)

func msgCount(err error) int {
	if err == nil {
		return 0
	}
	if de, ok := domainerr.IsDomain(err); ok {
		return len(de.Messages())
	}
	return 1
}

func ptr[T any](v T) *T { return &v }

// ─── Ciclo: ClosingDate ─────────────────────────────────────────────────────

func TestClosingDate(t *testing.T) {
	cases := []struct {
		ref        string
		closingDay int
		want       string
	}{
		{"2026-07", 3, "2026-07-03"},
		{"2026-02", 31, "2026-02-28"}, // clamp
		{"2028-02", 31, "2028-02-29"}, // bissexto
		{"2026-04", 31, "2026-04-30"}, // clamp 30
		{"2026-12", 15, "2026-12-15"},
	}
	for _, tc := range cases {
		got, err := creditcard.ClosingDate(tc.ref, tc.closingDay)
		if err != nil {
			t.Fatalf("ClosingDate(%q,%d): %v", tc.ref, tc.closingDay, err)
		}
		if got != tc.want {
			t.Errorf("ClosingDate(%q,%d) = %q, want %q", tc.ref, tc.closingDay, got, tc.want)
		}
	}
}

func TestClosingDate_InvalidReference(t *testing.T) {
	if _, err := creditcard.ClosingDate("2026/07", 3); err == nil {
		t.Error("expected error for invalid reference")
	}
}

// ─── Ciclo: DueDate ─────────────────────────────────────────────────────────

func TestDueDate(t *testing.T) {
	cases := []struct {
		ref                string
		closingDay, dueDay int
		want               string
	}{
		{"2026-07", 3, 10, "2026-07-10"},  // dueDay>closingDay, mesmo mês
		{"2026-07", 20, 5, "2026-08-05"},  // dueDay<=closingDay, mês seguinte
		{"2026-12", 20, 5, "2027-01-05"},  // virada de ano
		{"2026-01", 31, 31, "2026-02-28"}, // dueDay<=closingDay → fev, clamp
		{"2026-07", 3, 3, "2026-08-03"},   // dueDay==closingDay → mês seguinte
	}
	for _, tc := range cases {
		got, err := creditcard.DueDate(tc.ref, tc.closingDay, tc.dueDay)
		if err != nil {
			t.Fatalf("DueDate(%q,%d,%d): %v", tc.ref, tc.closingDay, tc.dueDay, err)
		}
		if got != tc.want {
			t.Errorf("DueDate(%q,%d,%d) = %q, want %q", tc.ref, tc.closingDay, tc.dueDay, got, tc.want)
		}
	}
}

// ─── Ciclo: CycleStart ──────────────────────────────────────────────────────

func TestCycleStart(t *testing.T) {
	cases := []struct {
		ref        string
		closingDay int
		want       string
	}{
		{"2026-07", 3, "2026-06-04"},
		{"2026-03", 31, "2026-03-01"}, // fev clampa em 28 → +1 dia = 01/mar
		{"2026-01", 10, "2025-12-11"}, // virada de ano pra trás
	}
	for _, tc := range cases {
		got, err := creditcard.CycleStart(tc.ref, tc.closingDay)
		if err != nil {
			t.Fatalf("CycleStart(%q,%d): %v", tc.ref, tc.closingDay, err)
		}
		if got != tc.want {
			t.Errorf("CycleStart(%q,%d) = %q, want %q", tc.ref, tc.closingDay, got, tc.want)
		}
	}
}

// ─── Ciclo: InvoiceReferenceFor (o mais crítico) ────────────────────────────

func TestInvoiceReferenceFor(t *testing.T) {
	cases := []struct {
		purchase   string
		closingDay int
		want       string
	}{
		{"2026-07-02", 3, "2026-07"},
		{"2026-07-03", 3, "2026-07"}, // exatamente no fechamento → intervalo (prev, atual]
		{"2026-07-05", 3, "2026-08"},
		{"2026-06-04", 3, "2026-07"},  // dia seguinte ao fechamento → próxima fatura
		{"2026-02-28", 31, "2026-02"}, // clamp: 28 <= effClosing 28
		{"2026-12-31", 5, "2027-01"},  // virada de ano
	}
	for _, tc := range cases {
		got, err := creditcard.InvoiceReferenceFor(tc.purchase, tc.closingDay)
		if err != nil {
			t.Fatalf("InvoiceReferenceFor(%q,%d): %v", tc.purchase, tc.closingDay, err)
		}
		if got != tc.want {
			t.Errorf("InvoiceReferenceFor(%q,%d) = %q, want %q", tc.purchase, tc.closingDay, got, tc.want)
		}
	}
}

func TestInvoiceReferenceFor_InvalidDate(t *testing.T) {
	if _, err := creditcard.InvoiceReferenceFor("31/12/2026", 5); err == nil {
		t.Error("expected error for invalid purchase date")
	}
}

// ─── Ciclo: BestPurchaseDay ─────────────────────────────────────────────────

func TestBestPurchaseDay(t *testing.T) {
	cases := []struct {
		closingDay, want int
	}{
		{3, 4}, {27, 28}, {28, 1}, {30, 1}, {31, 1}, {1, 2},
	}
	for _, tc := range cases {
		if got := creditcard.BestPurchaseDay(tc.closingDay); got != tc.want {
			t.Errorf("BestPurchaseDay(%d) = %d, want %d", tc.closingDay, got, tc.want)
		}
	}
}

// ─── Ciclo: DeriveInvoiceStatus ─────────────────────────────────────────────

func TestDeriveInvoiceStatus(t *testing.T) {
	// ciclo: start 2026-06-04, closing 2026-07-03, due 2026-07-10
	const start, closing, due = "2026-06-04", "2026-07-03", "2026-07-10"
	cases := []struct {
		name       string
		today      string
		hasPayment bool
		want       creditcard.InvoiceStatus
	}{
		{"pagamento vence tudo", "2026-08-01", true, creditcard.StatusPaga},
		{"antes do ciclo", "2026-06-01", false, creditcard.StatusFutura},
		{"início do ciclo", "2026-06-04", false, creditcard.StatusAberta},
		{"meio do ciclo", "2026-06-20", false, creditcard.StatusAberta},
		{"dia do fechamento", "2026-07-03", false, creditcard.StatusAberta},
		{"fechada (entre fechamento e vencimento)", "2026-07-05", false, creditcard.StatusFechada},
		{"dia do vencimento", "2026-07-10", false, creditcard.StatusFechada},
		{"vencida", "2026-07-11", false, creditcard.StatusVencida},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := creditcard.DeriveInvoiceStatus(tc.today, start, closing, due, tc.hasPayment)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ─── Utilização ─────────────────────────────────────────────────────────────

func TestUtilizationPercent(t *testing.T) {
	cases := []struct {
		used, limit int64
		want        int
	}{
		{0, 500000, 0},
		{132000, 500000, 26},
		{460000, 500000, 92},
		{500000, 500000, 100},
		{600000, 500000, 120}, // estouro
		{100, 0, 0},           // limite zero → 0
	}
	for _, tc := range cases {
		if got := creditcard.UtilizationPercent(tc.used, tc.limit); got != tc.want {
			t.Errorf("UtilizationPercent(%d,%d) = %d, want %d", tc.used, tc.limit, got, tc.want)
		}
	}
}

func TestClassifyUtilization(t *testing.T) {
	cases := []struct {
		percent int
		want    creditcard.UtilizationLevel
	}{
		{0, creditcard.LevelSaudavel},
		{29, creditcard.LevelSaudavel},
		{30, creditcard.LevelAtencao},
		{69, creditcard.LevelAtencao},
		{70, creditcard.LevelAlto},
		{90, creditcard.LevelAlto},
		{91, creditcard.LevelCritico},
		{150, creditcard.LevelCritico},
	}
	for _, tc := range cases {
		if got := creditcard.ClassifyUtilization(tc.percent); got != tc.want {
			t.Errorf("ClassifyUtilization(%d) = %q, want %q", tc.percent, got, tc.want)
		}
	}
}

// ─── Validação ──────────────────────────────────────────────────────────────

func TestValidateCreate(t *testing.T) {
	valid := creditcard.CreateCreditCardInput{
		Name:        "Nubank Roxinho",
		Brand:       creditcard.BrandMastercard,
		CreditLimit: 500000,
		ClosingDay:  3,
		DueDay:      10,
	}

	cases := []struct {
		name          string
		modify        func(*creditcard.CreateCreditCardInput)
		wantErr       bool
		wantMsgCount  int
		wantMsgSubstr string
	}{
		{name: "válido mínimo", wantErr: false},
		{
			name: "válido completo",
			modify: func(in *creditcard.CreateCreditCardInput) {
				in.LastFourDigits = ptr("1234")
				in.Issuer = ptr("Nubank")
				in.Color = ptr("#820AD1")
				in.Icon = ptr("credit-card")
			},
			wantErr: false,
		},
		{name: "nome vazio", modify: func(in *creditcard.CreateCreditCardInput) { in.Name = "" }, wantErr: true, wantMsgSubstr: "nome é obrigatório"},
		{name: "nome > 80", modify: func(in *creditcard.CreateCreditCardInput) { in.Name = strings.Repeat("x", 81) }, wantErr: true, wantMsgSubstr: "no máximo 80"},
		{name: "bandeira inválida", modify: func(in *creditcard.CreateCreditCardInput) { in.Brand = "diners" }, wantErr: true, wantMsgSubstr: "bandeira inválida"},
		{name: "limite zero", modify: func(in *creditcard.CreateCreditCardInput) { in.CreditLimit = 0 }, wantErr: true, wantMsgSubstr: "limite do cartão deve ser maior que zero"},
		{name: "limite negativo", modify: func(in *creditcard.CreateCreditCardInput) { in.CreditLimit = -1 }, wantErr: true, wantMsgSubstr: "limite do cartão deve ser maior que zero"},
		{name: "closingDay 0", modify: func(in *creditcard.CreateCreditCardInput) { in.ClosingDay = 0 }, wantErr: true, wantMsgSubstr: "dia de fechamento inválido"},
		{name: "closingDay 32", modify: func(in *creditcard.CreateCreditCardInput) { in.ClosingDay = 32 }, wantErr: true, wantMsgSubstr: "dia de fechamento inválido"},
		{name: "dueDay 0", modify: func(in *creditcard.CreateCreditCardInput) { in.DueDay = 0 }, wantErr: true, wantMsgSubstr: "dia de vencimento inválido"},
		{name: "dueDay 32", modify: func(in *creditcard.CreateCreditCardInput) { in.DueDay = 32 }, wantErr: true, wantMsgSubstr: "dia de vencimento inválido"},
		{name: "lastFour não numérico", modify: func(in *creditcard.CreateCreditCardInput) { in.LastFourDigits = ptr("12ab") }, wantErr: true, wantMsgSubstr: "últimos 4 dígitos"},
		{name: "lastFour curto", modify: func(in *creditcard.CreateCreditCardInput) { in.LastFourDigits = ptr("123") }, wantErr: true, wantMsgSubstr: "últimos 4 dígitos"},
		{name: "issuer > 60", modify: func(in *creditcard.CreateCreditCardInput) { in.Issuer = ptr(strings.Repeat("x", 61)) }, wantErr: true, wantMsgSubstr: "emissor deve ter no máximo"},
		{name: "cor inválida", modify: func(in *creditcard.CreateCreditCardInput) { in.Color = ptr("vermelho") }, wantErr: true, wantMsgSubstr: "cor deve estar no formato"},
		{
			name: "múltiplos erros: nome + limite",
			modify: func(in *creditcard.CreateCreditCardInput) {
				in.Name = ""
				in.CreditLimit = 0
			},
			wantErr:      true,
			wantMsgCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			if tc.modify != nil {
				tc.modify(&in)
			}
			err := creditcard.ValidateCreate(in)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantMsgCount > 0 && msgCount(err) != tc.wantMsgCount {
				t.Fatalf("want %d messages, got %d: %v", tc.wantMsgCount, msgCount(err), err)
			}
			if tc.wantMsgSubstr != "" && !strings.Contains(err.Error(), tc.wantMsgSubstr) {
				t.Fatalf("want message containing %q, got: %v", tc.wantMsgSubstr, err)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	valid := creditcard.UpdateCreditCardInput{
		ID:          "card-1",
		Name:        "Inter Black",
		Brand:       creditcard.BrandVisa,
		CreditLimit: 1000000,
		ClosingDay:  15,
		DueDay:      22,
	}
	if err := creditcard.ValidateUpdate(valid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	valid.CreditLimit = 0
	if err := creditcard.ValidateUpdate(valid); err == nil {
		t.Error("expected error for zero limit")
	}
}
