package engine

import (
	"context"
	"fmt"
)

type Engine interface {
    Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error)
}

type engine struct {
	executor Executor
}

func New(executor Executor) Engine {
	return &engine{executor: executor}
}

func (e *engine) Execute(
	ctx context.Context,
	req ExecuteRequest,
) (*ExecuteResult, error) {
	result, err := e.executor.Run(ctx, req.Language, req.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to run executor: %w", err)
	}
	return result, nil
}