package postgres

import (
	"context"
	"log/slog"
	"time"
)

const slowQueryThreshold = 100 * time.Millisecond

// LogSlowQuery logs a warning when a query exceeds 100ms.
// Usage: defer LogSlowQuery(ctx, time.Now(), "SELECT ...")()
//
//	start := time.Now()
//	rows, err := r.db.QueryContext(ctx, query, args...)
//	LogSlowQuery(ctx, start, query)(err)
func LogSlowQuery(ctx context.Context, start time.Time, query string) func(err ...error) {
	return func(errs ...error) {
		elapsed := time.Since(start)
		if elapsed < slowQueryThreshold {
			return
		}
		q := query
		if len(q) > 200 {
			q = q[:200] + "…"
		}
		attrs := []any{
			slog.Duration("duration", elapsed),
			slog.String("query", q),
		}
		if len(errs) > 0 && errs[0] != nil {
			attrs = append(attrs, slog.Any("err", errs[0]))
		}
		slog.WarnContext(ctx, "slow query", attrs...)
	}
}
