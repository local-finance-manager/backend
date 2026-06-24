package installment_test

import (
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/installment"
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

func sum(xs []int64) int64 {
	var t int64
	for _, x := range xs {
		t += x
	}
	return t
}

// ─── Rateio de centavos (invariante Σ == total) ─────────────────────────────

func TestComputeAmounts(t *testing.T) {
	cases := []struct {
		total int64
		n     int
		want  []int64
	}{
		{500000, 10, []int64{50000, 50000, 50000, 50000, 50000, 50000, 50000, 50000, 50000, 50000}},
		{10000, 3, []int64{3334, 3333, 3333}},     // R$100 / 3 → 1ª maior
		{10, 3, []int64{4, 3, 3}},                 // resto 1
		{1, 2, []int64{1, 0}},                     // total mínimo
		{7, 7, []int64{1, 1, 1, 1, 1, 1, 1}},      // exato
		{100, 6, []int64{17, 17, 17, 17, 16, 16}}, // resto 4
	}
	for _, tc := range cases {
		got := installment.ComputeAmounts(tc.total, tc.n)
		if len(got) != tc.n {
			t.Fatalf("ComputeAmounts(%d,%d): len %d, want %d", tc.total, tc.n, len(got), tc.n)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("ComputeAmounts(%d,%d)[%d] = %d, want %d", tc.total, tc.n, i, got[i], tc.want[i])
			}
		}
		if s := sum(got); s != tc.total {
			t.Errorf("INVARIANTE QUEBRADA: soma=%d, total=%d", s, tc.total)
		}
	}
}

// Invariante exaustiva: para muitos (total, n), soma das parcelas == total.
func TestComputeAmounts_InvariantExhaustive(t *testing.T) {
	for total := int64(1); total <= 1000; total++ {
		for n := 2; n <= 72; n++ {
			if s := sum(installment.ComputeAmounts(total, n)); s != total {
				t.Fatalf("Σ != total para total=%d n=%d (soma=%d)", total, n, s)
			}
		}
	}
}

func TestResolvePlan(t *testing.T) {
	// by_total
	total, amounts, err := installment.ResolvePlan(installment.ByTotal, 10000, 0, 3)
	if err != nil {
		t.Fatalf("by_total: %v", err)
	}
	if total != 10000 || sum(amounts) != 10000 || len(amounts) != 3 {
		t.Errorf("by_total inesperado: total=%d amounts=%v", total, amounts)
	}

	// by_installment
	total, amounts, err = installment.ResolvePlan(installment.ByInstallment, 0, 55000, 10)
	if err != nil {
		t.Fatalf("by_installment: %v", err)
	}
	if total != 550000 || len(amounts) != 10 {
		t.Errorf("by_installment: total=%d len=%d, want 550000/10", total, len(amounts))
	}
	for _, a := range amounts {
		if a != 55000 {
			t.Errorf("by_installment: parcela %d != 55000", a)
		}
	}

	// modo inválido
	if _, _, err := installment.ResolvePlan("xpto", 1, 1, 2); err == nil {
		t.Error("modo inválido deveria falhar")
	}
}

func TestInterest(t *testing.T) {
	cases := []struct {
		total     int64
		principal *int64
		want      int64
	}{
		{550000, ptr(int64(500000)), 50000},
		{500000, ptr(int64(500000)), 0}, // sem juros
		{500000, nil, 0},                // sem principal
		{400000, ptr(int64(500000)), 0}, // principal > total → 0
		{550000, ptr(int64(0)), 0},      // principal zero ignorado
	}
	for _, tc := range cases {
		if got := installment.Interest(tc.total, tc.principal); got != tc.want {
			t.Errorf("Interest(%d, %v) = %d, want %d", tc.total, tc.principal, got, tc.want)
		}
	}
}

// ─── Cronograma de competências (clamp) ─────────────────────────────────────

func TestCompetenceSchedule(t *testing.T) {
	cases := []struct {
		purchase string
		n        int
		want     []string
	}{
		{"2026-06-22", 3, []string{"2026-06-22", "2026-07-22", "2026-08-22"}},
		{"2026-01-31", 3, []string{"2026-01-31", "2026-02-28", "2026-03-31"}}, // clamp fev
		{"2028-01-31", 2, []string{"2028-01-31", "2028-02-29"}},               // bissexto
		{"2026-11-15", 3, []string{"2026-11-15", "2026-12-15", "2027-01-15"}}, // virada de ano
		{"2026-12-31", 2, []string{"2026-12-31", "2027-01-31"}},
	}
	for _, tc := range cases {
		got, err := installment.CompetenceSchedule(tc.purchase, tc.n)
		if err != nil {
			t.Fatalf("CompetenceSchedule(%q,%d): %v", tc.purchase, tc.n, err)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("CompetenceSchedule(%q,%d)[%d] = %q, want %q", tc.purchase, tc.n, i, got[i], tc.want[i])
			}
		}
	}
}

func TestCompetenceSchedule_InvalidDate(t *testing.T) {
	if _, err := installment.CompetenceSchedule("22/06/2026", 3); err == nil {
		t.Error("data inválida deveria falhar")
	}
}

// ─── Status do grupo ────────────────────────────────────────────────────────

func TestDeriveGroupStatus(t *testing.T) {
	cases := []struct {
		c    installment.StatusCounts
		want installment.GroupStatus
	}{
		{installment.StatusCounts{Pending: 7, Realized: 3}, installment.GroupActive},
		{installment.StatusCounts{Realized: 10}, installment.GroupSettled},
		{installment.StatusCounts{Realized: 3, Cancelled: 7}, installment.GroupCancelled},
		{installment.StatusCounts{Cancelled: 10}, installment.GroupCancelled},
		{installment.StatusCounts{Pending: 1, Cancelled: 9}, installment.GroupActive}, // pending vence
	}
	for _, tc := range cases {
		if got := installment.DeriveGroupStatus(tc.c); got != tc.want {
			t.Errorf("DeriveGroupStatus(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

// ─── Validação ──────────────────────────────────────────────────────────────

func TestValidateCreate(t *testing.T) {
	valid := installment.CreateInput{
		CreditCardID:      "card-1",
		SubcategoryID:     "sub-1",
		Title:             "Notebook Dell",
		InstallmentsCount: 10,
		InputMode:         installment.ByTotal,
		TotalAmount:       500000,
		PurchaseDate:      "2026-06-22",
	}

	cases := []struct {
		name          string
		modify        func(*installment.CreateInput)
		wantErr       bool
		wantMsgCount  int
		wantMsgSubstr string
	}{
		{name: "válido by_total", wantErr: false},
		{
			name: "válido by_installment",
			modify: func(in *installment.CreateInput) {
				in.InputMode = installment.ByInstallment
				in.TotalAmount = 0
				in.InstallmentAmount = 55000
			},
			wantErr: false,
		},
		{name: "título vazio", modify: func(in *installment.CreateInput) { in.Title = "" }, wantErr: true, wantMsgSubstr: "título é obrigatório"},
		{name: "título > 150", modify: func(in *installment.CreateInput) { in.Title = strings.Repeat("x", 151) }, wantErr: true, wantMsgSubstr: "no máximo 150"},
		{name: "cartão vazio", modify: func(in *installment.CreateInput) { in.CreditCardID = "" }, wantErr: true, wantMsgSubstr: "cartão é obrigatório"},
		{name: "subcategoria vazia", modify: func(in *installment.CreateInput) { in.SubcategoryID = "" }, wantErr: true, wantMsgSubstr: "subcategoria é obrigatória"},
		{name: "N = 1", modify: func(in *installment.CreateInput) { in.InstallmentsCount = 1 }, wantErr: true, wantMsgSubstr: "entre 2 e 72"},
		{name: "N = 73", modify: func(in *installment.CreateInput) { in.InstallmentsCount = 73 }, wantErr: true, wantMsgSubstr: "entre 2 e 72"},
		{name: "by_total sem total", modify: func(in *installment.CreateInput) { in.TotalAmount = 0 }, wantErr: true, wantMsgSubstr: "valor total deve ser maior que zero"},
		{
			name: "by_installment sem parcela",
			modify: func(in *installment.CreateInput) {
				in.InputMode = installment.ByInstallment
				in.TotalAmount = 0
				in.InstallmentAmount = 0
			},
			wantErr: true, wantMsgSubstr: "valor da parcela deve ser maior que zero",
		},
		{name: "modo inválido", modify: func(in *installment.CreateInput) { in.InputMode = "xpto" }, wantErr: true, wantMsgSubstr: "modo de entrada inválido"},
		{name: "data inválida", modify: func(in *installment.CreateInput) { in.PurchaseDate = "22/06/2026" }, wantErr: true, wantMsgSubstr: "data da compra inválida"},
		{name: "data vazia", modify: func(in *installment.CreateInput) { in.PurchaseDate = "" }, wantErr: true, wantMsgSubstr: "data da compra é obrigatória"},
		{name: "descrição > 1000", modify: func(in *installment.CreateInput) { in.Description = ptr(strings.Repeat("x", 1001)) }, wantErr: true, wantMsgSubstr: "no máximo 1.000"},
		{name: "principal zero", modify: func(in *installment.CreateInput) { in.PrincipalAmount = ptr(int64(0)) }, wantErr: true, wantMsgSubstr: "valor à vista"},
		{
			name: "múltiplos erros",
			modify: func(in *installment.CreateInput) {
				in.Title = ""
				in.InstallmentsCount = 1
			},
			wantErr: true, wantMsgCount: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			if tc.modify != nil {
				tc.modify(&in)
			}
			err := installment.ValidateCreate(in)
			if tc.wantErr && err == nil {
				t.Fatal("esperava erro, veio nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if tc.wantMsgCount > 0 && msgCount(err) != tc.wantMsgCount {
				t.Fatalf("want %d msgs, got %d: %v", tc.wantMsgCount, msgCount(err), err)
			}
			if tc.wantMsgSubstr != "" && !strings.Contains(err.Error(), tc.wantMsgSubstr) {
				t.Fatalf("want msg contendo %q, got: %v", tc.wantMsgSubstr, err)
			}
		})
	}
}

func TestValidateUpdateSeries(t *testing.T) {
	valid := installment.UpdateSeriesInput{ID: "g1", Title: "Novo", SubcategoryID: "sub-1"}
	if err := installment.ValidateUpdateSeries(valid); err != nil {
		t.Fatalf("válido: %v", err)
	}
	valid.Title = ""
	if err := installment.ValidateUpdateSeries(valid); err == nil {
		t.Error("título vazio deveria falhar")
	}
}
