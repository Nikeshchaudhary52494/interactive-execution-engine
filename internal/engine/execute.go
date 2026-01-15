package engine

import "context"

type Executor interface {
	Run(
		ctx context.Context,
		lang string,
		code string,
	) (*ExecuteResult, error)
}