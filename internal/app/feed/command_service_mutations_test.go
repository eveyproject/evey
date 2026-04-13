package feed_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/feed/fakes"
	"github.com/gravity-zero/evey/internal/domain/library"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureSlog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf, func() { slog.SetDefault(prev) }
}

// L24: if err := s.EnsureLibraryFeedConfig(...); err != nil { slog.Warn(...) }
// Negation -> slog.Warn triggers on success. Fail path must log, success must NOT log.
func TestCommandService_LibraryCreated_ErrorLogged(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	cmdRepo.Err = errors.New("upsert boom")
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	_ = bus.Publish(context.Background(), library.LibraryCreated{LibraryID: "lib-x", Name: "X"})

	assert.Contains(t, buf.String(), "auto-create library feed config")
	assert.Contains(t, buf.String(), "upsert boom")
}

func TestCommandService_LibraryCreated_NoErrorNoWarn(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	require.NoError(t, bus.Publish(context.Background(), library.LibraryCreated{LibraryID: "lib-ok", Name: "OK"}))

	assert.NotContains(t, buf.String(), "auto-create library feed config")
}

// L33: same for DeleteLibraryFeedConfig warn.
func TestCommandService_LibraryDeleted_ErrorLogged(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	cmdRepo.Err = errors.New("delete boom")
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	_ = bus.Publish(context.Background(), library.LibraryDeleted{LibraryID: "lib-x"})

	assert.Contains(t, buf.String(), "auto-delete library feed config")
	assert.Contains(t, buf.String(), "delete boom")
}

func TestCommandService_LibraryDeleted_NoErrorNoWarn(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	require.NoError(t, bus.Publish(context.Background(), library.LibraryDeleted{LibraryID: "lib-ok"}))

	assert.NotContains(t, buf.String(), "auto-delete library feed config")
}

// L58: if err := s.bus.Publish(...); err != nil { slog.Warn("publish feed event", ...) }
// Bus.Publish fails when PublishErr set. We target the UpdateAdminConfig path which calls s.publish.
// Need a bus that lets Subscribe work but fails on subsequent Publish. FakeEventBus.PublishErr fails
// ALL publishes, including the initial library events. For UpdateAdminConfig there's no initial
// publish, so setting PublishErr after wiring is fine.
func TestCommandService_PublishError_Logged(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	bus.PublishErr = errors.New("publish boom")

	cfg := &feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20}
	require.NoError(t, svc.UpdateAdminConfig(context.Background(), cfg))

	assert.Contains(t, buf.String(), "publish feed event")
	assert.Contains(t, buf.String(), "publish boom")
}

func TestCommandService_PublishOK_NoWarn(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	cfg := &feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20}
	require.NoError(t, svc.UpdateAdminConfig(context.Background(), cfg))

	assert.NotContains(t, buf.String(), "publish feed event")
}

// L122: if c.DisplayOrder > maxOrder { maxOrder = c.DisplayOrder }
// Boundary `>` -> `>=`. Logically equivalent: both yield the same maxOrder value.
// Kill via explicit semantics check: a config with DisplayOrder == 0 does NOT advance maxOrder.
// With `>`: [0, 2] -> maxOrder=2 (0>0 F, then 2>0 T). With `>=`: same result. Truly equivalent;
// however we pin the observable: DisplayOrder increments correctly for mixed orders.
func TestCommandService_EnsureLibraryFeedConfig_DisplayOrderFromMax(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, DisplayOrder: 7, MaxItems: 10},
		{FeedType: feed.TypeRecommended, DisplayOrder: 3, MaxItems: 10},
		{FeedType: feed.TypeContinueWatching, DisplayOrder: 5, MaxItems: 10},
	}

	require.NoError(t, svc.EnsureLibraryFeedConfig(context.Background(), "lib-9", "L"))
	created := cmdRepo.AdminConfigs[feed.LibraryType("lib-9")]
	require.NotNil(t, created)
	assert.Equal(t, 8, created.DisplayOrder)
}
