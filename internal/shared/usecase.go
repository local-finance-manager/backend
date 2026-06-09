package shared

import "context"

// UseCase is a generic use case with one input and one output.
type UseCase[In, Out any] interface {
	Execute(ctx context.Context, in In) (Out, error)
}

// VoidUseCase is a use case that returns only an error (e.g., delete).
type VoidUseCase[In any] interface {
	Execute(ctx context.Context, in In) error
}

// QueryUseCase is a use case that takes no input (e.g., list all).
type QueryUseCase[Out any] interface {
	Execute(ctx context.Context) (Out, error)
}
