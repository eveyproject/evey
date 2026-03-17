package pluginhost

import (
	"context"
	"testing"

	pluginv1 "github.com/gravity-zero/evey/gen/plugin/v1"
	"github.com/gravity-zero/evey/internal/domain/library"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeMediaQuerier struct {
	items map[string]*library.MediaItem
}

func (f *fakeMediaQuerier) GetByID(_ context.Context, id string) (*library.MediaItem, error) {
	item, ok := f.items[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return item, nil
}

func (f *fakeMediaQuerier) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	var result []*library.MediaItem
	for _, item := range f.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (f *fakeMediaQuerier) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	items, err := f.List(context.Background(), filter)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

type fakeConfigStore struct {
	plugins map[string]*plugindomain.Plugin
}

func (f *fakeConfigStore) GetByID(_ context.Context, id string) (*plugindomain.Plugin, error) {
	p, ok := f.plugins[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return p, nil
}

func (f *fakeConfigStore) Update(_ context.Context, p *plugindomain.Plugin) error {
	f.plugins[p.ID] = p
	return nil
}

type fakeEventBus struct {
	published []shared.Event
}

func (b *fakeEventBus) Publish(_ context.Context, event shared.Event) error {
	b.published = append(b.published, event)
	return nil
}

func (b *fakeEventBus) Subscribe(_ string, _ func(ctx context.Context, event shared.Event)) string {
	return ""
}

func (b *fakeEventBus) Unsubscribe(_ string) {}

type fakePluginLogger struct {
	entries []string
}

func (l *fakePluginLogger) WriteLog(_, _, msg string, _ map[string]string) {
	l.entries = append(l.entries, msg)
}

func newTestHostServer() *HostServer {
	year := 2024
	media := &fakeMediaQuerier{items: map[string]*library.MediaItem{
		"m1": {ID: "m1", LibraryID: "lib1", Title: "Test Movie", Type: "movie", Year: &year},
	}}
	plugins := &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{
		"plg-1": {ID: "plg-1", Config: map[string]any{"key1": "val1"}},
	}}
	bus := &fakeEventBus{}
	logger := &fakePluginLogger{}
	return NewHostServer(media, plugins, bus, logger, nil)
}

func ctxWithPluginID(pluginID string) context.Context {
	return context.WithValue(context.Background(), pluginIDKey{}, pluginID)
}

func TestHostGetMediaItem(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "m1"})
	require.NoError(t, err)
	assert.Equal(t, "m1", resp.Id)
	assert.Equal(t, "Test Movie", resp.Title)
	assert.Equal(t, int32(2024), resp.Year)
}

func TestHostGetMediaItemNotFound(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "nonexistent"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestHostListMediaItems(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{LibraryId: "lib1"})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, int32(1), resp.Total)
}

func TestHostGetConfig(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "key1"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, "val1", resp.Value)
}

func TestHostGetConfigNotFound(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "nonexistent"})
	require.NoError(t, err)
	assert.False(t, resp.Found)
}

func TestHostSetConfig(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "newkey", Value: "newval"})
	require.NoError(t, err)

	resp, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "newkey"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, "newval", resp.Value)
}

func TestHostLog(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	logger := h.logger.(*fakePluginLogger)

	_, err := h.Log(ctx, &pluginv1.LogRequest{
		Level:   pluginv1.LogLevel_LOG_LEVEL_INFO,
		Message: "test log message",
	})
	require.NoError(t, err)
	assert.Contains(t, logger.entries, "test log message")
}

func TestHostEmitEvent(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	bus := h.events.(*fakeEventBus)

	_, err := h.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventType: "custom.event"})
	require.NoError(t, err)
	require.Len(t, bus.published, 1)
	assert.Equal(t, "plugin.event.custom.event", bus.published[0].EventName())
}

func TestHostUpdateMetadataUnimplemented(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.UpdateMetadata(ctx, &pluginv1.UpdateMetadataRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}
