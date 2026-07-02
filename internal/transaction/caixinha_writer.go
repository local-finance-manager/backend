package transaction

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// CaixinhaWriter satisfaz patrimonio.MovementWriter (structural typing): cria/exclui
// os lançamentos neutros de aporte/resgate passando pelos use cases (que derivam o
// tipo `transferencia` da subcategoria de sistema e aplicam validações/guards).
// Definido aqui (produtor); injetado no main.go.
type CaixinhaWriter struct {
	create CreateTransactionUseCase
	delete DeleteTransactionUseCase
}

// NewCaixinhaWriter cria o writer reusando os use cases de criação/exclusão.
func NewCaixinhaWriter(create CreateTransactionUseCase, del DeleteTransactionUseCase) *CaixinhaWriter {
	return &CaixinhaWriter{create: create, delete: del}
}

// Register cria o lançamento neutro do movimento e devolve o id criado. Aporte usa a
// subcategoria de sistema `sub-caixinha-aporte`; resgate `sub-caixinha-resgate` — ambas
// sob a categoria `cat-caixinha` (type=transferencia), então o lançamento sai neutro.
func (w *CaixinhaWriter) Register(ctx context.Context, in shared.NewCaixinhaMovement) (string, error) {
	sub := "sub-caixinha-aporte"
	title := "Aporte em caixinha"
	switch {
	case in.Opening:
		// Saldo inicial: conta no guardado, neutro ao disponível (is_balance_adjustment).
		sub = "sub-caixinha-saldo-inicial"
		title = "Saldo inicial"
	case in.Rendimento:
		// Rendimento reinvestido: conta no guardado/patrimônio, neutro ao disponível.
		sub = "sub-caixinha-rendimento"
		title = "Rendimento"
	case in.Direction == "resgate":
		sub = "sub-caixinha-resgate"
		title = "Resgate de caixinha"
	}
	pay := in.Date
	cx := in.CaixinhaID
	d, err := w.create.Execute(ctx, CreateTransactionInput{
		Title:          title,
		Description:    in.Description,
		Amount:         in.Amount,
		SubcategoryID:  sub,
		PaymentMethod:  MethodOutros,
		Status:         StatusRealizado,
		CompetenceDate: in.Date,
		PaymentDate:    &pay,
		CaixinhaID:     &cx,
	})
	if err != nil {
		return "", err
	}
	return d.ID, nil
}

// RegisterAporte é um atalho para registrar um aporte (usado pela materialização da
// Receitas quando o destino aponta para uma caixinha). Satisfaz budget.CaixinhaAporter.
func (w *CaixinhaWriter) RegisterAporte(ctx context.Context, caixinhaID string, amount int64, date string, desc *string) (string, error) {
	return w.Register(ctx, shared.NewCaixinhaMovement{
		CaixinhaID: caixinhaID, Direction: "aporte", Amount: amount, Date: date, Description: desc,
	})
}

// Delete exclui o lançamento do movimento (via use case).
func (w *CaixinhaWriter) Delete(ctx context.Context, txID string) error {
	return w.delete.Execute(ctx, txID)
}
