package transaction

import "context"

// GetTransactionUseCase retrieves a single transaction by ID.
type GetTransactionUseCase interface {
	Execute(ctx context.Context, id string) (TransactionDetail, error)
}

// ListTransactionsUseCase returns a paginated list plus financial summary.
type ListTransactionsUseCase interface {
	Execute(ctx context.Context, in ListTransactionsInput) (ListTransactionsResult, error)
}

// CreateTransactionUseCase creates a new transaction.
type CreateTransactionUseCase interface {
	Execute(ctx context.Context, in CreateTransactionInput) (TransactionDetail, error)
}

// UpdateTransactionUseCase replaces all mutable fields of an existing transaction (PUT semantics).
type UpdateTransactionUseCase interface {
	Execute(ctx context.Context, in UpdateTransactionInput) (TransactionDetail, error)
}

// ConfirmTransactionUseCase transitions a pendente transaction to realizado.
type ConfirmTransactionUseCase interface {
	Execute(ctx context.Context, in ConfirmTransactionInput) (TransactionDetail, error)
}

// CancelTransactionUseCase transitions a transaction to cancelado (no body needed).
type CancelTransactionUseCase interface {
	Execute(ctx context.Context, id string) (TransactionDetail, error)
}

// DeleteTransactionUseCase removes a transaction permanently.
type DeleteTransactionUseCase interface {
	Execute(ctx context.Context, id string) error
}
