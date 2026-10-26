package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// Consume moves an authorized request to consumed in one conditional write:
// of two concurrent callers exactly one wins, and a pending or already
// consumed request is refused.
func TestQuickConnectRepo_ConsumeIsOneShot(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	users := postgres.NewUserRepository(db)
	repo := postgres.NewQuickConnectRepository(db)
	ctx := context.Background()

	require.NoError(t, users.Create(ctx, newTestUser("qc-consume-u")))
	require.NoError(t, repo.Create(ctx, &user.QuickConnectRequest{
		ID: "qc-c1", Code: "CONSUME1", Secret: "s", Status: user.Pending,
		ExpiresAt: time.Now().Add(10 * time.Minute), CreatedAt: time.Now(),
	}))
	require.ErrorIs(t, repo.Consume(ctx, "CONSUME1"), shared.ErrNotFound, "a pending request is not redeemable")

	uid := "qc-consume-u"
	require.NoError(t, repo.UpdateStatus(ctx, "qc-c1", string(user.Authorized), &uid))

	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if repo.Consume(ctx, "CONSUME1") == nil {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load())

	got, err := repo.GetByCode(ctx, "CONSUME1")
	require.NoError(t, err)
	assert.Equal(t, user.Consumed, got.Status)
	require.ErrorIs(t, repo.Consume(ctx, "CONSUME1"), shared.ErrNotFound)
}
