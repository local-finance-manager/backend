package transaction_test

import (
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"

	"github.com/local-finance-manager/backend/internal/transaction"
)

// msgCount returns the number of validation messages in an error.
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

// ─── ValidateCreate ───────────────────────────────────────────────────────────

func TestValidateCreate(t *testing.T) {
	validBase := transaction.CreateTransactionInput{
		Title:          "Aluguel",
		Amount:         150000,
		SubcategoryID:  "sub-1",
		PaymentMethod:  transaction.MethodPix,
		Status:         transaction.StatusPendente,
		CompetenceDate: "2026-07-01",
	}

	cases := []struct {
		name          string
		modify        func(*transaction.CreateTransactionInput)
		wantErr       bool
		wantMsgCount  int
		wantMsgSubstr string
	}{
		{
			name:    "válido — pendente sem paymentDate",
			wantErr: false,
		},
		{
			name: "válido — realizado com paymentDate",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusRealizado
				in.PaymentDate = ptr("2026-07-01")
			},
			wantErr: false,
		},
		{
			name: "válido — cancelado sem paymentDate",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusCancelado
			},
			wantErr: false,
		},
		{
			name:          "título vazio",
			modify:        func(in *transaction.CreateTransactionInput) { in.Title = "" },
			wantErr:       true,
			wantMsgSubstr: "título é obrigatório",
		},
		{
			name:          "título > 150 chars",
			modify:        func(in *transaction.CreateTransactionInput) { in.Title = strings.Repeat("x", 151) },
			wantErr:       true,
			wantMsgSubstr: "título deve ter no máximo 150 caracteres",
		},
		{
			name:          "amount zero",
			modify:        func(in *transaction.CreateTransactionInput) { in.Amount = 0 },
			wantErr:       true,
			wantMsgSubstr: "valor deve ser maior que zero",
		},
		{
			name:          "amount negativo",
			modify:        func(in *transaction.CreateTransactionInput) { in.Amount = -1 },
			wantErr:       true,
			wantMsgSubstr: "valor deve ser maior que zero",
		},
		{
			name:          "paymentMethod inválido",
			modify:        func(in *transaction.CreateTransactionInput) { in.PaymentMethod = "cheque" },
			wantErr:       true,
			wantMsgSubstr: "forma de pagamento inválida",
		},
		{
			name:          "status inválido",
			modify:        func(in *transaction.CreateTransactionInput) { in.Status = "agendado" },
			wantErr:       true,
			wantMsgSubstr: "status inválido",
		},
		{
			name:          "subcategoryId vazio",
			modify:        func(in *transaction.CreateTransactionInput) { in.SubcategoryID = "" },
			wantErr:       true,
			wantMsgSubstr: "subcategoryId é obrigatório",
		},
		{
			name:          "competenceDate vazio",
			modify:        func(in *transaction.CreateTransactionInput) { in.CompetenceDate = "" },
			wantErr:       true,
			wantMsgSubstr: "data de competência é obrigatória",
		},
		{
			name:          "competenceDate formato errado",
			modify:        func(in *transaction.CreateTransactionInput) { in.CompetenceDate = "01/07/2026" },
			wantErr:       true,
			wantMsgSubstr: "data de competência inválida",
		},
		{
			name: "realizado sem paymentDate",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusRealizado
				in.PaymentDate = nil
			},
			wantErr:       true,
			wantMsgSubstr: "data de pagamento é obrigatória",
		},
		{
			name: "realizado, paymentDate formato errado",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusRealizado
				in.PaymentDate = ptr("01/07")
			},
			wantErr:       true,
			wantMsgSubstr: "data de pagamento inválida",
		},
		{
			name: "pendente com paymentDate",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusPendente
				in.PaymentDate = ptr("2026-07-01")
			},
			wantErr:       true,
			wantMsgSubstr: "data de pagamento deve ser nula",
		},
		{
			name: "cancelado com paymentDate",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Status = transaction.StatusCancelado
				in.PaymentDate = ptr("2026-07-01")
			},
			wantErr:       true,
			wantMsgSubstr: "data de pagamento deve ser nula",
		},
		{
			name: "descrição > 1000 chars",
			modify: func(in *transaction.CreateTransactionInput) {
				s := strings.Repeat("x", 1001)
				in.Description = &s
			},
			wantErr:       true,
			wantMsgSubstr: "descrição deve ter no máximo",
		},
		{
			name: "múltiplos erros: título + amount",
			modify: func(in *transaction.CreateTransactionInput) {
				in.Title = ""
				in.Amount = 0
			},
			wantErr:      true,
			wantMsgCount: 2,
		},
		{
			name: "creditCardId com paymentMethod != cartao_credito (D8)",
			modify: func(in *transaction.CreateTransactionInput) {
				in.PaymentMethod = transaction.MethodPix
				cardID := "card-1"
				in.CreditCardID = &cardID
			},
			wantErr:       true,
			wantMsgSubstr: "cartão de crédito só pode ser vinculado",
		},
		{
			name: "válido — creditCardId com cartao_credito",
			modify: func(in *transaction.CreateTransactionInput) {
				in.PaymentMethod = transaction.MethodCartaoCredito
				cardID := "card-1"
				in.CreditCardID = &cardID
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validBase
			if tc.modify != nil {
				tc.modify(&in)
			}
			err := transaction.ValidateCreate(in)
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

// ─── ValidateUpdate ───────────────────────────────────────────────────────────

func TestValidateUpdate(t *testing.T) {
	validBase := transaction.UpdateTransactionInput{
		ID:             "txn-1",
		Title:          "Aluguel",
		Amount:         150000,
		SubcategoryID:  "sub-1",
		PaymentMethod:  transaction.MethodPix,
		Status:         transaction.StatusPendente,
		CompetenceDate: "2026-07-01",
	}

	cases := []struct {
		name    string
		modify  func(*transaction.UpdateTransactionInput)
		wantErr bool
	}{
		{"válido — pendente", nil, false},
		{
			"válido — realizado com paymentDate",
			func(in *transaction.UpdateTransactionInput) {
				in.Status = transaction.StatusRealizado
				in.PaymentDate = ptr("2026-07-01")
			},
			false,
		},
		{"título vazio", func(in *transaction.UpdateTransactionInput) { in.Title = "" }, true},
		{"amount zero", func(in *transaction.UpdateTransactionInput) { in.Amount = 0 }, true},
		{"paymentMethod inválido", func(in *transaction.UpdateTransactionInput) { in.PaymentMethod = "boleto_flash" }, true},
		{
			"realizado sem paymentDate",
			func(in *transaction.UpdateTransactionInput) {
				in.Status = transaction.StatusRealizado
				in.PaymentDate = nil
			},
			true,
		},
		{
			"pendente com paymentDate",
			func(in *transaction.UpdateTransactionInput) { in.PaymentDate = ptr("2026-07-01") },
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validBase
			if tc.modify != nil {
				tc.modify(&in)
			}
			err := transaction.ValidateUpdate(in)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ─── ValidateConfirm ──────────────────────────────────────────────────────────

func TestValidateConfirm(t *testing.T) {
	cases := []struct {
		name    string
		in      transaction.ConfirmTransactionInput
		wantErr bool
	}{
		{"válido", transaction.ConfirmTransactionInput{ID: "x", PaymentDate: "2026-07-01"}, false},
		{"paymentDate vazio", transaction.ConfirmTransactionInput{ID: "x", PaymentDate: ""}, true},
		{"paymentDate formato errado", transaction.ConfirmTransactionInput{ID: "x", PaymentDate: "07/2026"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := transaction.ValidateConfirm(tc.in)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ─── CanTransitionTo ──────────────────────────────────────────────────────────

func TestCanTransitionTo(t *testing.T) {
	cases := []struct {
		from, to transaction.TransactionStatus
		want     bool
	}{
		{transaction.StatusPendente, transaction.StatusRealizado, true},
		{transaction.StatusPendente, transaction.StatusCancelado, true},
		{transaction.StatusPendente, transaction.StatusPendente, false},
		{transaction.StatusRealizado, transaction.StatusPendente, true},
		{transaction.StatusRealizado, transaction.StatusCancelado, true},
		{transaction.StatusRealizado, transaction.StatusRealizado, false},
		{transaction.StatusCancelado, transaction.StatusPendente, true},
		{transaction.StatusCancelado, transaction.StatusRealizado, false}, // critical rule
		{transaction.StatusCancelado, transaction.StatusCancelado, false},
	}
	for _, tc := range cases {
		got := transaction.CanTransitionTo(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransitionTo(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}
