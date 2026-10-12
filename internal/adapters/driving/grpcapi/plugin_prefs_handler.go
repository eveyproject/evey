package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginUserDataHandler serves the iframe-facing plugin preference RPCs. The
// (plugin, user) pair is read from the scoped plugin token in context, never
// from the request, so a plugin can only touch its own namespace for the
// current user.
type pluginUserDataHandler struct {
	eveyv1.UnimplementedPluginUserDataServiceServer
	data plugin.PluginDataRepository
}

func newPluginUserDataHandler(data plugin.PluginDataRepository) *pluginUserDataHandler {
	return &pluginUserDataHandler{data: data}
}

// scope resolves the calling plugin, user and active profile from the token.
// These methods are only meaningful for a scoped plugin token (the iframe); any
// other caller is rejected. profileID may be empty (no active profile) - the
// store then falls back to the per-user scope.
func (h *pluginUserDataHandler) scope(ctx context.Context) (pluginID, userID, profileID string, err error) {
	c := interceptors.PluginClaimsFromContext(ctx)
	if c == nil {
		return "", "", "", status.Error(codes.PermissionDenied, "plugin token required")
	}
	if c.PluginID == "" || c.UserID == "" {
		return "", "", "", status.Error(codes.PermissionDenied, "plugin token missing plugin or user binding")
	}
	return c.PluginID, c.UserID, c.ProfileID, nil
}

func (h *pluginUserDataHandler) GetPref(ctx context.Context, req *eveyv1.GetPrefRequest) (*eveyv1.GetPrefResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.plugin.GetPref")
	defer span.End()
	pluginID, userID, profileID, err := h.scope(ctx)
	if err != nil {
		return nil, err
	}
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "plugin data store unavailable")
	}
	e, err := h.data.Get(ctx, pluginID, userID, profileID, req.Key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &eveyv1.GetPrefResponse{Found: false}, nil
		}
		return nil, internalError(ctx, "get pref", err)
	}
	return &eveyv1.GetPrefResponse{Value: e.Value, Found: true}, nil
}

func (h *pluginUserDataHandler) SetPref(ctx context.Context, req *eveyv1.SetPrefRequest) (*eveyv1.SetPrefResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.plugin.SetPref")
	defer span.End()
	pluginID, userID, profileID, err := h.scope(ctx)
	if err != nil {
		return nil, err
	}
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "plugin data store unavailable")
	}
	if err := h.data.Set(ctx, pluginID, userID, profileID, req.Key, req.Value); err != nil {
		return nil, internalError(ctx, "set pref", err)
	}
	return &eveyv1.SetPrefResponse{}, nil
}

func (h *pluginUserDataHandler) DeletePref(ctx context.Context, req *eveyv1.DeletePrefRequest) (*eveyv1.DeletePrefResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.plugin.DeletePref")
	defer span.End()
	pluginID, userID, profileID, err := h.scope(ctx)
	if err != nil {
		return nil, err
	}
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "plugin data store unavailable")
	}
	if err := h.data.Delete(ctx, pluginID, userID, profileID, req.Key); err != nil {
		return nil, internalError(ctx, "delete pref", err)
	}
	return &eveyv1.DeletePrefResponse{}, nil
}

func (h *pluginUserDataHandler) ListPrefs(ctx context.Context, _ *eveyv1.ListPrefsRequest) (*eveyv1.ListPrefsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.plugin.ListPrefs")
	defer span.End()
	pluginID, userID, profileID, err := h.scope(ctx)
	if err != nil {
		return nil, err
	}
	if h.data == nil {
		return nil, status.Error(codes.Unimplemented, "plugin data store unavailable")
	}
	entries, err := h.data.List(ctx, pluginID, userID, profileID)
	if err != nil {
		return nil, internalError(ctx, "list prefs", err)
	}
	resp := &eveyv1.ListPrefsResponse{}
	for _, e := range entries {
		resp.Prefs = append(resp.Prefs, &eveyv1.PluginPref{Key: e.Key, Value: e.Value})
	}
	return resp, nil
}
