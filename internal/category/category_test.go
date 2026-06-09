package category

import (
	"strings"
	"testing"

	"github.com/Lucas-Lopes-II/govalidator/domainerr"
)

// ─── ValidateCreateCategory ───────────────────────────────────────────────────

func TestValidateCreateCategory(t *testing.T) {
	tests := []struct {
		name         string
		in           CreateCategoryInput
		wantErr      bool
		wantMsgCount int // 0 = not checked; >0 = expect CompositeErr with N messages
	}{
		{
			name:    "valid despesa",
			in:      CreateCategoryInput{Name: "Moradia", Type: Expense},
			wantErr: false,
		},
		{
			name:    "valid receita",
			in:      CreateCategoryInput{Name: "Salário", Type: Income},
			wantErr: false,
		},
		{
			name:    "valid transferencia",
			in:      CreateCategoryInput{Name: "PIX", Type: Transfer},
			wantErr: false,
		},
		{
			name:    "empty name",
			in:      CreateCategoryInput{Name: "", Type: Expense},
			wantErr: true,
		},
		{
			name:    "whitespace-only name",
			in:      CreateCategoryInput{Name: "   ", Type: Expense},
			wantErr: true,
		},
		{
			name:    "name too long",
			in:      CreateCategoryInput{Name: strings.Repeat("a", 101), Type: Expense},
			wantErr: true,
		},
		{
			name:    "name exactly at limit",
			in:      CreateCategoryInput{Name: strings.Repeat("a", 100), Type: Expense},
			wantErr: false,
		},
		{
			name:    "invalid type",
			in:      CreateCategoryInput{Name: "Test", Type: "invalido"},
			wantErr: true,
		},
		{
			name:    "empty type",
			in:      CreateCategoryInput{Name: "Test", Type: ""},
			wantErr: true,
		},
		// ─── Color validation ─────────────────────────────────────────────
		{
			name:    "valid hex color",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Color: "#A1B2C3"},
			wantErr: false,
		},
		{
			name:    "lowercase hex color",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Color: "#a1b2c3"},
			wantErr: false,
		},
		{
			name:    "empty color is optional",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Color: ""},
			wantErr: false,
		},
		{
			name:    "invalid color format",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Color: "red"},
			wantErr: true,
		},
		{
			name:    "color without hash",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Color: "A1B2C3"},
			wantErr: true,
		},
		// ─── Icon validation ──────────────────────────────────────────────
		{
			name:    "icon at max length",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Icon: strings.Repeat("i", 100)},
			wantErr: false,
		},
		{
			name:    "icon too long",
			in:      CreateCategoryInput{Name: "Test", Type: Expense, Icon: strings.Repeat("i", 101)},
			wantErr: true,
		},
		// ─── Multi-error accumulation ──────────────────────────────────────
		{
			name:         "empty name AND invalid type → two errors accumulated",
			in:           CreateCategoryInput{Name: "", Type: "invalido"},
			wantErr:      true,
			wantMsgCount: 2,
		},
		{
			name:         "name too long AND invalid type → two errors accumulated",
			in:           CreateCategoryInput{Name: strings.Repeat("x", 101), Type: ""},
			wantErr:      true,
			wantMsgCount: 2,
		},
		{
			name:         "invalid color AND invalid type → two errors accumulated",
			in:           CreateCategoryInput{Name: "Test", Type: "invalido", Color: "not-a-color"},
			wantErr:      true,
			wantMsgCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateCategory(tc.in)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.wantMsgCount > 0 {
				de, ok := domainerr.IsDomain(err)
				if !ok {
					t.Fatalf("expected DomainError, got %T", err)
				}
				msgs := de.Messages()
				if len(msgs) != tc.wantMsgCount {
					t.Errorf("message count: got %d, want %d (msgs: %v)", len(msgs), tc.wantMsgCount, msgs)
				}
			}
		})
	}
}

// ─── ValidateUpdateCategory ───────────────────────────────────────────────────

func TestValidateUpdateCategory(t *testing.T) {
	tests := []struct {
		name    string
		in      UpdateCategoryInput
		wantErr bool
	}{
		{
			name:    "valid update",
			in:      UpdateCategoryInput{ID: "cat-1", Name: "Novo Nome"},
			wantErr: false,
		},
		{
			name:    "empty name",
			in:      UpdateCategoryInput{ID: "cat-1", Name: ""},
			wantErr: true,
		},
		{
			name:    "whitespace name",
			in:      UpdateCategoryInput{ID: "cat-1", Name: "\t\n"},
			wantErr: true,
		},
		{
			name:    "name too long",
			in:      UpdateCategoryInput{ID: "cat-1", Name: strings.Repeat("x", 101)},
			wantErr: true,
		},
		{
			name:    "valid color",
			in:      UpdateCategoryInput{ID: "cat-1", Name: "Nome", Color: "#FF0000"},
			wantErr: false,
		},
		{
			name:    "invalid color",
			in:      UpdateCategoryInput{ID: "cat-1", Name: "Nome", Color: "#ZZZ"},
			wantErr: true,
		},
		{
			name:    "icon too long",
			in:      UpdateCategoryInput{ID: "cat-1", Name: "Nome", Icon: strings.Repeat("i", 101)},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUpdateCategory(tc.in)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ─── ValidateCreateSubcategory ────────────────────────────────────────────────

func TestValidateCreateSubcategory(t *testing.T) {
	tests := []struct {
		name    string
		in      CreateSubcategoryInput
		wantErr bool
	}{
		{
			name:    "valid input",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: "Aluguel"},
			wantErr: false,
		},
		{
			name:    "empty category_id",
			in:      CreateSubcategoryInput{CategoryID: "", Name: "Aluguel"},
			wantErr: true,
		},
		{
			name:    "empty name",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: ""},
			wantErr: true,
		},
		{
			name:    "name too long",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: strings.Repeat("n", 101)},
			wantErr: true,
		},
		{
			name:    "valid color",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: "Sub", Color: "#FF0000"},
			wantErr: false,
		},
		{
			name:    "invalid color",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: "Sub", Color: "blue"},
			wantErr: true,
		},
		{
			name:    "icon too long",
			in:      CreateSubcategoryInput{CategoryID: "cat-1", Name: "Sub", Icon: strings.Repeat("i", 101)},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateSubcategory(tc.in)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ─── ValidateUpdateSubcategory ────────────────────────────────────────────────

func TestValidateUpdateSubcategory(t *testing.T) {
	tests := []struct {
		name    string
		in      UpdateSubcategoryInput
		wantErr bool
	}{
		{
			name:    "valid update",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: "Água"},
			wantErr: false,
		},
		{
			name:    "empty name",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: ""},
			wantErr: true,
		},
		{
			name:    "name too long",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: strings.Repeat("y", 101)},
			wantErr: true,
		},
		{
			name:    "valid color",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: "Sub", Color: "#1A2B3C"},
			wantErr: false,
		},
		{
			name:    "invalid color",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: "Sub", Color: "#GGG"},
			wantErr: true,
		},
		{
			name:    "icon too long",
			in:      UpdateSubcategoryInput{ID: "sub-1", Name: "Sub", Icon: strings.Repeat("i", 101)},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUpdateSubcategory(tc.in)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
