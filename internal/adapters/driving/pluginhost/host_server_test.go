package pluginhost

import (
	"context"
	"errors"
	"testing"

	pluginv1 "github.com/gravity-zero/evey/gen/plugin/v1"
	"github.com/gravity-zero/evey/internal/domain/library"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeMediaQuerier struct {
	items   map[string]*library.MediaItem
	listErr error
	cntErr  error
}

func (f *fakeMediaQuerier) GetByID(_ context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	item, ok := f.items[string(id)]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return item, nil
}

func (f *fakeMediaQuerier) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
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
	if f.cntErr != nil {
		return 0, f.cntErr
	}
	items, err := f.List(context.Background(), filter)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

type fakeConfigStore struct {
	plugins   map[string]*plugindomain.Plugin
	updateErr error
}

func (f *fakeConfigStore) GetByID(_ context.Context, id string) (*plugindomain.Plugin, error) {
	p, ok := f.plugins[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return p, nil
}

func (f *fakeConfigStore) Update(_ context.Context, p *plugindomain.Plugin) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.plugins[p.ID] = p
	return nil
}

type fakeEventBus struct {
	published  []shared.Event
	publishErr error
}

func (b *fakeEventBus) Publish(_ context.Context, event shared.Event) error {
	if b.publishErr != nil {
		return b.publishErr
	}
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

func (l *fakePluginLogger) WriteLog(_, line string) {
	l.entries = append(l.entries, line)
}

func newTestHostServer() *HostServer {
	year := 2024
	media := &fakeMediaQuerier{items: map[string]*library.MediaItem{
		"m1": {
			ID: "m1", LibraryID: "lib1", Title: "Test Movie", Type: "movie",
			Year: &year, ProviderIDs: map[string]string{"tmdb": "123"},
			ParentID: "p1", Overview: "desc", FilePath: "/movies/test.mkv",
			Genres: []string{"Action"},
		},
	}}
	plugins := &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{
		"plg-1": {ID: "plg-1", Config: map[string]any{"key1": "val1"}},
		"plg-2": {ID: "plg-2", Config: nil},
	}}
	bus := &fakeEventBus{}
	logger := &fakePluginLogger{}
	return NewHostServer(media, plugins, bus, logger, nil)
}

func ctxWithPluginID(pluginID string) context.Context {
	return context.WithValue(context.Background(), pluginIDKey{}, pluginID)
}

// --- pluginIDInterceptor ---

func TestPluginIDInterceptor_Valid(t *testing.T) {
	h := newTestHostServer()
	interceptor := h.pluginIDInterceptor()

	md := metadata.Pairs("x-plugin-id", "plg-1")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedCtx context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		capturedCtx = ctx
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, "plg-1", pluginIDFromCtx(capturedCtx))
}

func TestPluginIDInterceptor_MissingMetadata(t *testing.T) {
	h := newTestHostServer()
	interceptor := h.pluginIDInterceptor()

	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "missing metadata")
}

func TestPluginIDInterceptor_EmptyPluginID(t *testing.T) {
	h := newTestHostServer()
	interceptor := h.pluginIDInterceptor()

	md := metadata.Pairs("x-plugin-id", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "missing x-plugin-id")
}

func TestPluginIDInterceptor_NoPluginIDHeader(t *testing.T) {
	h := newTestHostServer()
	interceptor := h.pluginIDInterceptor()

	md := metadata.Pairs("x-other", "val")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// --- pluginIDFromCtx ---

func TestPluginIDFromCtx_Empty(t *testing.T) {
	assert.Empty(t, pluginIDFromCtx(context.Background()))
}

// --- GetMediaItem ---

func TestHostGetMediaItem(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "m1"})
	require.NoError(t, err)
	assert.Equal(t, "m1", resp.Id)
	assert.Equal(t, "Test Movie", resp.Title)
	assert.Equal(t, int32(2024), resp.Year)
	assert.Equal(t, map[string]string{"tmdb": "123"}, resp.ProviderIds)
	assert.Equal(t, "p1", resp.ParentId)
	assert.Equal(t, "desc", resp.Overview)
	assert.Equal(t, "/movies/test.mkv", resp.FilePath)
	assert.Equal(t, []string{"Action"}, resp.Genres)
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

func TestHostGetMediaItem_EmptyID(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: ""})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// --- ListMediaItems ---

func TestHostListMediaItems(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{LibraryId: "lib1"})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, int32(1), resp.Total)
}

func TestHostListMediaItems_WithFilters(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{
		Type:     "movie",
		ParentId: "p1",
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHostListMediaItems_NoMatch(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{LibraryId: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
	assert.Equal(t, int32(0), resp.Total)
}

func TestHostListMediaItems_ListError(t *testing.T) {
	media := &fakeMediaQuerier{
		items:   map[string]*library.MediaItem{},
		listErr: errors.New("db error"),
	}
	h := NewHostServer(media, &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{}}, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	_, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestHostListMediaItems_CountError(t *testing.T) {
	media := &fakeMediaQuerier{
		items:  map[string]*library.MediaItem{},
		cntErr: errors.New("count error"),
	}
	h := NewHostServer(media, &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{}}, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	_, err := h.ListMediaItems(ctx, &pluginv1.ListMediaItemsRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// --- GetUserPreferences ---

func TestHostGetUserPreferencesUnimplemented(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.GetUserPreferences(ctx, &pluginv1.GetUserPreferencesRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

// --- UpdateMetadata ---

func TestHostUpdateMetadataUnimplemented(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")

	_, err := h.UpdateMetadata(ctx, &pluginv1.UpdateMetadataRequest{})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

// --- EmitEvent ---

func TestHostEmitEvent(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	bus := h.events.(*fakeEventBus)

	_, err := h.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventType: "custom.event"})
	require.NoError(t, err)
	require.Len(t, bus.published, 1)
	assert.Equal(t, "plugin.event.custom.event", bus.published[0].EventName())
}

func TestHostEmitEvent_WithPayload(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	bus := h.events.(*fakeEventBus)

	payload, err := structpb.NewStruct(map[string]any{"key": "value"})
	require.NoError(t, err)

	_, err = h.EmitEvent(ctx, &pluginv1.EmitEventRequest{
		EventType: "test.payload",
		Payload:   payload,
	})
	require.NoError(t, err)
	require.Len(t, bus.published, 1)
}

func TestHostEmitEvent_PublishError(t *testing.T) {
	bus := &fakeEventBus{publishErr: errors.New("bus down")}
	h := NewHostServer(&fakeMediaQuerier{items: map[string]*library.MediaItem{}}, &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{}}, bus, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	_, err := h.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventType: "fail"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// --- Log ---

func TestHostLog(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	logger := h.logger.(*fakePluginLogger)

	_, err := h.Log(ctx, &pluginv1.LogRequest{
		Level:   pluginv1.LogLevel_LOG_LEVEL_INFO,
		Message: "test log message",
	})
	require.NoError(t, err)
	require.Len(t, logger.entries, 1)
	assert.Contains(t, logger.entries[0], "test log message")
}

func TestHostLog_WithFields(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	logger := h.logger.(*fakePluginLogger)

	_, err := h.Log(ctx, &pluginv1.LogRequest{
		Level:   pluginv1.LogLevel_LOG_LEVEL_ERROR,
		Message: "err happened",
		Fields:  map[string]string{"code": "500", "detail": "timeout"},
	})
	require.NoError(t, err)
	require.Len(t, logger.entries, 1)
	assert.Contains(t, logger.entries[0], "[ERROR]")
	assert.Contains(t, logger.entries[0], "err happened")
	assert.Contains(t, logger.entries[0], "code=500")
}

func TestHostLog_UnspecifiedLevel(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-1")
	logger := h.logger.(*fakePluginLogger)

	_, err := h.Log(ctx, &pluginv1.LogRequest{
		Level:   pluginv1.LogLevel_LOG_LEVEL_UNSPECIFIED,
		Message: "default level",
	})
	require.NoError(t, err)
	require.Len(t, logger.entries, 1)
	assert.Contains(t, logger.entries[0], "[INFO]")
}

func TestHostLog_NilLogger(t *testing.T) {
	h := NewHostServer(&fakeMediaQuerier{items: map[string]*library.MediaItem{}}, &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{}}, &fakeEventBus{}, nil, nil)
	ctx := ctxWithPluginID("plg-1")

	resp, err := h.Log(ctx, &pluginv1.LogRequest{
		Level:   pluginv1.LogLevel_LOG_LEVEL_INFO,
		Message: "no logger",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// --- GetConfig ---

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

func TestHostGetConfig_PluginNotFound(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("unknown-plugin")

	_, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "key1"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

// --- SetConfig ---

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

func TestHostSetConfig_NilConfigMap(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-2")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "k", Value: "v"})
	require.NoError(t, err)

	resp, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "k"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, "v", resp.Value)
}

func TestHostSetConfig_PluginNotFound(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("unknown-plugin")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "k", Value: "v"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestHostSetConfig_UpdateError(t *testing.T) {
	store := &fakeConfigStore{
		plugins:   map[string]*plugindomain.Plugin{"plg-1": {ID: "plg-1", Config: map[string]any{}}},
		updateErr: errors.New("write failed"),
	}
	h := NewHostServer(&fakeMediaQuerier{items: map[string]*library.MediaItem{}}, store, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "k", Value: "v"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// --- mediaItemToProto ---

func TestMediaItemToProto_NoYear(t *testing.T) {
	item := &library.MediaItem{
		ID: "m2", LibraryID: "lib1", Title: "No Year",
	}
	pb := mediaItemToProto(item)
	assert.Equal(t, int32(0), pb.Year)
	assert.Nil(t, pb.ProviderIds)
}

// --- pluginEmittedEvent ---

func TestPluginEmittedEvent_EventName(t *testing.T) {
	e := pluginEmittedEvent{pluginID: "p1", eventType: "scan.complete"}
	assert.Equal(t, "plugin.event.scan.complete", e.EventName())
}
