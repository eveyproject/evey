package feed_test

import (
	"context"
	"testing"
	"time"

	appfeed "github.com/eveyproject/evey/internal/app/feed"
	libevent "github.com/eveyproject/evey/internal/domain/library"
	notiffakes "github.com/eveyproject/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CacheWithEventBus subscribes to library events; each fires invalidation.
func TestCachedAnalyzer_CacheWithEventBus_InvalidatesOnMediaItemAdded(t *testing.T) {
	t.Parallel()
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action"}
	bus := sharedfakes.NewFakeEventBus()
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute, appfeed.CacheWithEventBus(bus))

	ctx := context.Background()
	// prime cache
	g1, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"action"}, g1)

	// fire media.item.added → cache invalidates
	fake.Genres = []string{"drama"}
	require.NoError(t, bus.Publish(ctx, libevent.MediaItemAdded{
		MediaItemID: "mi-1", LibraryID: "lib-1", Title: "t", Type: "movie", OccurredAt: time.Now(),
	}))

	g2, err := cached.GetFavoriteGenres(ctx, "p1", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"drama"}, g2, "cache should have been invalidated by the event")
}

// Nil bus option has no effect (defense test).
func TestCachedAnalyzer_CacheWithEventBus_OptionIsIdempotent(t *testing.T) {
	t.Parallel()
	fake := notiffakes.NewFakeWatchHistoryAnalyzer()
	fake.Genres = []string{"action"}
	bus := sharedfakes.NewFakeEventBus()
	// Applying the option twice should not panic.
	cached := appfeed.NewCachedAnalyzer(fake, 5*time.Minute,
		appfeed.CacheWithEventBus(bus),
		appfeed.CacheWithEventBus(bus),
	)
	_, err := cached.GetFavoriteGenres(context.Background(), "p1", 3)
	require.NoError(t, err)
}
