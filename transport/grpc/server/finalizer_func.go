package server

import (
	"context"
)

type FinalizerFunc func(ctx context.Context, err error)
