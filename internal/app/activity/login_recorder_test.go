package activity_test

import (
	"context"
	"testing"
	"time"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The login events feed the login history: a success is a row for the user,
// a refused password a failure row with its reason, a username that is no
// account nothing at all.
func TestLoginRecorder_WritesTheHistoryFromTheLoginEvents(t *testing.T) {
	t.Parallel()
	cmdSvc, querySvc, _, _, _, _, _ := newTestServices()
	ctx := context.Background()
	users := userfakes.NewFakeUserRepository()
	require.NoError(t, users.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true}))
	bus := sharedfakes.NewFakeEventBus()
	appactivity.NewLoginRecorder(cmdSvc, users).Register(bus)

	require.NoError(t, bus.Publish(ctx, user.LoginSucceeded{UserID: "u1", ClientIP: "10.0.0.2", AuthMethod: "local", OccurredAt: time.Now()}))
	require.NoError(t, bus.Publish(ctx, user.LoginFailed{Username: "alice", Reason: "bad_password", AuthMethod: "local", OccurredAt: time.Now()}))
	require.NoError(t, bus.Publish(ctx, user.LoginFailed{Username: "nobody", Reason: "unknown_user", AuthMethod: "local", OccurredAt: time.Now()}))

	entries, err := querySvc.GetLoginHistory(ctx, "u1", 10, 0)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	var successes, failures int
	for _, e := range entries {
		if e.Success {
			successes++
			assert.Equal(t, "10.0.0.2", e.ClientIP)
			assert.Equal(t, "local", e.AuthMethod)
		} else {
			failures++
			require.NotNil(t, e.FailureReason)
			assert.Equal(t, "bad_password", *e.FailureReason)
		}
		assert.False(t, e.CreatedAt.IsZero())
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
}
