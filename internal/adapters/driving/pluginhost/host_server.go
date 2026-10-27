package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	pluginv1 "github.com/eveyproject/evey/gen/plugin/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/library"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	pluginSockBaseDir = "/tmp/evey/plugins"
	pluginUID         = 1001
	pluginGID         = 1001
)

// PluginHostSockPath returns the per-plugin host socket path. Deterministic
// from pluginID so the runner can set EVEY_HOST_SOCK before the plugin
// process starts.
func PluginHostSockPath(pluginID string) string {
	return filepath.Join(pluginSockBaseDir, pluginID, "host.sock")
}

type MediaItemQuerier interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error)
	List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error)
	Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error)
}

type PluginConfigStore interface {
	GetByID(ctx context.Context, id string) (*plugindomain.Plugin, error)
	// UpdateConfig, not Update: the writer on this surface is the PLUGIN, so it
	// must never be able to write back a whole row it read earlier. See the
	// domain port's doc.
	UpdateConfig(ctx context.Context, id string, config map[string]any) error
}

// HostServer holds the shared dependencies for HostService RPCs and a
// registry of per-plugin gRPC listeners. Each registered plugin gets its
// own Unix socket, gRPC server, and authentication interceptor that pins
// the connection to the expected process PID via SO_PEERCRED.
type HostServer struct {
	pluginv1.UnimplementedHostServiceServer
	mediaItems      MediaItemQuerier
	plugins         PluginConfigStore
	events          shared.EventBus
	logger          plugindomain.PluginLogger
	permInterceptor grpc.UnaryServerInterceptor
	rpcLimiter      *ratelimit.Limiter                // per-plugin HostService RPC rate limit (anti-flood)
	data            plugindomain.PluginDataRepository // plugin-global key/value store; nil disables the data RPCs
	activity        func(pluginID string)             // interaction signal for the idle manager; nil = disabled
	pluginTexts     library.PluginTextRepository      // metadata plugins' texts; nil disables UpdateMetadata

	mu       sync.RWMutex
	registry map[string]*pluginEntry
}

// pluginRPCRatePerMin caps each plugin's HostService RPC rate. Lenient on
// purpose: a legitimate plugin makes occasional calls, so this only trips on a
// flood (a misbehaving plugin spamming RPCs to load the server).
const pluginRPCRatePerMin = 600

type pluginEntry struct {
	pluginID    string
	expectedPID int32
	sockPath    string
	grpcSrv     *grpc.Server
	listener    net.Listener
}

func NewHostServer(
	mediaItems MediaItemQuerier,
	plugins PluginConfigStore,
	events shared.EventBus,
	logger plugindomain.PluginLogger,
	permInterceptor grpc.UnaryServerInterceptor,
) *HostServer {
	return &HostServer{
		mediaItems:      mediaItems,
		plugins:         plugins,
		events:          events,
		logger:          logger,
		permInterceptor: permInterceptor,
		rpcLimiter:      ratelimit.New(pluginRPCRatePerMin),
		registry:        make(map[string]*pluginEntry),
	}
}

// WithDataStore wires the plugin data-store repository, enabling the GetData/
// SetData/DeleteData/ListDataKeys RPCs. Without it those RPCs return
// Unimplemented.
func (h *HostServer) WithDataStore(repo plugindomain.PluginDataRepository) *HostServer {
	h.data = repo
	return h
}

// WithPluginTexts wires the metadata plugins' text store, enabling
// UpdateMetadata. Without it that RPC returns Unimplemented.
func (h *HostServer) WithPluginTexts(store library.PluginTextRepository) *HostServer {
	h.pluginTexts = store
	return h
}

// WithActivityFunc wires an interaction callback invoked on every accepted
// HostService RPC, keyed by plugin ID. The low-memory profile's idle manager
// uses it to know a plugin is alive and working; nil disables the signal.
func (h *HostServer) WithActivityFunc(fn func(pluginID string)) *HostServer {
	h.activity = fn
	return h
}

// makeRateLimitInterceptor caps a plugin's HostService RPC rate (keyed by
// plugin ID). On overrun it returns ResourceExhausted rather than letting a
// flood load the server.
func (h *HostServer) makeRateLimitInterceptor(entry *pluginEntry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if h.rpcLimiter != nil && !h.rpcLimiter.Allow(entry.pluginID) {
			return nil, status.Error(codes.ResourceExhausted, "plugin RPC rate limit exceeded")
		}
		if h.activity != nil {
			h.activity(entry.pluginID)
		}
		return handler(ctx, req)
	}
}

// Start prepares the per-plugin socket base directory and removes any
// stale socket left behind by a crashed previous run. No long-running
// listener is created here - listeners are spun up by RegisterPlugin.
func (h *HostServer) Start() error {
	if err := os.MkdirAll(pluginSockBaseDir, 0o755); err != nil {
		return fmt.Errorf("create sock base dir: %w", err)
	}
	// Sweep legacy single host sock from before per-plugin sockets shipped.
	_ = os.Remove(filepath.Join(pluginSockBaseDir, "evey-host.sock"))
	return nil
}

// Stop tears down every per-plugin listener.
func (h *HostServer) Stop() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.registry))
	for id := range h.registry {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		h.UnregisterPlugin(id)
	}
}

// RegisterPlugin creates the plugin-scoped Unix socket and starts a gRPC
// server bound to it. The returned path is what should be set on the
// plugin's EVEY_HOST_SOCK env var. Authentication is not yet armed - the
// runner MUST call SetExpectedPID after cmd.Start to bind the listener
// to the actual plugin PID. Until then any RPC is rejected.
func (h *HostServer) RegisterPlugin(pluginID string) (string, error) {
	if pluginID == "" {
		return "", fmt.Errorf("RegisterPlugin: pluginID required")
	}

	sockPath := PluginHostSockPath(pluginID)
	sockDir := filepath.Dir(sockPath)

	h.mu.Lock()
	_, exists := h.registry[pluginID]
	h.mu.Unlock()
	if exists {
		// Self-heal a stale orphan: a prior stop/crash/restart that left the
		// entry behind would otherwise lock the plugin out of re-enabling until
		// a full server restart. Tear the old registration down and re-create
		// it fresh. UnregisterPlugin takes h.mu itself, so call it unlocked.
		slog.Warn("plugin host socket already registered; evicting stale entry to re-register", slog.String("plugin", pluginID))
		h.UnregisterPlugin(pluginID)
	}

	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		return "", fmt.Errorf("create sock dir: %w", err)
	}
	if os.Getuid() == 0 {
		_ = os.Chown(sockDir, pluginUID, pluginGID)
	}
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove old host sock", slog.String("plugin", pluginID), slog.Any("err", err))
	}

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("listen host sock: %w", err)
	}
	if chErr := os.Chmod(sockPath, 0o660); chErr != nil {
		slog.Warn("chmod host sock", slog.Any("err", chErr))
	}
	if os.Getuid() == 0 {
		if chErr := os.Chown(sockPath, pluginUID, pluginGID); chErr != nil {
			slog.Warn("chown host sock", slog.Any("err", chErr))
		}
	}

	entry := &pluginEntry{
		pluginID: pluginID,
		sockPath: sockPath,
		listener: lis,
	}

	// RecoveryUnary goes first so it also catches a panic inside auth/ratelimit,
	// not just the RPC handler - this is the only gRPC surface reachable by
	// third-party plugin code, so an unrecovered panic here would crash the
	// whole Evey process.
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		interceptors.RecoveryUnary(),
		h.makePluginAuthInterceptor(entry),
		h.makeRateLimitInterceptor(entry),
	}
	if h.permInterceptor != nil {
		unaryInterceptors = append(unaryInterceptors, h.permInterceptor)
	}

	srv := grpc.NewServer(
		grpc.Creds(NewPeerCredsTransport()),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		// HostService messages are small; don't rely on grpc's 4 MB default.
		grpc.MaxRecvMsgSize(1<<20),
	)
	pluginv1.RegisterHostServiceServer(srv, h)
	entry.grpcSrv = srv

	h.mu.Lock()
	h.registry[pluginID] = entry
	h.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Long-lived per-plugin gRPC server goroutine, not tied to any
				// single request - no ctx to carry.
				slog.Error("panic in host gRPC serve",
					slog.String("plugin", pluginID), slog.Any("recover", r))
			}
		}()
		if err := srv.Serve(lis); err != nil {
			slog.Warn("host gRPC serve ended",
				slog.String("plugin", pluginID), slog.Any("err", err))
		}
	}()

	return sockPath, nil
}

// SetExpectedPID arms the per-plugin auth interceptor with the OS PID of
// the just-started plugin process. Until this is set the interceptor
// rejects every RPC so a malicious peer that wins the race against
// cmd.Start cannot impersonate the plugin.
func (h *HostServer) SetExpectedPID(pluginID string, pid int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if entry, ok := h.registry[pluginID]; ok {
		entry.expectedPID = pid
	}
}

// UnregisterPlugin tears down the listener, the gRPC server and removes
// the socket file for a single plugin.
func (h *HostServer) UnregisterPlugin(pluginID string) {
	h.mu.Lock()
	entry, ok := h.registry[pluginID]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.registry, pluginID)
	h.mu.Unlock()

	if entry.grpcSrv != nil {
		entry.grpcSrv.GracefulStop()
	}
	if err := os.Remove(entry.sockPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("remove host sock on unregister",
			slog.String("plugin", pluginID), slog.Any("err", err))
	}
}

// makePluginAuthInterceptor returns an interceptor that verifies every
// incoming RPC came from the process whose PID matches the registered
// plugin. The pluginID is captured by the closure (never trusted from
// the wire), and the PID comes from the kernel via SO_PEERCRED.
func (h *HostServer) makePluginAuthInterceptor(entry *pluginEntry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		p, ok := peer.FromContext(ctx)
		if !ok || p.AuthInfo == nil {
			return nil, status.Error(codes.Unauthenticated, "no peer credentials")
		}
		creds, ok := p.AuthInfo.(PeerCredsAuthInfo)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing SO_PEERCRED auth info")
		}

		h.mu.RLock()
		expected := entry.expectedPID
		h.mu.RUnlock()

		if expected == 0 {
			return nil, status.Error(codes.Unauthenticated, "plugin PID not bound yet")
		}
		if creds.PID != expected {
			slog.WarnContext(ctx, "plugin auth: PID mismatch - possible spoofing attempt",
				slog.String("plugin", entry.pluginID),
				slog.Int("expected_pid", int(expected)),
				slog.Int("actual_pid", int(creds.PID)),
				slog.Int("actual_uid", int(creds.UID)))
			return nil, status.Error(codes.Unauthenticated, "PID mismatch")
		}

		ctx = context.WithValue(ctx, pluginIDKey{}, entry.pluginID)
		return handler(ctx, req)
	}
}

type pluginIDKey struct{}

// PluginIDFromCtx returns the plugin ID injected by the auth interceptor
// after a successful SO_PEERCRED check. Returns empty string if the
// caller is not an authenticated plugin.
func PluginIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(pluginIDKey{}).(string); ok {
		return v
	}
	return ""
}

// CtxWithPluginID stamps an authenticated plugin ID into the context.
// Exported so downstream interceptor tests can simulate the effect of
// the per-plugin auth layer without booting a real Unix listener.
func CtxWithPluginID(ctx context.Context, pluginID string) context.Context {
	return context.WithValue(ctx, pluginIDKey{}, pluginID)
}

// pluginIDFromCtx is kept as a package-internal helper for tests inside
// this package; external callers should use PluginIDFromCtx.
func pluginIDFromCtx(ctx context.Context) string { return PluginIDFromCtx(ctx) }

func (h *HostServer) GetMediaItem(ctx context.Context, req *pluginv1.GetMediaItemRequest) (*pluginv1.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.GetMediaItem")
	defer span.End()
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}
	item, err := h.mediaItems.GetByID(ctx, shared.MediaItemID(req.Id))
	if err != nil || item == nil {
		return nil, status.Errorf(codes.NotFound, "media item %s not found", req.Id)
	}
	return mediaItemToProto(item), nil
}

func (h *HostServer) ListMediaItems(ctx context.Context, req *pluginv1.ListMediaItemsRequest) (*pluginv1.ListMediaItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.ListMediaItems")
	defer span.End()
	filter := library.ListMediaItemsFilter{}
	if req.LibraryId != "" {
		libID := shared.LibraryID(req.LibraryId)
		filter.LibraryID = &libID
	}
	if req.Type != "" {
		filter.Type = &req.Type
	}
	if req.ParentId != "" {
		filter.ParentID = &req.ParentId
	}
	if req.Limit > 0 {
		filter.Limit = int(req.Limit)
	}
	if req.Offset > 0 {
		filter.Offset = int(req.Offset)
	}

	items, err := h.mediaItems.List(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list media: %v", err)
	}
	total, err := h.mediaItems.Count(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "count media: %v", err)
	}

	pbItems := make([]*pluginv1.MediaItem, 0, len(items))
	for _, item := range items {
		pbItems = append(pbItems, mediaItemToProto(item))
	}
	return &pluginv1.ListMediaItemsResponse{Items: pbItems, Total: int32(total)}, nil
}

// UpdateMetadata lets a metadata plugin (capability metadata.provider) give
// its text for one movie or series in one locale, at any time: a correction,
// a catalogue it keeps up to date. It is the answer to metadata.request sent
// unasked, stored the same way: beside TMDB's text, shown through the fields
// the administrator allows. fields carries "locale" (required) and any of
// "title", "overview", "tagline"; the text replaces what the plugin gave for
// that locale, and a text with no field removes it. provider_ids is refused:
// TMDB is the item's identity, a plugin does not rematch it.
func (h *HostServer) UpdateMetadata(ctx context.Context, req *pluginv1.UpdateMetadataRequest) (*pluginv1.UpdateMetadataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.UpdateMetadata")
	defer span.End()
	if h.pluginTexts == nil {
		return nil, status.Error(codes.Unimplemented, "UpdateMetadata is not available on this server")
	}
	pluginID := pluginIDFromCtx(ctx)
	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil || p == nil {
		return nil, status.Error(codes.PermissionDenied, "unknown plugin")
	}
	if !slices.Contains(p.Capabilities, plugindomain.CapabilityMetadataProvider) {
		return nil, status.Errorf(codes.PermissionDenied, "plugin %s lacks the %s capability", pluginID, plugindomain.CapabilityMetadataProvider)
	}
	if len(req.GetProviderIds()) > 0 {
		return nil, status.Error(codes.InvalidArgument, "provider_ids cannot be set: TMDB is the item's identity")
	}
	text := library.PluginText{MediaItemID: req.GetMediaItemId(), PluginID: pluginID}
	for k, v := range req.GetFields() {
		v = strings.TrimSpace(v)
		if len(v) > maxPluginTextField {
			return nil, status.Errorf(codes.InvalidArgument, "field %q is longer than %d bytes", k, maxPluginTextField)
		}
		switch k {
		case "locale":
			text.Locale = normalizePluginLocale(v)
		case plugindomain.MetadataFieldTitle:
			text.Title = v
		case plugindomain.MetadataFieldOverview:
			text.Overview = v
		case plugindomain.MetadataFieldTagline:
			text.Tagline = v
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unknown field %q (locale, title, overview, tagline)", k)
		}
	}
	if text.Locale == "" {
		return nil, status.Error(codes.InvalidArgument, `fields["locale"] is required (a language code such as "fr")`)
	}
	item, err := h.mediaItems.GetByID(ctx, shared.MediaItemID(req.GetMediaItemId()))
	if err != nil || item == nil {
		return nil, status.Error(codes.NotFound, "media item not found")
	}
	if item.Type != library.MovieItem && item.Type != library.Series {
		return nil, status.Errorf(codes.FailedPrecondition, "a metadata plugin covers movies and series, not a %s", item.Type)
	}
	if err := h.pluginTexts.ReplaceLocale(ctx, text); err != nil {
		span.RecordError(err)
		return nil, status.Error(codes.Internal, "store the plugin's text")
	}
	return &pluginv1.UpdateMetadataResponse{Success: true}, nil
}

// maxPluginTextField bounds one field a plugin gives, as the answer to
// metadata.request is bounded.
const maxPluginTextField = 8 << 10

// normalizePluginLocale keeps the language of a locale tag ("fr-FR" -> "fr"),
// "" when it is not one.
func normalizePluginLocale(l string) string {
	l = strings.ToLower(l)
	if i := strings.IndexAny(l, "-_"); i >= 0 {
		l = l[:i]
	}
	if len(l) < 2 || len(l) > 3 {
		return ""
	}
	for _, c := range l {
		if c < 'a' || c > 'z' {
			return ""
		}
	}
	return l
}

func (h *HostServer) GetUserPreferences(_ context.Context, _ *pluginv1.GetUserPreferencesRequest) (*pluginv1.GetUserPreferencesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetUserPreferences not yet implemented")
}

func (h *HostServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.EmitEvent")
	defer span.End()
	pluginID := pluginIDFromCtx(ctx)

	payload := make(map[string]any)
	if req.Payload != nil {
		payload = req.Payload.AsMap()
	}

	evt := pluginEmittedEvent{
		pluginID:  pluginID,
		eventType: req.EventType,
		payload:   payload,
	}
	if err := h.events.Publish(ctx, evt); err != nil {
		return nil, status.Errorf(codes.Internal, "publish event: %v", err)
	}
	return &pluginv1.EmitEventResponse{}, nil
}

func (h *HostServer) Log(ctx context.Context, req *pluginv1.LogRequest) (*pluginv1.LogResponse, error) {
	_, span := tracing.StartSpan(ctx, "pluginhost.Log")
	defer span.End()
	pluginID := pluginIDFromCtx(ctx)
	level := strings.ToUpper(req.Level.String())
	if level == "LOG_LEVEL_UNSPECIFIED" {
		level = "INFO"
	}
	level = strings.TrimPrefix(level, "LOG_LEVEL_")

	if h.logger != nil {
		var b strings.Builder
		fmt.Fprintf(&b, "[%s] %s", level, req.Message)
		if len(req.Fields) > 0 {
			for k, v := range req.Fields {
				fmt.Fprintf(&b, " %s=%s", k, v)
			}
		}
		line := b.String()
		h.logger.WriteLog(pluginID, line)
	}
	return &pluginv1.LogResponse{}, nil
}

func (h *HostServer) GetConfig(ctx context.Context, req *pluginv1.GetConfigRequest) (*pluginv1.GetConfigResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.GetConfig")
	defer span.End()
	pluginID := pluginIDFromCtx(ctx)
	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "plugin %s not found", pluginID)
	}
	val, ok := p.Config[req.Key]
	if !ok {
		return &pluginv1.GetConfigResponse{Found: false}, nil
	}
	return &pluginv1.GetConfigResponse{Value: fmt.Sprintf("%v", val), Found: true}, nil
}

func (h *HostServer) SetConfig(ctx context.Context, req *pluginv1.SetConfigRequest) (*pluginv1.SetConfigResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.SetConfig")
	defer span.End()
	pluginID := pluginIDFromCtx(ctx)
	p, err := h.plugins.GetByID(ctx, pluginID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "plugin %s not found", pluginID)
	}
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "config key must not be empty")
	}
	if len(req.Key) > maxConfigKeyLen || len(req.Value) > maxConfigValueLen {
		return nil, status.Errorf(codes.InvalidArgument,
			"config key must be <= %d bytes and value <= %d bytes", maxConfigKeyLen, maxConfigValueLen)
	}
	if p.Config == nil {
		p.Config = make(map[string]any)
	}
	// Bound the number of DISTINCT keys, not just one call's size. MaxRecvMsgSize
	// caps a single request; nothing capped the config JSON growing without limit
	// across thousands of calls, and this RPC needs no permission.
	if _, exists := p.Config[req.Key]; !exists && len(p.Config) >= maxConfigKeys {
		return nil, status.Errorf(codes.ResourceExhausted,
			"config is limited to %d keys", maxConfigKeys)
	}
	p.Config[req.Key] = req.Value
	// UpdateConfig, NOT Update: writing back the snapshot read above would replay
	// permissions, external_fetch, enabled and status as they were at GetByID
	// time, so a plugin looping here would eventually land across an admin's
	// revocation and restore it - from the one RPC needing no permission.
	if err := h.plugins.UpdateConfig(ctx, pluginID, p.Config); err != nil {
		return nil, status.Errorf(codes.Internal, "save config: %v", err)
	}
	return &pluginv1.SetConfigResponse{}, nil
}

// Config bounds for the plugin-writable store. SetConfig requires no permission,
// so its growth must be bounded by something other than the caller's restraint.
const (
	maxConfigKeyLen   = 256
	maxConfigValueLen = 16 * 1024
	maxConfigKeys     = 256
)

// The data RPCs persist the plugin's OWN key/value store (plugin-global scope,
// table plugin_data). The plugin id is taken from the authenticated context, so
// a plugin can only ever read/write its own namespace.

func (h *HostServer) GetData(ctx context.Context, req *pluginv1.GetDataRequest) (*pluginv1.GetDataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.GetData")
	defer span.End()
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "data store unavailable")
	}
	e, err := h.data.Get(ctx, pluginIDFromCtx(ctx), "", "", req.Key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &pluginv1.GetDataResponse{Found: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "get data: %v", err)
	}
	return &pluginv1.GetDataResponse{Value: e.Value, Found: true}, nil
}

func (h *HostServer) SetData(ctx context.Context, req *pluginv1.SetDataRequest) (*pluginv1.SetDataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.SetData")
	defer span.End()
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "data store unavailable")
	}
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	pluginID := pluginIDFromCtx(ctx)
	if err := plugindomain.CheckPluginDataWrite(ctx, h.data, pluginID, "", "", req.Key, req.Value); err != nil {
		return nil, dataWriteStatus(err)
	}
	if err := h.data.Set(ctx, pluginID, "", "", req.Key, req.Value); err != nil {
		return nil, status.Errorf(codes.Internal, "set data: %v", err)
	}
	return &pluginv1.SetDataResponse{}, nil
}

// dataWriteStatus maps a refused data write to its gRPC code.
func dataWriteStatus(err error) error {
	switch {
	case errors.Is(err, plugindomain.ErrPluginDataTooLarge):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, plugindomain.ErrPluginDataFull):
		return status.Error(codes.ResourceExhausted, err.Error())
	}
	return status.Errorf(codes.Internal, "set data: %v", err)
}

func (h *HostServer) DeleteData(ctx context.Context, req *pluginv1.DeleteDataRequest) (*pluginv1.DeleteDataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.DeleteData")
	defer span.End()
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "data store unavailable")
	}
	if err := h.data.Delete(ctx, pluginIDFromCtx(ctx), "", "", req.Key); err != nil {
		return nil, status.Errorf(codes.Internal, "delete data: %v", err)
	}
	return &pluginv1.DeleteDataResponse{}, nil
}

func (h *HostServer) ListDataKeys(ctx context.Context, _ *pluginv1.ListDataKeysRequest) (*pluginv1.ListDataKeysResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "pluginhost.ListDataKeys")
	defer span.End()
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "data store unavailable")
	}
	entries, err := h.data.List(ctx, pluginIDFromCtx(ctx), "", "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list data: %v", err)
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	return &pluginv1.ListDataKeysResponse{Keys: keys}, nil
}

func mediaItemToProto(item *library.MediaItem) *pluginv1.MediaItem {
	if item == nil {
		return nil
	}
	pb := &pluginv1.MediaItem{
		Id:        string(item.ID),
		LibraryId: string(item.LibraryID),
		ParentId:  item.ParentID,
		Type:      string(item.Type),
		Title:     item.Title,
		Overview:  item.Overview,
	}
	if item.File != nil {
		pb.FilePath = item.File.Path
	}
	if item.Catalog != nil {
		pb.Genres = item.Catalog.Genres
		if item.Catalog.Year != nil {
			pb.Year = int32(*item.Catalog.Year)
		}
		if item.Catalog.ProviderIDs != nil {
			pb.ProviderIds = item.Catalog.ProviderIDs
		}
	}
	return pb
}

type pluginEmittedEvent struct {
	pluginID  string
	eventType string
	payload   map[string]any
}

func (e pluginEmittedEvent) EventName() string {
	return "plugin.event." + e.eventType
}
