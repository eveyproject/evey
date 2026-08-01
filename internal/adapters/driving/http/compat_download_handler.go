package httpapi

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
	"sync"

	"strings"

	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/singleflight"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// compatCacheFormat versions the on-disk compat-download cache. Bump it when
// the produced bytes can change shape (audio codec/bitrate choice, muxer
// flags) so stale-format files are re-produced, not served. Orphans are
// reclaimed by the cache janitor, exactly like the remux cache.
const compatCacheFormat = "compat-v1"

// compatCacheName mirrors remuxCacheName: identity keyed on the item, the
// format version and the SOURCE FINGERPRINT (a replaced file yields a new
// name, so a stale variant is never served), hex-encoded so filepath.Join
// cannot escape the cache dir whatever the id contains.
func compatCacheName(itemID, fingerprint string) string {
	sum := sha256.Sum256([]byte(itemID + "\x00" + compatCacheFormat + "\x00" + fingerprint))
	return compatCacheFormat + "-" + hex.EncodeToString(sum[:]) + ".mkv"
}

// CompatTranscoder produces the phone-compatible variant of a media file
// (video/subs copied, audio re-encoded to stereo AAC). Satisfied by the
// ffmpeg driven adapter.
type CompatTranscoder interface {
	TranscodeCompat(ctx context.Context, srcPath, dstPath string) error
}

// compatJob is the in-memory state of one production. "ready" is not a state
// here on purpose: the on-disk file IS the ready signal (it survives restarts,
// this map does not).
type compatJob struct {
	failed bool
	errMsg string
}

// compatServe implements the compat-download trio:
//
//	POST /api/v1/media/{id}/compat-download         -> start (or reuse) the production, 202
//	GET  /api/v1/media/{id}/compat-download/status  -> {"status": preparing|ready|failed|absent}
//	GET  /api/v1/media/{id}/compat-download         -> serve the finished MKV (Range OK)
//
// The split exists because the production is MINUTES of ffmpeg on the weak-NAS
// target - a blocking GET (the remux.mp4 shape) would sit on an open request
// past every proxy timeout. The client polls status, then downloads through
// the same guard set as the raw download.
type compatServe struct {
	libSvc        MediaItemGetter
	streams       StreamsByMediaGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	libLister     LibraryLister
	transcoder    CompatTranscoder
	cacheDir      string // <transcoding cache>/compat
	baseURL       string
	group         singleflight.Group

	mu   sync.Mutex
	jobs map[string]*compatJob // keyed by outPath
}

// StreamsByMediaGetter is the one method of MediaStreamsGetter this handler
// needs (the "is a transcode even necessary" check below).
type StreamsByMediaGetter interface {
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}

func newCompatServe(libSvc MediaItemGetter, streams StreamsByMediaGetter, libraryAccess LibraryAccessChecker, parental ParentalChecker, libLister LibraryLister, transcoder CompatTranscoder, cacheRoot, baseURL string) *compatServe {
	bgtasks.Register("compat_download")
	return &compatServe{
		libSvc:        libSvc,
		streams:       streams,
		libraryAccess: libraryAccess,
		parental:      parental,
		libLister:     libLister,
		transcoder:    transcoder,
		cacheDir:      filepath.Join(cacheRoot, "compat"),
		baseURL:       baseURL,
		jobs:          map[string]*compatJob{},
	}
}

// transcodeUnnecessary decides whether a production would be pure waste: the
// client DECLARES the audio codecs it can render (?renderable=aac,mp3,...),
// and if the source already carries a track the client can play - or carries
// no audio at all - the server refuses to burn minutes of NAS CPU on it. The
// invariant is the owner's: the transcode must run when and ONLY when the
// device says it cannot play the file as it is. No declaration = the caller
// takes responsibility (our client always declares).
func (h *compatServe) transcodeUnnecessary(ctx context.Context, mediaID, renderableParam string) bool {
	if renderableParam == "" || h.streams == nil {
		return false
	}
	renderable := map[string]bool{}
	for _, c := range strings.Split(renderableParam, ",") {
		if c = strings.ToLower(strings.TrimSpace(c)); c != "" {
			renderable[c] = true
		}
	}
	if len(renderable) == 0 {
		return false
	}
	streams, err := h.streams.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return false // unknowable is not "unnecessary" - let the production decide
	}
	hasAudio := false
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		hasAudio = true
		if renderable[strings.ToLower(s.Codec)] {
			return true
		}
	}
	return !hasAudio
}

// resolve runs the shared guard set (the same ones the raw download enforces:
// token scope, library access, parental) and returns the item plus its cache
// path. A nil item means the response has already been written.
func (h *compatServe) resolve(w http.ResponseWriter, r *http.Request) (mediaID, srcPath, outPath string, ok bool) {
	mediaID = r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	if st, tok := StreamTokenFromContext(r.Context()); tok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return "", "", "", false
		}
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return "", "", "", false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return "", "", "", false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return "", "", "", false
	}
	if item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return "", "", "", false
	}
	if h.libLister != nil && !pathWithinRoots(mediaRoots(r.Context(), h.libLister), item.File.Path) {
		slog.WarnContext(r.Context(), "compat-download: file path outside configured library roots",
			slog.String("media_id", mediaID), slog.String("path", item.File.Path))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return "", "", "", false
	}

	fingerprint := item.File.Fingerprint
	if fingerprint == "" {
		if fi, statErr := os.Stat(item.File.Path); statErr == nil {
			fingerprint = fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return mediaID, item.File.Path, filepath.Join(h.cacheDir, compatCacheName(string(item.ID), fingerprint)), true
}

func (h *compatServe) status(outPath string) (string, string) {
	if fi, err := os.Stat(outPath); err == nil && fi.Size() > 0 {
		return "ready", ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if job, ok := h.jobs[outPath]; ok {
		if job.failed {
			return "failed", job.errMsg
		}
		return "preparing", ""
	}
	return "absent", ""
}

func writeCompatStatus(w http.ResponseWriter, code int, status, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": status,
		"error":  errMsg,
	})
}

// handlePrepare starts (or reports) the production. Idempotent: re-POSTing a
// preparing item answers 202, a ready one 200, and a failed one clears the
// failure and tries again (the operator fixed the disk, the client retries).
func (h *compatServe) handlePrepare(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.CompatPrepare")
	defer span.End()
	r = r.WithContext(ctx)

	mediaID, srcPath, outPath, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	if h.transcodeUnnecessary(r.Context(), mediaID, r.URL.Query().Get("renderable")) {
		writeCompatStatus(w, http.StatusOK, "unnecessary", "")
		return
	}
	switch st, _ := h.status(outPath); st {
	case "ready":
		writeCompatStatus(w, http.StatusOK, "ready", "")
		return
	case "preparing":
		writeCompatStatus(w, http.StatusAccepted, "preparing", "")
		return
	}

	h.mu.Lock()
	h.jobs[outPath] = &compatJob{}
	h.mu.Unlock()
	slog.InfoContext(r.Context(), "compat-download: production started",
		slog.String("media_id", mediaID), slog.String("src", srcPath))

	// Detached from the request on purpose: the phone that asked may drop off
	// Wi-Fi mid-production; the next poller still wants the file.
	go h.produce(context.WithoutCancel(ctx), mediaID, srcPath, outPath)
	writeCompatStatus(w, http.StatusAccepted, "preparing", "")
}

func (h *compatServe) produce(ctx context.Context, mediaID, srcPath, outPath string) {
	done := bgtasks.Begin("compat_download", mediaID)
	defer done()
	_, err, _ := h.group.Do(outPath, func() (any, error) {
		if fi, statErr := os.Stat(outPath); statErr == nil && fi.Size() > 0 {
			return nil, nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0o755); mkErr != nil {
			return nil, mkErr
		}
		tmp := outPath + ".tmp"
		bestEffortRemove(tmp)
		if tErr := h.transcoder.TranscodeCompat(ctx, srcPath, tmp); tErr != nil {
			bestEffortRemove(tmp)
			return nil, tErr
		}
		return nil, os.Rename(tmp, outPath)
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		slog.ErrorContext(ctx, "compat-download: production failed",
			slog.String("media_id", mediaID), slog.Any("err", err))
		h.jobs[outPath] = &compatJob{failed: true, errMsg: err.Error()}
		return
	}
	delete(h.jobs, outPath) // the file on disk is the durable "ready"
	slog.InfoContext(ctx, "compat-download: production finished",
		slog.String("media_id", mediaID), slog.String("path", outPath))
}

func (h *compatServe) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.CompatStatus")
	defer span.End()
	r = r.WithContext(ctx)

	_, _, outPath, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	st, errMsg := h.status(outPath)
	writeCompatStatus(w, http.StatusOK, st, errMsg)
}

// handleServe streams the finished variant with the raw download's serving
// contract: attachment disposition, Range support, and the same served/refused
// observability (a compat download that never logs is the invisible-refusal
// class all over again).
func (h *compatServe) handleServe(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.CompatServe")
	defer span.End()
	r = r.WithContext(ctx)

	mediaID, _, outPath, ok := h.resolve(w, r)
	if !ok {
		span.SetStatus(codes.Error, "refused")
		return
	}
	st, errMsg := h.status(outPath)
	if st != "ready" {
		span.SetStatus(codes.Error, "not ready")
		writeCompatStatus(w, http.StatusConflict, st, errMsg)
		return
	}
	f, err := os.Open(outPath)
	if err != nil {
		span.RecordError(err)
		http.Error(w, `{"message":"error"}`, http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		span.RecordError(err)
		http.Error(w, `{"message":"error"}`, http.StatusInternalServerError)
		return
	}

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	name := mediaID + ".mkv"
	if err == nil && item.Title != "" {
		name = sanitizeFilename(item.Title) + ".mkv"
	}
	setCORSHeaders(w, r, h.baseURL)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	slog.InfoContext(r.Context(), "download served",
		slog.String("component", "download"),
		slog.String("media_id", mediaID),
		slog.String("filename", name),
		slog.Bool("compat", true),
		slog.Int64("bytes", fi.Size()),
		slog.String("range", r.Header.Get("Range")),
		slog.String("client_ip", ratelimit.ClientIP(r)))
	http.ServeContent(w, r, name, fi.ModTime(), f)
}
