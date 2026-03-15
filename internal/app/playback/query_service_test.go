package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	pbfakes "github.com/gravity-zero/evey/internal/domain/playback/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryService_NilSessions_ReturnsNil(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	q := appplayback.NewQueryService(pr)

	sessions, err := q.GetActiveSessions(context.Background())
	require.NoError(t, err)
	assert.Nil(t, sessions)
}

func TestQueryService_GetNextUp(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	q := appplayback.NewQueryService(pr)
	ctx := context.Background()

	items, err := q.GetNextUp(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Empty(t, items)
}
