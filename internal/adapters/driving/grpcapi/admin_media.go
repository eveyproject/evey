package grpcapi

import (
	"context"
	"errors"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainmkv "github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// libraryGetter is the narrow slice of applibrary.QueryService the type guard
// needs; an interface so tests can fake a library without the full service.
type libraryGetter interface {
	GetLibrary(ctx context.Context, id string) (*domainlibrary.Library, error)
}

type adminMediaHandler struct {
	eveyv1.UnimplementedAdminMediaServiceServer
	metadataCmd      *appmetadata.CommandService
	libCmd           *applibrary.CommandService
	libQuery         libraryGetter
	seekIndex        *appmaintenance.SeekIndexService
	heavyOpSemaphore chan struct{}
	translator       *eveyi18n.Translator
	serverCtx        context.Context
}

// guardProviderMatchable rejects provider re-match requests on library types
// the enrichment chain cannot serve: music and photo items never live in
// media_items, so the sweep would be a silent no-op that still holds the
// heavy-op slot and shows a lying "started" toast. Movie and show libraries
// pass; an empty scope (whole catalogue) and a missing libQuery keep the
// legacy permissive behavior.
func (d *adminMediaHandler) guardProviderMatchable(ctx context.Context, libraryID string) error {
	if libraryID == "" || d.libQuery == nil {
		return nil
	}
	lib, err := d.libQuery.GetLibrary(ctx, libraryID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return status.Error(codes.NotFound, "library not found")
		}
		return internalError(ctx, "load library for metadata guard", err)
	}
	if lib.Type != domainlibrary.Movie && lib.Type != domainlibrary.TVShow {
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.metadata_library_unmatchable")
	}
	return nil
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
	if err := d.guardProviderMatchable(ctx, req.GetLibraryId()); err != nil {
		span.SetStatus(otelcodes.Error, "library type not provider-matchable")
		return nil, err
	}
	if !d.acquireHeavyOp() {
		span.SetStatus(otelcodes.Error, "heavy op running")
		return nil, d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.heavy_op_running")
	}
	// Detach: AutoRefreshAllItems pages the WHOLE library, re-enriching each
	// item via external provider calls (TMDB/OMDb rate-limited) - minutes to
	// hours for a large catalogue. Running it on the request goroutine blocks
	// until done, so any reverse-proxy read-timeout 504s and cancels the work
	// mid-pass. Hand it to the background (mirrors the HTTP refresh-all
	// sibling), holding the heavy-op slot until the work finishes (releasing on
	// RPC return would let a second refresh start concurrently). The request
	// returns immediately.
	srvCtx := d.serverCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	libraryID := req.GetLibraryId()
	go func() {
		defer d.releaseHeavyOp()
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in refresh all metadata", slog.Any("error", r))
			}
		}()
		if err := d.metadataCmd.AutoRefreshAllItems(srvCtx, libraryID); err != nil {
			slog.ErrorContext(ctx, "refresh all metadata", slog.Any("err", err))
		} else {
			slog.Info("refresh all metadata complete", slog.String("library_id", libraryID))
		}
	}()
	return &eveyv1.RefreshAllMetadataResponse{}, nil
}

func (d *adminMediaHandler) RelinkCollections(ctx context.Context, req *eveyv1.RelinkCollectionsRequest) (*eveyv1.RelinkCollectionsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RelinkCollections")
	defer span.End()
	if d.metadataCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	if !d.acquireHeavyOp() {
		span.SetStatus(otelcodes.Error, "heavy op running")
		return nil, d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.heavy_op_running")
	}
	// No limit means the whole catalogue: the sweep pages through it, and
	// stopping early would leave the titles behind the cut unrepaired with no
	// way for a second press to reach them.
	limit := int(req.GetLimit())
	// Detached for the same reason as RefreshAllMetadata: one provider call per
	// candidate title, rate-limited, minutes on a large catalogue - a blocking
	// request would 504 at the proxy and cancel the repair mid-sweep.
	srvCtx := d.serverCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	go func() {
		defer d.releaseHeavyOp()
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in collection relink", slog.Any("error", r))
			}
		}()
		rep, err := d.metadataCmd.RelinkCollections(srvCtx, limit)
		if err != nil {
			slog.ErrorContext(ctx, "collection relink", slog.Any("err", err))
			return
		}
		slog.Info("collection relink complete",
			slog.Int("scanned", rep.Scanned), slog.Int("linked", rep.Linked), slog.Int("failed", rep.Failed))
	}()
	return &eveyv1.RelinkCollectionsResponse{Accepted: true}, nil
}

func (d *adminMediaHandler) MatchOrphansInLibrary(ctx context.Context, req *eveyv1.MatchOrphansInLibraryRequest) (*eveyv1.MatchOrphansInLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.MatchOrphansInLibrary")
	defer span.End()
	if d.metadataCmd == nil {
		span.SetStatus(otelcodes.Error, "service not configured")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	}
	if err := d.guardProviderMatchable(ctx, req.GetLibraryId()); err != nil {
		span.SetStatus(otelcodes.Error, "library type not provider-matchable")
		return nil, err
	}
	queued, err := d.metadataCmd.MatchOrphansInLibrary(ctx, req.GetLibraryId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "match orphans", err)
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
		return nil, internalError(ctx, "generate all trickplays", err)
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
		return nil, internalError(ctx, "delete media item", err)
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
// volume, job already running, unknown media) to translated, non-500 statuses;
// everything else stays a sanitized internal error.
func (d *adminMediaHandler) seekIndexError(ctx context.Context, op string, err error) error {
	var spaceErr *appmaintenance.BackupSpaceError
	switch {
	case appmaintenance.IsReadOnlyFS(err):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.seek_index_read_only")
	case errors.As(err, &spaceErr):
		// The typed error carries the per-volume shortfall; the numbers are
		// locale-neutral, so they ride a translated envelope as one param.
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition,
			"errors.admin.seek_index_backup_overcommit", map[string]string{"volumes": spaceErr.Detail()})
	case errors.Is(err, appmaintenance.ErrRemuxUnavailable):
		return d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	case errors.Is(err, appmaintenance.ErrJobRunning):
		return d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.seek_index_job_running")
	case errors.Is(err, domainmkv.ErrNoBackup):
		return d.translator.TranslatedError(ctx, codes.NotFound, "errors.admin.seek_index_no_backup")
	case errors.Is(err, shared.ErrNotFound):
		return status.Error(codes.NotFound, "media item not found")
	default:
		return internalError(ctx, op, err)
	}
}

// ScanSeekIndex serves the cached result of the last completed scan - instant,
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
	out.AudioDelayed = int32(res.AudioDelayed)
	out.Truncated = int32(res.Truncated)
	out.Damaged = int32(res.Damaged)
	out.WrongContainer = int32(res.WrongContainer)
	out.TrailingJunk = int32(res.TrailingJunk)
	out.AudioShort = int32(res.AudioShort)
	out.SeekCoarse = int32(res.SeekCoarse)
	out.InplaceAvailable = d.seekIndex.SupportsInPlace()
	out.ScannedAtMs = at.UnixMilli()
	for _, it := range res.Items {
		out.Items = append(out.Items, &eveyv1.SeekIndexItem{
			MediaId:           it.MediaID,
			Title:             it.Title,
			Path:              it.Path,
			HasIndex:          it.HasIndex,
			HasBackup:         it.HasBackup,
			SizeBytes:         it.SizeBytes,
			Container:         it.Container,
			AudioDelaySec:     it.AudioDelaySec,
			Truncated:         it.Truncated,
			DamagedBytes:      it.DamagedBytes,
			WrongContainer:    it.WrongContainer,
			TrailingJunk:      it.TrailingJunk,
			AudioShortfallSec: it.AudioShortfallSec,
			MissingTailBytes:  it.MissingTailBytes,
			SeekCoarse:        it.SeekCoarse(),
		})
	}
	// Live per-volume numbers (free space vs candidate sizes) so the client
	// advises for/against backups itself - computed fresh on every read, the
	// free space moves under a cached scan.
	for _, v := range appmaintenance.VolumeSummary(res.Items) {
		out.Volumes = append(out.Volumes, &eveyv1.SeekIndexVolume{
			Root:                v.Root,
			FreeBytes:           v.FreeBytes,
			MissingBytes:        v.MissingBytes,
			LargestMissingBytes: v.LargestMissingBytes,
			MissingFiles:        int32(v.MissingFiles), //nolint:gosec // bounded by the library size
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
	run := appmaintenance.ReindexRunOptions{
		Mode:       req.GetMode(),
		DeepVerify: req.GetDeepVerify(),
		SkipBackup: req.GetSkipBackup(),
		Resync:     req.GetResync(),
	}
	if err := d.seekIndex.StartReindexMedia(ctx, req.GetMediaId(), run); err != nil {
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
	run := appmaintenance.ReindexRunOptions{
		Mode:         req.GetMode(),
		DeepVerify:   req.GetDeepVerify(),
		SkipBackup:   req.GetSkipBackup(),
		BackupWindow: int(req.GetBackupWindow()),
	}
	// One entry point: StartRepairLibrary is a pure reindex batch when
	// include_audio is false, and folds the audio remux in when true.
	if err := d.seekIndex.StartRepairLibrary(ctx, req.GetLibraryId(), run, req.GetIncludeAudio()); err != nil {
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

// RemuxAudioMedia repairs one file's audio start delay in the background.
func (d *adminMediaHandler) RemuxAudioMedia(ctx context.Context, req *eveyv1.RemuxAudioMediaRequest) (*eveyv1.RemuxAudioMediaResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemuxAudioMedia")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartRemuxAudioMedia(ctx, req.GetMediaId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "remux audio media", err)
	}
	return &eveyv1.RemuxAudioMediaResponse{Accepted: true}, nil
}

// RemuxAudioLibrary repairs every audio-delayed file of the scope in the
// background.
func (d *adminMediaHandler) RemuxAudioLibrary(ctx context.Context, req *eveyv1.RemuxAudioLibraryRequest) (*eveyv1.RemuxAudioLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemuxAudioLibrary")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartRemuxAudioLibrary(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "remux audio library", err)
	}
	return &eveyv1.RemuxAudioLibraryResponse{Accepted: true}, nil
}

// DeleteSeekIndexBackup removes one media's reindex backup - synchronous, an
// unlink; the freed space is the point once the repair is confirmed fine.
func (d *adminMediaHandler) DeleteSeekIndexBackup(ctx context.Context, req *eveyv1.DeleteSeekIndexBackupRequest) (*eveyv1.DeleteSeekIndexBackupResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.DeleteSeekIndexBackup")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.DeleteBackupMedia(ctx, req.GetMediaId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "delete seek index backup", err)
	}
	return &eveyv1.DeleteSeekIndexBackupResponse{}, nil
}

// PurgeSeekIndexBackups deletes every reindex backup in scope (background job).
func (d *adminMediaHandler) PurgeSeekIndexBackups(ctx context.Context, req *eveyv1.PurgeSeekIndexBackupsRequest) (*eveyv1.PurgeSeekIndexBackupsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.PurgeSeekIndexBackups")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartPurgeBackups(ctx, req.GetLibraryId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "purge seek index backups", err)
	}
	return &eveyv1.PurgeSeekIndexBackupsResponse{}, nil
}
