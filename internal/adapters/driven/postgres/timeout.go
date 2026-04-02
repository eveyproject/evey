package postgres

import (
	"context"
	"time"
)

const defaultQueryTimeout = 30 * time.Second

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
