package grpcapi

import (
	"context"
	"errors"

	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// pluginMetadataRefresher is the library walk of one metadata plugin
// (*appmetadata.PluginTextRefresher).
type pluginMetadataRefresher interface {
	Start(pluginID string) error
	Status(pluginID string) appmetadata.PluginTextRefreshStatus
}

// pluginMetadataSettings is the field choice per metadata plugin
// (*appplugin.MetadataSettingsService).
type pluginMetadataSettings interface {
	Fields(ctx context.Context, pluginID string) ([]string, error)
	SetFields(ctx context.Context, pluginID string, fields []string) ([]string, error)
}

func (d *adminPluginHandler) GetPluginMetadataSettings(ctx context.Context, req *eveyv1.GetPluginMetadataSettingsRequest) (*eveyv1.PluginMetadataSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetPluginMetadataSettings")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	if d.metadataSettings == nil {
		return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.system_disabled")
	}
	fields, err := d.metadataSettings.Fields(ctx, req.GetPluginId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.pluginMetadataError(ctx, "get plugin metadata settings", err)
	}
	return d.pluginMetadataSettingsProto(req.GetPluginId(), fields), nil
}

func (d *adminPluginHandler) UpdatePluginMetadataSettings(ctx context.Context, req *eveyv1.UpdatePluginMetadataSettingsRequest) (*eveyv1.PluginMetadataSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UpdatePluginMetadataSettings")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	if err := d.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if d.metadataSettings == nil {
		return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.system_disabled")
	}
	fields, err := d.metadataSettings.SetFields(ctx, req.GetPluginId(), req.GetFields())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.pluginMetadataError(ctx, "update plugin metadata settings", err)
	}
	return d.pluginMetadataSettingsProto(req.GetPluginId(), fields), nil
}

func (d *adminPluginHandler) RefreshPluginMetadata(ctx context.Context, req *eveyv1.RefreshPluginMetadataRequest) (*eveyv1.PluginMetadataSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RefreshPluginMetadata")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	if err := d.requireEnabled(ctx); err != nil {
		return nil, err
	}
	if d.metadataSettings == nil || d.metadataRefresh == nil {
		return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.system_disabled")
	}
	// The settings read checks the plugin exists and is a metadata provider.
	fields, err := d.metadataSettings.Fields(ctx, req.GetPluginId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.pluginMetadataError(ctx, "refresh plugin metadata", err)
	}
	if err := d.metadataRefresh.Start(req.GetPluginId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.pluginMetadataError(ctx, "refresh plugin metadata", err)
	}
	return d.pluginMetadataSettingsProto(req.GetPluginId(), fields), nil
}

// pluginMetadataError maps the metadata settings and refresh refusals.
func (d *adminPluginHandler) pluginMetadataError(ctx context.Context, op string, err error) error {
	switch {
	case errors.Is(err, appplugin.ErrPluginNotFound):
		return d.translator.TranslatedError(ctx, codes.NotFound, "errors.plugin.not_found")
	case errors.Is(err, appplugin.ErrNotMetadataPlugin):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.not_metadata_provider")
	case errors.Is(err, appplugin.ErrUnknownMetadataField):
		return d.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.plugin.metadata_field_unknown")
	case errors.Is(err, appmetadata.ErrPluginRefreshRunning):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.plugin.metadata_refresh_running")
	}
	return internalError(ctx, op, err)
}

func (d *adminPluginHandler) pluginMetadataSettingsProto(pluginID string, fields []string) *eveyv1.PluginMetadataSettings {
	out := &eveyv1.PluginMetadataSettings{
		PluginId:        pluginID,
		Fields:          fields,
		AvailableFields: append([]string(nil), plugin.MetadataFields...),
		Refresh:         &eveyv1.PluginMetadataRefresh{},
	}
	if d.metadataRefresh != nil {
		st := d.metadataRefresh.Status(pluginID)
		out.Refresh = &eveyv1.PluginMetadataRefresh{
			Running: st.Running,
			Done:    int32(st.Done),
			Total:   int32(st.Total),
			Covered: int32(st.Covered),
			Failed:  int32(st.Failed),
		}
		if !st.FinishedAt.IsZero() {
			out.Refresh.FinishedAt = timestamppb.New(st.FinishedAt)
		}
	}
	return out
}
