package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
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
	// activitySvc records who asked for each repair: these RPCs rewrite media
	// bytes or delete the only way back, and used to leave no trace.
	activitySvc *appactivity.CommandService
}

// audit records an accepted repair against the calling admin.
func (d *adminMediaHandler) audit(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	userID := ""
	if c := interceptors.ClaimsFromContext(ctx); c != nil {
		userID = c.UserID
	}
	var rid *string
	if resourceID != "" {
		rid = &resourceID
	}
	recordAudit(ctx, d.activitySvc, userID, action, resourceType, rid, details)
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
		if errors.Is(err, shared.ErrNotFound) {
			return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
		}
		return nil, internalError(ctx, "delete media item", err)
	}
	d.audit(ctx, "delete_media_item", "media", req.Id, nil)
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
	case errors.Is(err, appmaintenance.ErrAckNotApplicable):
		return d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.seek_index_ack_not_applicable")
	case errors.Is(err, appmaintenance.ErrNotInScan):
		return d.translator.TranslatedError(ctx, codes.NotFound, "errors.admin.seek_index_not_in_scan")
	case errors.Is(err, appmaintenance.ErrContainerFixUnavailable):
		return d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.service_not_configured")
	case errors.Is(err, appmaintenance.ErrJobRunning):
		return d.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.admin.seek_index_job_running")
	case errors.Is(err, domainmkv.ErrNoBackup):
		return d.translator.TranslatedError(ctx, codes.NotFound, "errors.admin.seek_index_no_backup")
	case errors.Is(err, shared.ErrNotFound):
		return status.Error(codes.NotFound, "media item not found")
	case errors.Is(err, appmaintenance.ErrShiftOutOfRange):
		return status.Error(codes.InvalidArgument, appmaintenance.ErrShiftOutOfRange.Error())
	default:
		// An undo the journal refuses is a verdict on the file, not a fault:
		// the sentinel's own sentence is the operator's answer.
		for _, refusal := range []error{appmaintenance.ErrNoRepairToUndo, appmaintenance.ErrNotReversible, appmaintenance.ErrJournalSchema} {
			if errors.Is(err, refusal) {
				return status.Error(codes.FailedPrecondition, refusal.Error())
			}
		}
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
	out.BrokenInterleave = int32(res.BrokenInterleave)
	out.Repairable = int32(res.Repairable)
	out.Informational = int32(res.Informational)
	out.Acked = int32(res.Acked)
	out.InplaceAvailable = d.seekIndex.SupportsInPlace()
	out.ContainerFixAvailable = d.seekIndex.SupportsContainerFix()
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
			ResyncRequired:    it.ResyncRequired,
			WrongContainer:    it.WrongContainer,
			TrailingJunk:      it.TrailingJunk,
			AudioShortfallSec: it.AudioShortfallSec,
			MissingTailBytes:  it.MissingTailBytes,
			SeekCoarse:        it.SeekCoarse(),
			Verdict:           string(it.Verdict()),
			AckedAtMs:         it.AckedAt * 1000,
			SeekCoarseDetail:  it.IndexSparseDetail,
			// The proof behind the offset, not just the offset.
			AudioTracksLagSec:     it.AudioTracksLagSec,
			AudioTracksChecked:    it.AudioTracksChecked,
			AudioTrackZeroed:      it.AudioTrackZeroed,
			InterleaveBackstepSec: it.InterleaveBackstepSec,
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
	run := appmaintenance.ScanRunOptions{Pace: appmaintenance.ScanPace(req.GetPace())}
	if err := d.seekIndex.StartScan(ctx, req.GetLibraryId(), run); err != nil {
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
	d.audit(ctx, "reindex_media_seek_index", "media", req.GetMediaId(), map[string]any{
		"mode": run.Mode, "deep_verify": run.DeepVerify, "skip_backup": run.SkipBackup, "resync": run.Resync,
	})
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
	d.audit(ctx, "rollback_media_seek_index", "media", req.GetMediaId(), nil)
	return &eveyv1.RollbackMediaSeekIndexResponse{Accepted: true}, nil
}

func (d *adminMediaHandler) ReindexLibrarySeekIndex(ctx context.Context, req *eveyv1.ReindexLibrarySeekIndexRequest) (*eveyv1.ReindexLibrarySeekIndexResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ReindexLibrarySeekIndex")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	run := appmaintenance.ReindexRunOptions{
		Mode:          req.GetMode(),
		DeepVerify:    req.GetDeepVerify(),
		SkipBackup:    req.GetSkipBackup(),
		BackupWindow:  int(req.GetBackupWindow()),
		ResyncDamaged: req.GetResyncDamaged(),
	}
	// One entry point: "repair all" acts on every repairable verdict (index,
	// audio start delay, container) - include_audio is deprecated and ignored.
	if err := d.seekIndex.StartRepairLibrary(ctx, req.GetLibraryId(), run); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "reindex library seek index", err)
	}
	d.audit(ctx, "reindex_library_seek_index", "library", req.GetLibraryId(), map[string]any{
		"mode": run.Mode, "deep_verify": run.DeepVerify, "skip_backup": run.SkipBackup,
		"backup_window": run.BackupWindow, "resync_damaged": run.ResyncDamaged,
	})
	// Queued is deprecated but still WIRED: an old client reads it, and -1
	// is the agreed "poll GetSeekIndexJob instead" sentinel.
	return &eveyv1.ReindexLibrarySeekIndexResponse{Queued: -1}, nil //nolint:staticcheck // SA1019: deliberate, see above
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
	// Queued is deprecated but still WIRED: an old client reads it, and -1
	// is the agreed "poll GetSeekIndexJob instead" sentinel.
	d.audit(ctx, "rollback_library_seek_index", "library", req.GetLibraryId(), nil)
	return &eveyv1.RollbackLibrarySeekIndexResponse{Queued: -1}, nil //nolint:staticcheck // SA1019: deliberate, see above
}

// RemuxAudioMedia is RETIRED and refuses. It used to pull every audio track
// that started after the picture back onto it, by its own declared delay, with
// no upper bound - and a deliberate 12.000 s dub alignment looks exactly like a
// mux slip to that rule. On 2026-07-13 it rewrote 98 files that way, the
// backups were purged by the rolling window, and the amounts removed exist
// nowhere any more. An audio start offset is a fact about a file, and nothing
// here can prove which track to move: the file is measured and reported, and
// the RPC stays in the API only so a deployed client gets an answer instead of
// an unimplemented error.
func (d *adminMediaHandler) RemuxAudioMedia(ctx context.Context, _ *eveyv1.RemuxAudioMediaRequest) (*eveyv1.RemuxAudioMediaResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemuxAudioMedia")
	defer span.End()
	return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.seek_index_audio_offset_not_a_repair")
}

// RemuxAudioLibrary is RETIRED and refuses, for the same reason as
// RemuxAudioMedia - and it is the one that did the July damage in bulk.
func (d *adminMediaHandler) RemuxAudioLibrary(ctx context.Context, _ *eveyv1.RemuxAudioLibraryRequest) (*eveyv1.RemuxAudioLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RemuxAudioLibrary")
	defer span.End()
	return nil, d.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.seek_index_audio_offset_not_a_repair")
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
	d.audit(ctx, "delete_seek_index_backup", "media", req.GetMediaId(), nil)
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
	d.audit(ctx, "purge_seek_index_backups", "library", req.GetLibraryId(), nil)
	return &eveyv1.PurgeSeekIndexBackupsResponse{}, nil
}

// AckSeekIndexMedia records or withdraws the operator's acknowledgement of one
// file's verdict (synchronous: a cache update + persist).
func (d *adminMediaHandler) AckSeekIndexMedia(ctx context.Context, req *eveyv1.AckSeekIndexMediaRequest) (*eveyv1.AckSeekIndexMediaResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.AckSeekIndexMedia")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.AckMedia(ctx, req.GetMediaId(), req.GetAcked()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "ack seek index media", err)
	}
	return &eveyv1.AckSeekIndexMediaResponse{}, nil
}

// GetMediaRepairHistory returns what has been done to one file's bytes. Evey
// keeps no copy of a repaired file, so this journal is the only place that
// history exists - and the only way back.
func (d *adminMediaHandler) GetMediaRepairHistory(ctx context.Context, req *eveyv1.GetMediaRepairHistoryRequest) (*eveyv1.GetMediaRepairHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.GetMediaRepairHistory")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	recs, err := d.seekIndex.RepairHistoryOfMedia(ctx, req.GetMediaId(), int(req.GetLimit()))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "media repair history", err)
	}
	out := &eveyv1.GetMediaRepairHistoryResponse{Entries: make([]*eveyv1.MediaRepairEntry, 0, len(recs))}
	for _, r := range recs {
		out.Entries = append(out.Entries, repairEntryToProto(r))
	}
	return out, nil
}

// UndoMediaRepair replays the file's last reversible repair backwards.
func (d *adminMediaHandler) UndoMediaRepair(ctx context.Context, req *eveyv1.UndoMediaRepairRequest) (*eveyv1.UndoMediaRepairResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UndoMediaRepair")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	shift, err := d.seekIndex.UndoLastRepairOfMedia(ctx, req.GetMediaId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "undo media repair", err)
	}
	d.audit(ctx, "undo_media_repair", "media", req.GetMediaId(), map[string]any{"shift_sec": shift})
	return &eveyv1.UndoMediaRepairResponse{ShiftSec: shift}, nil
}

// repairEntryToProto keeps the wire shape flat: the UI shows a list of what
// happened, and computing "can this be undone" belongs to the domain, not to
// a client re-deriving it from an operation name.
func repairEntryToProto(r appmaintenance.RepairRecord) *eveyv1.MediaRepairEntry {
	return &eveyv1.MediaRepairEntry{
		AtMs:               r.Started.UnixMilli(),
		Op:                 string(r.Op),
		Outcome:            r.Outcome,
		Error:              r.Error,
		Job:                r.Job,
		EvidenceMethod:     r.Evidence.Method,
		EvidenceLagSec:     r.Evidence.LagSec,
		EvidenceConfidence: r.Evidence.Confidence,
		ShiftSec:           r.ShiftSec,
		AudibleBefore:      int32(r.AudibleBefore),
		AudibleAfter:       int32(r.AudibleAfter),
		SizeBefore:         r.SizeBefore,
		SizeAfter:          r.SizeAfter,
		Reversible:         r.Reversible(),
		MediaId:            r.MediaID,
		Path:               r.Path,
	}
}

// ListMediaRepairs lists the journal across the library: a window, newest
// first, optionally one batch kind. What a repair batch touched, as a screen
// instead of a query on file timestamps.
func (d *adminMediaHandler) ListMediaRepairs(ctx context.Context, req *eveyv1.ListMediaRepairsRequest) (*eveyv1.ListMediaRepairsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListMediaRepairs")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	q := appmaintenance.RepairQuery{Job: req.GetJob(), Limit: int(req.GetLimit())}
	if req.GetSinceMs() > 0 {
		q.Since = time.UnixMilli(req.GetSinceMs())
	}
	if req.GetUntilMs() > 0 {
		q.Until = time.UnixMilli(req.GetUntilMs())
	}
	recs, err := d.seekIndex.ListRepairs(ctx, q)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "list media repairs", err)
	}
	out := &eveyv1.ListMediaRepairsResponse{Entries: make([]*eveyv1.MediaRepairEntry, 0, len(recs))}
	for _, r := range recs {
		out.Entries = append(out.Entries, repairEntryToProto(r))
	}
	return out, nil
}

// ApplyAudioShift moves one audio track by the amount an operator judged on a
// preview clip.
func (d *adminMediaHandler) ApplyAudioShift(ctx context.Context, req *eveyv1.ApplyAudioShiftRequest) (*eveyv1.ApplyAudioShiftResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ApplyAudioShift")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.ApplyOperatorShift(ctx, req.GetMediaId(), req.GetTrack(), req.GetShiftSec()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "apply audio shift", err)
	}
	d.audit(ctx, "apply_audio_shift", "media", req.GetMediaId(), map[string]any{"track": req.GetTrack(), "shift_sec": req.GetShiftSec()})
	return &eveyv1.ApplyAudioShiftResponse{}, nil
}

// AckSeekIndexGroup acknowledges every pending file of one verdict in scope.
func (d *adminMediaHandler) AckSeekIndexGroup(ctx context.Context, req *eveyv1.AckSeekIndexGroupRequest) (*eveyv1.AckSeekIndexGroupResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.AckSeekIndexGroup")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	n, err := d.seekIndex.AckGroup(ctx, req.GetLibraryId(), appmaintenance.Verdict(req.GetVerdict()))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "ack seek index group", err)
	}
	return &eveyv1.AckSeekIndexGroupResponse{Acked: int32(n)}, nil //nolint:gosec // bounded by the library size
}

// FixContainerMedia renames one mislabelled .mkv to .mp4 in the background.
func (d *adminMediaHandler) FixContainerMedia(ctx context.Context, req *eveyv1.FixContainerMediaRequest) (*eveyv1.FixContainerMediaResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.FixContainerMedia")
	defer span.End()
	if err := d.seekIndexConfigured(ctx, span); err != nil {
		return nil, err
	}
	if err := d.seekIndex.StartFixContainerMedia(ctx, req.GetMediaId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, d.seekIndexError(ctx, "fix container media", err)
	}
	d.audit(ctx, "fix_container_media", "media", req.GetMediaId(), nil)
	return &eveyv1.FixContainerMediaResponse{Accepted: true}, nil
}
