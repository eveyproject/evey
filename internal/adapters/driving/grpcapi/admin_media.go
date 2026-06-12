package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
)

type adminMediaHandler struct {
	eveyv1.UnimplementedAdminMediaServiceServer
	metadataCmd      *appmetadata.CommandService
	libCmd           *applibrary.CommandService
	seekIndex        *appmaintenance.SeekIndexService
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

func (d *adminMediaHandler) seekIndexConfigured(ctx context.Context, span trace.Span) error {
	if d.seekIndex == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	return nil
}

// seekIndexError maps the user-actionable seek-index failures (read-only media
// volume, job already running) to translated, non-500 statuses; everything
// else stays a sanitized internal error.
func (d *adminMediaHandler) seekIndexError(ctx context.Context, op string, err error) error {
	switch {
	case appmaintenance.IsReadOnlyFS(err):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.seek_index_read_only")
	case errors.Is(err, appmaintenance.ErrJobRunning):
		return d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.seek_index_job_running")
	default:
		return internalError(op, err)
	}
}

// ScanSeekIndex serves the cached result of the last completed scan — instant,
// no disk walk (a synchronous scan 504s behind a reverse proxy on big
// libraries). ScannedAtMs == 0 means no scan has completed for the scope yet.
func (d *adminMediaHandler) ScanSeekIndex(ctx context.Context, req *eveyv1.ScanSeekIndexRequest) (*eveyv1.ScanSeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ScanSeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	res, at, ok := d.seekIndex.CachedScan(req.GetLibraryId())
	out := &eveyv1.ScanSeekIndexResponse{}
	if !ok {
		return out, nil
	}
	out.Total = int32(res.Total)
	out.MissingIndex = int32(res.MissingIndex)
	out.WithBackup = int32(res.WithBackup)
	out.ScannedAtMs = at.UnixMilli()
	for _, it := range res.Items {
		out.Items = append(out.Items, &eveyv1.SeekIndexItem{
			MediaId:   it.MediaID,
			Title:     it.Title,
			Path:      it.Path,
			HasIndex:  it.HasIndex,
			HasBackup: it.HasBackup,
			SizeBytes: it.SizeBytes,
			Container: it.Container,
		})
	}
	return out, nil
}

func (d *adminMediaHandler) StartSeekIndexScan(ctx context.Context, req *eveyv1.StartSeekIndexScanRequest) (*eveyv1.StartSeekIndexScanResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.StartSeekIndexScan")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartScan(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "start seek index scan", err)
	}
	return &eveyv1.StartSeekIndexScanResponse{}, nil
}

func (d *adminMediaHandler) GetSeekIndexJob(ctx context.Context, _ *eveyv1.GetSeekIndexJobRequest) (*eveyv1.GetSeekIndexJobResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetSeekIndexJob")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	st := d.seekIndex.Status()
	out := &eveyv1.GetSeekIndexJobResponse{
		Running:     st.Running,
		Kind:        st.Kind,
		Scope:       st.Scope,
		Phase:       st.Phase,
		Done:        int32(st.Done),
		Failed:      int32(st.Failed),
		Skipped:     int32(st.Skipped),
		Total:       int32(st.Total),
		CurrentPath: st.CurrentPath,
	}
	if st.Running {
		out.StartedAtMs = st.StartedAt.UnixMilli()
	}
	if st.Last != nil {
		last := &eveyv1.SeekIndexJobSummary{
			Kind:         st.Last.Kind,
			Scope:        st.Last.Scope,
			Done:         int32(st.Last.Done),
			Failed:       int32(st.Last.Failed),
			Skipped:      int32(st.Last.Skipped),
			Total:        int32(st.Last.Total),
			FinishedAtMs: st.Last.FinishedAt.UnixMilli(),
			Error:        st.Last.Err,
			ReadOnly:     st.Last.ReadOnly,
		}
		for _, f := range st.Last.Failures {
			last.Failures = append(last.Failures, &eveyv1.SeekIndexJobFailure{
				Path:     f.Path,
				Error:    f.Error,
				ReadOnly: f.ReadOnly,
			})
		}
		out.Last = last
	}
	return out, nil
}

func (d *adminMediaHandler) ReindexMediaSeekIndex(ctx context.Context, req *eveyv1.ReindexMediaSeekIndexRequest) (*eveyv1.ReindexMediaSeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ReindexMediaSeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartReindexMedia(ctx, req.GetMediaId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "reindex media seek index", err)
	}
	return &eveyv1.ReindexMediaSeekIndexResponse{Accepted: true}, nil
}

func (d *adminMediaHandler) RollbackMediaSeekIndex(ctx context.Context, req *eveyv1.RollbackMediaSeekIndexRequest) (*eveyv1.RollbackMediaSeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RollbackMediaSeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartRollbackMedia(ctx, req.GetMediaId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "rollback media seek index", err)
	}
	return &eveyv1.RollbackMediaSeekIndexResponse{Accepted: true}, nil
}

func (d *adminMediaHandler) ReindexLibrarySeekIndex(ctx context.Context, req *eveyv1.ReindexLibrarySeekIndexRequest) (*eveyv1.ReindexLibrarySeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ReindexLibrarySeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartReindexLibrary(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "reindex library seek index", err)
	}
	return &eveyv1.ReindexLibrarySeekIndexResponse{Queued: -1}, nil
}

func (d *adminMediaHandler) RollbackLibrarySeekIndex(ctx context.Context, req *eveyv1.RollbackLibrarySeekIndexRequest) (*eveyv1.RollbackLibrarySeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RollbackLibrarySeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartRollbackLibrary(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "rollback library seek index", err)
	}
	return &eveyv1.RollbackLibrarySeekIndexResponse{Queued: -1}, nil
}
