package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
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
	plugins     map[string]*plugindomain.Plugin
	updateErr   error
	updateCalls int
	// afterGet runs once the caller has its snapshot, before it writes back.
	// That interval is the whole point of the read-modify-write hazard: an
	// admin's revocation landing there is invisible to the reader.
	afterGet func()
}

func (f *fakeConfigStore) GetByID(_ context.Context, id string) (*plugindomain.Plugin, error) {
	p, ok := f.plugins[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	snapshot := clonePlugin(p)
	if f.afterGet != nil {
		hook := f.afterGet
		f.afterGet = nil // once: the admin revokes a single time
		hook()
	}
	return snapshot, nil
}

// UpdateConfig writes ONLY the config field, like the SQL it stands in for. If
// this fake stored the whole record instead, a test would pass while the real
// UPDATE replayed permissions and enabled from the caller's stale snapshot -
// which is the bug the narrow write exists to prevent.
func (f *fakeConfigStore) UpdateConfig(_ context.Context, id string, config map[string]any) error {
	f.updateCalls++
	if f.updateErr != nil {
		return f.updateErr
	}
	p, ok := f.plugins[id]
	if !ok {
		return shared.ErrNotFound
	}
	cp := clonePlugin(p)
	cp.Config = make(map[string]any, len(config))
	maps.Copy(cp.Config, config)
	f.plugins[id] = cp
	return nil
}

// clonePlugin detaches the stored record from the caller's pointer (see
// cloneProfile in internal/domain/user/fakes/profile_repo.go for the same
// rationale): SetConfig mutates the plugin it gets back from GetByID
// in-place before persisting, so without this clone a test can't tell an
// in-memory mutation from an actually-saved one - dropping the Update call
// in production would still leave GetConfig reading the mutated pointer.
func clonePlugin(p *plugindomain.Plugin) *plugindomain.Plugin {
	if p == nil {
		return nil
	}
	cp := *p
	if p.Config != nil {
		cp.Config = maps.Clone(p.Config)
	}
	return &cp
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
			Catalog: &library.MediaCatalog{
				Year:        &year,
				ProviderIDs: map[string]string{"tmdb": "123"},
				Genres:      []string{"Action"},
			},
			ParentID: "p1", Overview: "desc",
			File: &library.MediaFile{Path: "/movies/test.mkv"},
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

// --- PluginIDFromCtx ---

func TestPluginIDFromCtx_Empty(t *testing.T) {
	assert.Empty(t, PluginIDFromCtx(context.Background()))
}

func TestPluginIDFromCtx_Set(t *testing.T) {
	ctx := ctxWithPluginID("plg-x")
	assert.Equal(t, "plg-x", PluginIDFromCtx(ctx))
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
	store := &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{
		"plg-1": {ID: "plg-1", Config: map[string]any{"key1": "val1"}},
	}}
	h := NewHostServer(&fakeMediaQuerier{items: map[string]*library.MediaItem{}}, store, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "newkey", Value: "newval"})
	require.NoError(t, err)

	// The store, not just the response, must have been asked to persist:
	// GetByID/Update in the fake return/store deep copies, so a handler that
	// mutates its in-memory plugin without calling Update can't fake this.
	assert.Equal(t, 1, store.updateCalls, "SetConfig must persist through Update")
	assert.Equal(t, "newval", store.plugins["plg-1"].Config["newkey"], "the stored plugin must reflect the new key")

	resp, err := h.GetConfig(ctx, &pluginv1.GetConfigRequest{Key: "newkey"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, "newval", resp.Value)
}

// SetConfig is the ONE HostService RPC a plugin may call with no permission at
// all (methodPermissions maps it to nil). It used to persist through the blind
// full-row Update, so the row it wrote back was whatever its own GetByID had
// read - permissions, external_fetch, enabled, status and all. A plugin calling
// it in a loop only had to land one write across an admin's revocation to
// restore the permission, re-enable itself, or wipe the error state the runtime
// had recorded about its crash. external_fetch bit hardest: the broker re-reads
// it from the DB on every call, so there is no cache to invalidate.
func TestHostSetConfig_CannotReplayTheRowItRead(t *testing.T) {
	stored := &plugindomain.Plugin{
		ID:            "plg-1",
		Config:        map[string]any{"key1": "val1"},
		Permissions:   []string{"network"},
		ExternalFetch: []string{"https://api.example.com"},
		Enabled:       true,
		Status:        "running",
	}
	store := &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{"plg-1": stored}}
	h := NewHostServer(&fakeMediaQuerier{items: map[string]*library.MediaItem{}}, store, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	// The race is INSIDE the handler: it reads the row, and the admin's
	// revocation lands before it writes back. A plugin calling SetConfig in a
	// loop hits that window - it is one Postgres round-trip wide - so the hook
	// makes the timing deterministic instead of hopeful.
	store.afterGet = func() {
		store.plugins["plg-1"] = &plugindomain.Plugin{
			ID:            "plg-1",
			Config:        map[string]any{"key1": "val1"},
			Permissions:   nil,
			ExternalFetch: nil,
			Enabled:       false,
			Status:        "error",
			ErrorMessage:  "crashed",
		}
	}

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "newkey", Value: "newval"})
	require.NoError(t, err)

	got := store.plugins["plg-1"]
	assert.Equal(t, "newval", got.Config["newkey"], "the config write itself must land")
	assert.Empty(t, got.Permissions, "a revoked permission must not come back through a config write")
	assert.Empty(t, got.ExternalFetch, "a revoked fetch allow-list must not come back")
	assert.False(t, got.Enabled, "a disabled plugin must not re-enable itself")
	assert.Equal(t, plugindomain.PluginStatus("error"), got.Status, "a plugin must not erase its own crash status")
	assert.Equal(t, "crashed", got.ErrorMessage)
}

// Nothing else bounds this store: MaxRecvMsgSize caps one call, not the config
// JSON growing across thousands of them, and the RPC needs no permission.
func TestHostSetConfig_BoundsKeysAndValues(t *testing.T) {
	h := newTestHostServer()
	ctx := ctxWithPluginID("plg-2")

	_, err := h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "", Value: "v"})
	require.Error(t, err, "an empty key is not a key")

	_, err = h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: strings.Repeat("k", maxConfigKeyLen+1), Value: "v"})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())

	_, err = h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "k", Value: strings.Repeat("v", maxConfigValueLen+1)})
	require.Error(t, err)

	for i := range maxConfigKeys {
		_, err = h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: fmt.Sprintf("k%d", i), Value: "v"})
		require.NoError(t, err)
	}
	_, err = h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "one-too-many", Value: "v"})
	require.Error(t, err, "the key count must be bounded")
	st, _ = status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())

	// An existing key must still be writable at the cap - otherwise a plugin
	// that legitimately filled its store could never update anything again.
	_, err = h.SetConfig(ctx, &pluginv1.SetConfigRequest{Key: "k0", Value: "v2"})
	require.NoError(t, err)
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

func TestMediaItemToProto_Nil(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.Nil(t, mediaItemToProto(nil))
	})
}

// nilItemMediaQuerier simulates a repo returning a nil item with a nil error,
// a contract violation GetByID's error-based callers don't guard against on
// their own - GetMediaItem must not panic dereferencing the result.
type nilItemMediaQuerier struct{}

func (nilItemMediaQuerier) GetByID(_ context.Context, _ shared.MediaItemID) (*library.MediaItem, error) {
	return nil, nil
}

func (nilItemMediaQuerier) List(_ context.Context, _ library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return nil, nil
}

func (nilItemMediaQuerier) Count(_ context.Context, _ library.ListMediaItemsFilter) (int, error) {
	return 0, nil
}

func TestHostGetMediaItem_NilItemNilError_NotFoundNotPanic(t *testing.T) {
	h := NewHostServer(nilItemMediaQuerier{}, &fakeConfigStore{plugins: map[string]*plugindomain.Plugin{}}, &fakeEventBus{}, &fakePluginLogger{}, nil)
	ctx := ctxWithPluginID("plg-1")

	assert.NotPanics(t, func() {
		_, err := h.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "m1"})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

// --- pluginEmittedEvent ---

func TestPluginEmittedEvent_EventName(t *testing.T) {
	e := pluginEmittedEvent{pluginID: "p1", eventType: "scan.complete"}
	assert.Equal(t, "plugin.event.scan.complete", e.EventName())
}

// --- PluginHostSockPath ---

func TestPluginHostSockPath(t *testing.T) {
	assert.Equal(t, "/tmp/evey/plugins/plg-foo/host.sock", PluginHostSockPath("plg-foo"))
}

// --- Data store RPCs ---

type fakeDataRepo struct {
	mu sync.Mutex
	m  map[string]string // "pluginID\x00userID\x00key" -> value
}

func newFakeDataRepo() *fakeDataRepo { return &fakeDataRepo{m: map[string]string{}} }

func (f *fakeDataRepo) k(p, u, pr, key string) string {
	return p + "\x00" + u + "\x00" + pr + "\x00" + key
}

func (f *fakeDataRepo) Get(_ context.Context, p, u, pr, key string) (*plugindomain.PluginDataEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.m[f.k(p, u, pr, key)]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &plugindomain.PluginDataEntry{PluginID: p, UserID: u, ProfileID: pr, Key: key, Value: v}, nil
}

func (f *fakeDataRepo) Set(_ context.Context, p, u, pr, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[f.k(p, u, pr, key)] = value
	return nil
}

func (f *fakeDataRepo) Delete(_ context.Context, p, u, pr, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, f.k(p, u, pr, key))
	return nil
}

func (f *fakeDataRepo) List(_ context.Context, p, u, pr string) ([]*plugindomain.PluginDataEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*plugindomain.PluginDataEntry
	prefix := p + "\x00" + u + "\x00" + pr + "\x00"
	for k, v := range f.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, &plugindomain.PluginDataEntry{PluginID: p, UserID: u, ProfileID: pr, Key: k[len(prefix):], Value: v})
		}
	}
	return out, nil
}

func TestHostDataStore_RoundTrip(t *testing.T) {
	h := newTestHostServer().WithDataStore(newFakeDataRepo())
	ctx := ctxWithPluginID("plg-1")

	// absent
	got, err := h.GetData(ctx, &pluginv1.GetDataRequest{Key: "k"})
	require.NoError(t, err)
	assert.False(t, got.Found)

	_, err = h.SetData(ctx, &pluginv1.SetDataRequest{Key: "k", Value: "v"})
	require.NoError(t, err)

	got, err = h.GetData(ctx, &pluginv1.GetDataRequest{Key: "k"})
	require.NoError(t, err)
	assert.True(t, got.Found)
	assert.Equal(t, "v", got.Value)

	keys, err := h.ListDataKeys(ctx, &pluginv1.ListDataKeysRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"k"}, keys.Keys)

	_, err = h.DeleteData(ctx, &pluginv1.DeleteDataRequest{Key: "k"})
	require.NoError(t, err)
	got, _ = h.GetData(ctx, &pluginv1.GetDataRequest{Key: "k"})
	assert.False(t, got.Found)
}

func TestHostDataStore_PluginScoped(t *testing.T) {
	repo := newFakeDataRepo()
	h := newTestHostServer().WithDataStore(repo)

	_, err := h.SetData(ctxWithPluginID("plg-1"), &pluginv1.SetDataRequest{Key: "k", Value: "one"})
	require.NoError(t, err)
	_, err = h.SetData(ctxWithPluginID("plg-2"), &pluginv1.SetDataRequest{Key: "k", Value: "two"})
	require.NoError(t, err)

	// plg-2 must never see plg-1's value (id comes from the authenticated ctx).
	got, _ := h.GetData(ctxWithPluginID("plg-2"), &pluginv1.GetDataRequest{Key: "k"})
	assert.Equal(t, "two", got.Value)
	got, _ = h.GetData(ctxWithPluginID("plg-1"), &pluginv1.GetDataRequest{Key: "k"})
	assert.Equal(t, "one", got.Value)
}

// TestHostDataStore_Bounded: SetData needs no permission, so the store's
// growth is capped per value and per number of distinct keys, like SetConfig.
func TestHostDataStore_Bounded(t *testing.T) {
	h := newTestHostServer().WithDataStore(newFakeDataRepo())
	ctx := ctxWithPluginID("plg-1")
	code := func(err error) codes.Code { st, _ := status.FromError(err); return st.Code() }

	_, err := h.SetData(ctx, &pluginv1.SetDataRequest{Key: strings.Repeat("k", maxDataKeyLen+1), Value: "v"})
	assert.Equal(t, codes.InvalidArgument, code(err), "oversized key")
	_, err = h.SetData(ctx, &pluginv1.SetDataRequest{Key: "big", Value: strings.Repeat("v", maxDataValueLen+1)})
	assert.Equal(t, codes.InvalidArgument, code(err), "oversized value")

	for i := range maxDataKeys {
		_, err = h.SetData(ctx, &pluginv1.SetDataRequest{Key: fmt.Sprintf("k%d", i), Value: "v"})
		require.NoError(t, err)
	}
	_, err = h.SetData(ctx, &pluginv1.SetDataRequest{Key: "one-too-many", Value: "v"})
	assert.Equal(t, codes.ResourceExhausted, code(err), "key count cap")
	_, err = h.SetData(ctx, &pluginv1.SetDataRequest{Key: "k0", Value: "rewritten"})
	require.NoError(t, err, "an existing key stays writable at the cap")
	_, err = h.SetData(ctxWithPluginID("plg-2"), &pluginv1.SetDataRequest{Key: "k", Value: "v"})
	require.NoError(t, err, "the cap is per plugin")
}

func TestHostDataStore_Unimplemented(t *testing.T) {
	h := newTestHostServer() // no data store wired
	_, err := h.GetData(ctxWithPluginID("plg-1"), &pluginv1.GetDataRequest{Key: "k"})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

// --- Panic recovery ---

// panickyMediaQuerier simulates a handler dependency blowing up mid-call -
// the kind of bug that, on the plugin host's socket, would otherwise take
// down the whole Evey process since it's the only gRPC surface reachable
// by third-party (plugin) code.
type panickyMediaQuerier struct{}

func (panickyMediaQuerier) GetByID(_ context.Context, _ shared.MediaItemID) (*library.MediaItem, error) {
	panic("boom")
}

func (panickyMediaQuerier) List(_ context.Context, _ library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return nil, nil
}

func (panickyMediaQuerier) Count(_ context.Context, _ library.ListMediaItemsFilter) (int, error) {
	return 0, nil
}

// TestHostServer_PanicRecovered_ServerSurvives drives a real HostService RPC
// over the per-plugin Unix socket (auth + ratelimit + handler chain, exactly
// as a plugin process would) into a handler whose dependency panics. It
// proves RecoveryUnary turns that into codes.Internal instead of crashing
// the listener - and that a follow-up RPC on the same server still works.
func TestHostServer_PanicRecovered_ServerSurvives(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "plugins"), 0o755))

	h := NewHostServer(
		panickyMediaQuerier{},
		&fakeConfigStore{plugins: map[string]*plugindomain.Plugin{"plg-panic": {ID: "plg-panic"}}},
		&fakeEventBus{},
		&fakePluginLogger{},
		nil,
	)
	t.Cleanup(h.Stop)

	sockPath, err := h.RegisterPlugin("plg-panic")
	require.NoError(t, err)
	defer h.UnregisterPlugin("plg-panic")
	h.SetExpectedPID("plg-panic", int32(os.Getpid()))

	_, client := dialPluginSock(t, sockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.GetMediaItem(ctx, &pluginv1.GetMediaItemRequest{Id: "m1"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())

	// The server (and its process) must survive: a follow-up RPC on the same
	// gRPC server still gets a normal response rather than a broken pipe.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_, err = client.GetConfig(ctx2, &pluginv1.GetConfigRequest{Key: "k"})
	require.NoError(t, err)
}
