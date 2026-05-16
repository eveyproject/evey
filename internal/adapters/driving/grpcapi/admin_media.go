package grpcapi

import (
	"context"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
)

type adminMediaHandler struct {
	eveyv1.UnimplementedAdminMediaServiceServer
	metadataCmd      *appmetadata.CommandService
	libCmd           *applibrary.CommandService
	heavyOpSemaphore chan struct{}
	translator       *eveyi18n.Translator
}

func (d *adminMediaHandler) acquireHeavyOp() bool {
	if d.heavyOpSemaphore == nil {
		return true
	}
	select {
	case d.heavyOpSemaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

func (d *adminMediaHandler) releaseHeavyOp() {
	if d.heavyOpSemaphore != nil {
		<-d.heavyOpSemaphore
	}
}

func (d *adminMediaHandler) RefreshAllMetadata(ctx context.Context, req *eveyv1.RefreshAllMetadataRequest) (*eveyv1.RefreshAllMetadataResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RefreshAllMetadata")
	defer span.End()
	if d.metadataCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	if !d.acquireHeavyOp() {
		span.SetStatus(otelcodes.Error, "heavy op running")
		return nil, d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.heavy_op_running")
	}
	defer d.releaseHeavyOp()
	if err := d.metadataCmd.AutoRefreshAllItems(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("refresh all metadata", err)
	}
	return &eveyv1.RefreshAllMetadataResponse{}, nil
}

func (d *adminMediaHandler) MatchOrphansInLibrary(ctx context.Context, req *eveyv1.MatchOrphansInLibraryRequest) (*eveyv1.MatchOrphansInLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.MatchOrphansInLibrary")
	defer span.End()
	if d.metadataCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	queued, err := d.metadataCmd.MatchOrphansInLibrary(ctx, req.GetLibraryId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("match orphans", err)
	}
	return &eveyv1.MatchOrphansInLibraryResponse{Queued: int32(queued)}, nil
}

func (d *adminMediaHandler) GenerateAllTrickplays(ctx context.Context, _ *eveyv1.GenerateAllTrickplaysRequest) (*eveyv1.GenerateAllTrickplaysResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GenerateAllTrickplays")
	defer span.End()
	if d.libCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	if !d.acquireHeavyOp() {
		span.SetStatus(otelcodes.Error, "heavy op running")
		return nil, d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.heavy_op_running")
	}
	defer d.releaseHeavyOp()
	n, err := d.libCmd.GenerateAllMissingTrickplays(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("generate all trickplays", err)
	}
	return &eveyv1.GenerateAllTrickplaysResponse{Generated: int32(n)}, nil
}

func (d *adminMediaHandler) DeleteMediaItem(ctx context.Context, req *eveyv1.DeleteMediaItemRequest) (*eveyv1.DeleteMediaItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.DeleteMediaItem")
	defer span.End()
	if d.libCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	if err := d.libCmd.DeleteMediaItem(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("delete media item", err)
	}
	return &eveyv1.DeleteMediaItemResponse{}, nil
}
