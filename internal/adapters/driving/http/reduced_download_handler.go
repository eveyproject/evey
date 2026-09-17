package httpapi

// reduced_download_handler.go - the size-reduced (720p) download, served in
// slices.
//
// A 4K remux does not fit on a phone. The owner's constraints are no second
// permanent copy anywhere and a final file REASSEMBLED ON THE DEVICE, so this
// endpoint never publishes a whole reduced file the way compat-download does.
// It runs one long ffmpeg that emits consecutive Matroska slices, hands them
// over one at a time, and drops each slice as soon as the phone moves past it.
// The phone joins them locally (mkvgo ops.Join) into the file it keeps.
//
// The shape is deliberately NOT compat-download's:
//
//   - compat re-encodes audio only (~10-30x realtime, minutes). This
//     re-encodes VIDEO, near realtime on the weak-NAS target: a two-hour film
//     can mean an hour of preparation. A single blocking artefact at the end
//     would mean an hour of exposure to one dropped connection.
//   - slice-at-a-time delivery makes resume cost one slice instead of the
//     whole film, and caps what the server holds to what the phone has not
//     collected yet.
//
//	GET    /api/v1/media/{id}/reduced-download/plan       -> is it worth offering, and how big
//	POST   /api/v1/media/{id}/reduced-download            -> start (or resume) the production
//	GET    /api/v1/media/{id}/reduced-download/status     -> {status, permille, slicesReady, slices}
//	GET    /api/v1/media/{id}/reduced-download/slice/{n}  -> slice n (Range OK); implicitly releases < n
//	DELETE /api/v1/media/{id}/reduced-download            -> the phone is done (or gave up): drop everything

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/singleflight"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/hdr"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/reduceddl"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// reducedJobFormat versions the on-disk job layout. Bump it when the produced
// bytes can change shape (target height, codec, slice length) so a half-
// collected job from an older build is never mixed with new slices.
//
// v2: the stereo fold got its matrix headroom (see
// domain/transcoding/audio_downmix.go). Beyond serving a clipped file, a
// resumed v1 job would join pre-fix slices onto post-fix ones and step 4 dB at
// the seam.
const reducedJobFormat = "reduced-v2"

// reducedIdleTTL is how long an untouched job dir survives. A phone that
// starts a production and never comes back (uninstalled, gave up, out of
// space) must not pin gigabytes forever - and unlike the compat cache there is
// nothing to keep warm here: the slices are consumed once, by one device.
const reducedIdleTTL = 24 * time.Hour

// reducedSweepInterval paces the idle sweep.
const reducedSweepInterval = 30 * time.Minute

// ReducedTranscoder produces the reduced variant as slices in a job dir.
// Satisfied by the ffmpeg driven adapter.
type ReducedTranscoder interface {
	// TranscodeReducedSlices returns when ffmpeg exits. Slices become
	// available WHILE it runs - that is the whole point - so the caller reads
	// readiness from the job dir, not from this returning.
	TranscodeReducedSlices(ctx context.Context, srcPath, dir string, spec reduceddl.Spec, durationMs, estBytes int64, onProgress func(permille int)) error
}

type reducedJob struct {
	running bool
	failed  bool
	errMsg  string
}

type reducedServe struct {
	libSvc        MediaItemGetter
	streams       StreamsByMediaGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	libLister     LibraryLister
	transcoder    ReducedTranscoder
	cacheDir      string // <transcoding cache>/reduced
	baseURL       string
	// offerAboveBytes is the operator's threshold: a source at or below it is
	// not worth offering a reduced copy for. The phone ORs this with its own
	// "it does not fit on this device" trigger, which no server can know.
	offerAboveBytes int64

	group singleflight.Group

	mu       sync.Mutex
	jobs     map[string]*reducedJob // keyed by job dir
	progress map[string]int         // production permille, keyed by job dir
}

func newReducedServe(libSvc MediaItemGetter, streams StreamsByMediaGetter, libraryAccess LibraryAccessChecker, parental ParentalChecker, libLister LibraryLister, transcoder ReducedTranscoder, cacheRoot, baseURL string, offerAboveBytes int64) *reducedServe {
	bgtasks.Register("reduced_download")
	return &reducedServe{
		libSvc:          libSvc,
		streams:         streams,
		libraryAccess:   libraryAccess,
		parental:        parental,
		libLister:       libLister,
		transcoder:      transcoder,
		cacheDir:        filepath.Join(cacheRoot, "reduced"),
		baseURL:         baseURL,
		offerAboveBytes: offerAboveBytes,
		jobs:            map[string]*reducedJob{},
		progress:        map[string]int{},
	}
}

// reducedJobDirName keys the job on the item, the layout version, the
// SOURCE FINGERPRINT and the CONSUMER, hex-encoded so filepath.Join cannot
// escape the cache dir whatever the id contains. Same contract as the compat
// cache: a replaced file yields a new dir, so half a stale film is never joined
// onto a fresh one. The consumer is in the key because slices are deleted as
// they are acked: two phones on one job would eat each other's bytes.
func reducedJobDirName(itemID, fingerprint, consumer string) string {
	sum := sha256.Sum256([]byte(itemID + "\x00" + reducedJobFormat + "\x00" + fingerprint + "\x00" + consumer))
	return reducedJobFormat + "-" + hex.EncodeToString(sum[:])
}

// reducedConsumer identifies who a job belongs to: the profile, else the user.
func reducedConsumer(claims *domainuser.Claims) string {
	if claims == nil {
		return ""
	}
	if claims.ProfileID != "" {
		return "p:" + claims.ProfileID
	}
	return "u:" + claims.UserID
}

// reducedTarget is everything resolve() digs out of the item: where to read,
// where to write, and what the encode should look like.
type reducedTarget struct {
	mediaID    string
	srcPath    string
	jobDir     string
	durationMs int64
	sourceSize int64
	spec       reduceddl.Spec
	// eligible is false when a reduced variant makes no sense for this source
	// (already 720p or smaller, dimensions unknown). Nothing is produced then.
	eligible bool
}

// resolve runs the same guard set as the raw download (token scope, library
// access, parental, path-within-roots) and works out the encode spec. ok=false
// means the response has already been written.
func (h *reducedServe) resolve(w http.ResponseWriter, r *http.Request) (reducedTarget, bool) {
	var t reducedTarget
	t.mediaID = r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	if st, tok := StreamTokenFromContext(r.Context()); tok {
		if st.MediaItemID != shared.MediaItemID(t.mediaID) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return t, false
		}
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), t.mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return t, false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return t, false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return t, false
	}
	if !checkPlayable(w, item) {
		return t, false
	}
	if item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return t, false
	}
	if h.libLister != nil && !pathWithinRoots(mediaRoots(r.Context(), h.libLister), item.File.Path) {
		slog.WarnContext(r.Context(), "reduced-download: file path outside configured library roots",
			slog.String("media_id", t.mediaID), slog.String("path", item.File.Path))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return t, false
	}

	fingerprint := item.File.Fingerprint
	if fingerprint == "" {
		if fi, statErr := os.Stat(item.File.Path); statErr == nil {
			fingerprint = fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	t.srcPath = item.File.Path
	t.sourceSize = item.File.Size
	t.durationMs = item.File.Duration.Milliseconds()
	t.jobDir = filepath.Join(h.cacheDir, reducedJobDirName(string(item.ID), fingerprint, reducedConsumer(claims)))
	t.spec = reduceddl.Spec{SliceSec: reduceddl.SliceSec}

	width, toneMap, src := h.videoShape(r.Context(), t.mediaID)
	t.spec.Width = width
	t.spec.ToneMap = toneMap
	t.spec.SourceWidth, t.spec.SourceHeight, t.spec.SourceTenBit = src.Width, src.Height, src.TenBit
	t.eligible = width > 0 && t.durationMs > 0
	return t, true
}

// videoShape reads the delivery width (0 = not worth reducing) and whether the
// source needs HDR->SDR grading off the item's probed streams. An unprobed or
// unreadable item yields 0: refusing to offer beats offering a variant we
// cannot size or grade.
func (h *reducedServe) videoShape(ctx context.Context, mediaID string) (width int, toneMap bool, src procfork.Source) {
	if h.streams == nil {
		return 0, false, src
	}
	streams, err := h.streams.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return 0, false, src
	}
	hdrStreams := make([]hdr.VideoStream, 0, len(streams))
	for _, s := range streams {
		if s == nil {
			continue
		}
		hdrStreams = append(hdrStreams, s)
		if s.StreamType == domainlibrary.Video && width == 0 && s.Width != nil && s.Height != nil {
			width = reduceddl.WidthFor(*s.Width, *s.Height)
			src = procfork.Source{Width: *s.Width, Height: *s.Height, TenBit: s.BitDepth != nil && *s.BitDepth > 8}
		}
	}
	return width, hdr.DetectHDR(hdrStreams).IsHDR, src
}

// --- readiness -------------------------------------------------------------

// slicesReady is how many leading slices ffmpeg has finished writing, plus the
// source time the last of them ends at (where a resume must pick up). Read off
// the segment muxer's list files: each list names the slice index its run
// started at, so a job resumed after a failure still reports against the whole
// film - the answer is max(start + closed) across the runs.
//
// This is deliberately NOT "count the .mkv files in the dir": a slice file
// exists while it is still being written, and consumed slices are deleted.
func slicesReady(jobDir string) (ready int, endSec float64) {
	entries, err := os.ReadDir(jobDir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		start, ok := reduceddl.ListStart(e.Name())
		if !ok {
			continue
		}
		f, err := os.Open(filepath.Join(jobDir, e.Name()))
		if err != nil {
			continue
		}
		n, end := reduceddl.ClosedSlices(f)
		_ = f.Close()
		if start+n > ready {
			ready, endSec = start+n, end
		}
	}
	return ready, endSec
}

// status collapses the job into what the client polls on. "ready" means every
// slice has been produced; the phone can and should start collecting long
// before that (slicesReady > 0).
func (h *reducedServe) status(jobDir string, totalSlices int) (state string, errMsg string, permille, ready int, endSec float64) {
	ready, endSec = slicesReady(jobDir)

	h.mu.Lock()
	defer h.mu.Unlock()
	job, tracked := h.jobs[jobDir]
	switch {
	case totalSlices > 0 && ready >= totalSlices:
		return "ready", "", 1000, ready, endSec
	case tracked && job.failed:
		return "failed", job.errMsg, h.progress[jobDir], ready, endSec
	case tracked && job.running:
		return "preparing", "", h.progress[jobDir], ready, endSec
	case ready > 0:
		// Slices on disk, nothing running: a production interrupted by a
		// restart. Not "absent" (there IS collectable work) and not "failed"
		// (nothing went wrong) - the client re-POSTs and the resume picks up.
		return "interrupted", "", h.progress[jobDir], ready, endSec
	}
	return "absent", "", 0, ready, endSec
}

func writeReducedStatus(w http.ResponseWriter, code int, state, errMsg string, permille, ready, total int, durationMs int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      state,
		"error":       errMsg,
		"permille":    permille,
		"slicesReady": ready,
		"slices":      total,
		"sliceSec":    reduceddl.SliceSec,
		// The joined file's header has to be stamped with the real length by
		// the client: mkvgo's Join copies the FIRST slice's duration into the
		// output, so an unrepaired download announces ten minutes and refuses
		// to seek past it. The client cannot compute this - only the server
		// knows the source length - so it travels with every status.
		"durationMs": durationMs,
	})
}

// --- handlers --------------------------------------------------------------

// handlePlan answers "should the app offer this user a lighter copy, and how
// big would it be" WITHOUT starting anything. The phone calls it before the
// download starts; a 404 (routes unregistered, operator turned the feature
// off) is a complete answer meaning "do not offer".
func (h *reducedServe) handlePlan(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ReducedPlan")
	defer span.End()
	r = r.WithContext(ctx)

	t, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	estimated := reduceddl.EstimatedBytes(t.durationMs)
	// Only offer when the reduced copy is actually smaller. A long, already
	// low-bitrate file can estimate ABOVE its source, and offering the user a
	// "lighter" version that weighs more is worse than not offering at all.
	worthIt := t.eligible && estimated > 0 && estimated < t.sourceSize
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"available": worthIt,
		// offer is the SERVER's opinion (the operator's size threshold). The
		// phone ORs it with "it does not fit here", which no server can know.
		"offer":          worthIt && h.offerAboveBytes > 0 && t.sourceSize > h.offerAboveBytes,
		"height":         reduceddl.TargetHeight,
		"width":          t.spec.Width,
		"estimatedBytes": estimated,
		"sourceBytes":    t.sourceSize,
		"slices":         reduceddl.SliceCount(t.durationMs),
		"durationMs":     t.durationMs,
		"sliceSec":       reduceddl.SliceSec,
	})
}

// handlePrepare starts the production, or resumes one a restart interrupted.
// Idempotent: a running job answers 202, a finished one 200, a failed one is
// cleared and retried (the operator freed the disk, the client asks again).
func (h *reducedServe) handlePrepare(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ReducedPrepare")
	defer span.End()
	r = r.WithContext(ctx)

	t, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	total := reduceddl.SliceCount(t.durationMs)
	if !t.eligible {
		// Not an error: the source is already small enough (or unprobed). The
		// client downloads the original, which is what it would have done.
		writeReducedStatus(w, http.StatusOK, "unnecessary", "", 0, 0, total, t.durationMs)
		return
	}

	switch state, _, permille, ready, endSec := h.status(t.jobDir, total); state {
	case "ready":
		writeReducedStatus(w, http.StatusOK, "ready", "", 1000, ready, total, t.durationMs)
		return
	case "preparing":
		writeReducedStatus(w, http.StatusAccepted, "preparing", "", permille, ready, total, t.durationMs)
		return
	default:
		// absent / failed / interrupted all start a run below; "interrupted"
		// and a retried "failed" resume from what is already on disk.
		t.spec.StartSlice = ready
		t.spec.StartSec = endSec
	}

	h.mu.Lock()
	h.jobs[t.jobDir] = &reducedJob{running: true}
	h.mu.Unlock()
	slog.InfoContext(r.Context(), "reduced-download: production started",
		slog.String("media_id", t.mediaID),
		slog.String("src", t.srcPath),
		slog.Int("start_slice", t.spec.StartSlice),
		slog.Int("slices", total),
		slog.Int("width", t.spec.Width),
		slog.Bool("tonemap", t.spec.ToneMap))

	// Detached from the request: the phone may drop off Wi-Fi mid-production,
	// and an hour of encoding must not die with its HTTP request.
	go h.produce(context.WithoutCancel(ctx), t)
	writeReducedStatus(w, http.StatusAccepted, "preparing", "", 0, t.spec.StartSlice, total, t.durationMs)
}

func (h *reducedServe) produce(ctx context.Context, t reducedTarget) {
	done := bgtasks.Begin("reduced_download", t.mediaID)
	defer done()
	_, err, _ := h.group.Do(t.jobDir, func() (any, error) {
		onProgress := func(permille int) {
			h.mu.Lock()
			h.progress[t.jobDir] = permille
			h.mu.Unlock()
		}
		return nil, h.transcoder.TranscodeReducedSlices(ctx, t.srcPath, t.jobDir, t.spec,
			t.durationMs, reduceddl.EstimatedBytes(t.durationMs), onProgress)
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		slog.ErrorContext(ctx, "reduced-download: production failed",
			slog.String("media_id", t.mediaID), slog.Any("err", err))
		h.jobs[t.jobDir] = &reducedJob{failed: true, errMsg: err.Error()}
		return
	}
	delete(h.jobs, t.jobDir) // the slice list on disk is the durable "ready"
	delete(h.progress, t.jobDir)
	slog.InfoContext(ctx, "reduced-download: production finished",
		slog.String("media_id", t.mediaID), slog.String("dir", t.jobDir))
}

func (h *reducedServe) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ReducedStatus")
	defer span.End()
	r = r.WithContext(ctx)

	t, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	total := reduceddl.SliceCount(t.durationMs)
	state, errMsg, permille, ready, _ := h.status(t.jobDir, total)
	writeReducedStatus(w, http.StatusOK, state, errMsg, permille, ready, total, t.durationMs)
}

// handleSlice serves one slice under the raw download's contract (Range,
// attachment disposition, the same served/refused observability).
//
// Fetching slice n is also the ACK for everything before it: the phone only
// asks for n once n-1 is joined into its local file, so the earlier slices are
// released here. That is what keeps the server's footprint to the tail the
// phone has not collected, instead of a second copy of the film.
func (h *reducedServe) handleSlice(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ReducedSlice")
	defer span.End()
	r = r.WithContext(ctx)

	t, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	idx, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || idx < 0 {
		http.Error(w, `{"message":"bad slice index"}`, http.StatusBadRequest)
		return
	}
	total := reduceddl.SliceCount(t.durationMs)
	state, errMsg, permille, ready, _ := h.status(t.jobDir, total)
	if idx >= ready {
		// Not produced yet (or never will be). 409 + the live status is the
		// same "come back later" contract the compat serve uses.
		span.SetStatus(codes.Error, "slice not ready")
		writeReducedStatus(w, http.StatusConflict, state, errMsg, permille, ready, total, t.durationMs)
		return
	}

	path := filepath.Join(t.jobDir, reduceddl.SliceName(idx))
	f, err := os.Open(path)
	if err != nil {
		// Produced but already released: the client is re-asking for a slice
		// it acknowledged by moving on. 410 says "this one is gone for good",
		// which a client can tell apart from "not yet" (409).
		span.SetStatus(codes.Error, "slice released")
		writeReducedStatus(w, http.StatusGone, state, errMsg, permille, ready, total, t.durationMs)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		span.RecordError(err)
		logInternal(r.Context(), "reduced-download: stat slice", err)
		http.Error(w, `{"message":"error"}`, http.StatusInternalServerError)
		return
	}

	name := fmt.Sprintf("%s-%s", t.mediaID, reduceddl.SliceName(idx))
	setCORSHeaders(w, r, h.baseURL)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	slog.InfoContext(r.Context(), "download served",
		slog.String("component", "download"),
		slog.String("media_id", t.mediaID),
		slog.String("filename", name),
		slog.Bool("reduced", true),
		slog.Int("slice", idx),
		slog.Int64("bytes", fi.Size()),
		slog.String("range", r.Header.Get("Range")),
		slog.String("client_ip", ratelimit.ClientIP(r)))
	http.ServeContent(w, r, name, fi.ModTime(), f)

	h.releaseBelow(t.jobDir, idx)
}

// releaseBelow deletes the slices the phone has demonstrably consumed. Only
// the slice FILES go: the list files stay, because they are what lets
// slicesReady keep answering after the bytes are gone.
func (h *reducedServe) releaseBelow(jobDir string, idx int) {
	for i := range idx {
		p := filepath.Join(jobDir, reduceddl.SliceName(i))
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			slog.Warn("reduced-download: releasing a consumed slice failed",
				slog.String("path", p), slog.Any("err", err))
		}
	}
}

// handleDiscard is the phone saying it is done - joined every slice, or gave
// up. Without it an abandoned job waits out reducedIdleTTL holding the tail of
// a film; with it the disk comes back immediately.
func (h *reducedServe) handleDiscard(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ReducedDiscard")
	defer span.End()
	r = r.WithContext(ctx)

	t, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	h.mu.Lock()
	running := h.jobs[t.jobDir] != nil && h.jobs[t.jobDir].running
	h.mu.Unlock()
	if running {
		// Deleting the dir under a live ffmpeg would have it writing into
		// unlinked files for the rest of the hour. Let the idle sweep collect
		// it once the process is gone.
		ready, _ := slicesReady(t.jobDir)
		writeReducedStatus(w, http.StatusConflict, "preparing", "", 0, ready, reduceddl.SliceCount(t.durationMs), t.durationMs)
		return
	}
	if err := os.RemoveAll(t.jobDir); err != nil {
		span.RecordError(err)
		logInternal(r.Context(), "reduced-download: remove job dir", err)
		http.Error(w, `{"message":"error"}`, http.StatusInternalServerError)
		return
	}
	h.mu.Lock()
	delete(h.jobs, t.jobDir)
	delete(h.progress, t.jobDir)
	h.mu.Unlock()
	slog.InfoContext(r.Context(), "reduced-download: job discarded",
		slog.String("media_id", t.mediaID), slog.String("dir", t.jobDir))
	w.WriteHeader(http.StatusNoContent)
}

// --- idle sweep ------------------------------------------------------------

// startIdleSweep reclaims job dirs nobody has touched for reducedIdleTTL. The
// normal path releases slices as they are consumed and handleDiscard drops the
// rest, so this only ever catches phones that vanished mid-download.
func (h *reducedServe) startIdleSweep(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(reducedSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// One bad sweep must never take the server down nor stop the
				// next one, same posture as every sibling background worker.
				func() {
					defer func() {
						if rec := recover(); rec != nil {
							slog.Error("panic in reduced-download idle sweep", slog.Any("panic", rec))
						}
					}()
					h.sweepIdle()
				}()
			}
		}
	}()
}

func (h *reducedServe) sweepIdle() {
	entries, err := os.ReadDir(h.cacheDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-reducedIdleTTL)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(h.cacheDir, e.Name())
		h.mu.Lock()
		job := h.jobs[dir]
		h.mu.Unlock()
		if job != nil && job.running {
			continue // ffmpeg is writing in there
		}
		// The dir's own mtime moves whenever a slice lands or is released, so
		// it tracks activity from both sides (producer and consumer).
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("reduced-download: idle sweep remove failed", slog.String("dir", dir), slog.Any("err", err))
			continue
		}
		h.mu.Lock()
		delete(h.jobs, dir)
		delete(h.progress, dir)
		h.mu.Unlock()
		slog.Info("reduced-download: idle job reclaimed", slog.String("dir", dir))
	}
}
