package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

type memMetadataSettings struct{ fields map[string][]string }

func (m *memMetadataSettings) MetadataFields(_ context.Context, id string) ([]string, error) {
	if f, ok := m.fields[id]; ok {
		return f, nil
	}
	return plugin.DefaultMetadataFields, nil
}

func (m *memMetadataSettings) SetMetadataFields(_ context.Context, id string, fields []string) error {
	m.fields[id] = fields
	return nil
}

type fakeRefresher struct {
	started []string
	err     error
	status  appmetadata.PluginTextRefreshStatus
}

func (f *fakeRefresher) Start(id string) error {
	if f.err != nil {
		return f.err
	}
	f.started = append(f.started, id)
	return nil
}

func (f *fakeRefresher) Status(string) appmetadata.PluginTextRefreshStatus { return f.status }

func newMetadataHandler(t *testing.T) (*adminPluginHandler, *fakeRefresher) {
	t.Helper()
	repo := plgfakes.NewFakePluginRepository()
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugin.Plugin{ID: "allocine", Name: "allocine", Capabilities: []string{plugin.CapabilityMetadataProvider}}))
	require.NoError(t, repo.Create(ctx, &plugin.Plugin{ID: "weather", Name: "weather", Capabilities: []string{"media.read"}}))
	ref := &fakeRefresher{status: appmetadata.PluginTextRefreshStatus{Done: 3, Total: 10, Covered: 2, Failed: 1, FinishedAt: time.Unix(1700000000, 0)}}
	return &adminPluginHandler{
		translator:       eveyi18n.NewEmpty(),
		enabled:          true,
		metadataSettings: appplugin.NewMetadataSettingsService(repo, &memMetadataSettings{fields: map[string][]string{}}),
		metadataRefresh:  ref,
	}, ref
}

// The settings read gives the default, the update keeps display order, and
// both carry the walk's progress.
func TestPluginMetadataSettings_GetThenUpdate(t *testing.T) {
	h, _ := newMetadataHandler(t)
	ctx := adminCtx()

	got, err := h.GetPluginMetadataSettings(ctx, &eveyv1.GetPluginMetadataSettingsRequest{PluginId: "allocine"})
	require.NoError(t, err)
	assert.Equal(t, []string{"overview", "tagline"}, got.Fields)
	assert.Equal(t, []string{"title", "overview", "tagline"}, got.AvailableFields)
	assert.Equal(t, int32(3), got.Refresh.Done)
	assert.Equal(t, int32(10), got.Refresh.Total)
	assert.Equal(t, int32(2), got.Refresh.Covered)
	assert.Equal(t, int32(1), got.Refresh.Failed)
	assert.Equal(t, int64(1700000000), got.Refresh.FinishedAt.GetSeconds())

	upd, err := h.UpdatePluginMetadataSettings(ctx, &eveyv1.UpdatePluginMetadataSettingsRequest{PluginId: "allocine", Fields: []string{"tagline", "title", "title"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"title", "tagline"}, upd.Fields)
	again, err := h.GetPluginMetadataSettings(ctx, &eveyv1.GetPluginMetadataSettingsRequest{PluginId: "allocine"})
	require.NoError(t, err)
	assert.Equal(t, []string{"title", "tagline"}, again.Fields)
}

func TestPluginMetadataSettings_Refusals(t *testing.T) {
	h, ref := newMetadataHandler(t)
	ctx := adminCtx()
	code := func(err error) codes.Code { return status.Code(err) }

	_, err := h.GetPluginMetadataSettings(ctx, &eveyv1.GetPluginMetadataSettingsRequest{PluginId: "missing"})
	assert.Equal(t, codes.NotFound, code(err))
	_, err = h.GetPluginMetadataSettings(ctx, &eveyv1.GetPluginMetadataSettingsRequest{PluginId: "weather"})
	assert.Equal(t, codes.FailedPrecondition, code(err))
	_, err = h.UpdatePluginMetadataSettings(ctx, &eveyv1.UpdatePluginMetadataSettingsRequest{PluginId: "allocine", Fields: []string{"poster"}})
	assert.Equal(t, codes.InvalidArgument, code(err))
	_, err = h.RefreshPluginMetadata(ctx, &eveyv1.RefreshPluginMetadataRequest{PluginId: "weather"})
	assert.Equal(t, codes.FailedPrecondition, code(err))
	assert.Empty(t, ref.started)

	ref.err = appmetadata.ErrPluginRefreshRunning
	_, err = h.RefreshPluginMetadata(ctx, &eveyv1.RefreshPluginMetadataRequest{PluginId: "allocine"})
	assert.Equal(t, codes.FailedPrecondition, code(err))

	h.enabled = false
	_, err = h.UpdatePluginMetadataSettings(ctx, &eveyv1.UpdatePluginMetadataSettingsRequest{PluginId: "allocine"})
	assert.Equal(t, codes.FailedPrecondition, code(err))

	_, err = h.GetPluginMetadataSettings(context.Background(), &eveyv1.GetPluginMetadataSettingsRequest{PluginId: "allocine"})
	assert.Equal(t, codes.Unauthenticated, code(err))
}

func TestPluginMetadataSettings_RefreshStarts(t *testing.T) {
	h, ref := newMetadataHandler(t)
	_, err := h.RefreshPluginMetadata(adminCtx(), &eveyv1.RefreshPluginMetadataRequest{PluginId: "allocine"})
	require.NoError(t, err)
	assert.Equal(t, []string{"allocine"}, ref.started)
}
