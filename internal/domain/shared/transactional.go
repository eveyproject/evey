package shared

import "context"

// TransactionalUnit executes fn atomically. Adapters may propagate a
// transaction through ctx; repos should honor it transparently.
type TransactionalUnit interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
