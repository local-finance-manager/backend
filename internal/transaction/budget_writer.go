package transaction

import (
	"context"

	"github.com/local-finance-manager/backend/internal/shared"
)

// BudgetWriter satisfaz budget.TransactionWriter (structural typing): cria e exclui
// lançamentos a partir do plano de alocação, passando pelos use cases (que derivam o
// tipo da subcategoria, validam e aplicam o guard de mês fechado do report). Definido
// aqui (produtor); injetado no main.go.
type BudgetWriter struct {
	create CreateTransactionUseCase
	delete DeleteTransactionUseCase
}

// NewBudgetWriter cria o writer reusando os use cases de criação/exclusão.
func NewBudgetWriter(create CreateTransactionUseCase, del DeleteTransactionUseCase) *BudgetWriter {
	return &BudgetWriter{create: create, delete: del}
}

// Create cria um lançamento a partir do pedido neutro e devolve o id criado.
func (w *BudgetWriter) Create(ctx context.Context, in shared.NewTransaction) (string, error) {
	pm := in.PaymentMethod
	if pm == "" {
		pm = string(MethodOutros) // destino de alocação sem forma de pagamento → neutro
	}
	d, err := w.create.Execute(ctx, CreateTransactionInput{
		Title:          in.Title,
		Description:    in.Description,
		Amount:         in.Amount,
		SubcategoryID:  in.SubcategoryID,
		PaymentMethod:  PaymentMethod(pm),
		Status:         TransactionStatus(in.Status),
		CompetenceDate: in.CompetenceDate,
		PaymentDate:    in.PaymentDate,
	})
	if err != nil {
		return "", err
	}
	return d.ID, nil
}

// Delete exclui o lançamento (respeita o guard de mês fechado via use case).
func (w *BudgetWriter) Delete(ctx context.Context, id string) error {
	return w.delete.Execute(ctx, id)
}
