package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/singleflight"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// remuxCacheFormat versions the on-disk remux cache. Bump it whenever the
// remuxed MP4 byte layout can change (e.g. a mkvgo mp4 output change across a
// version bump - the package is flagged experimental) so stale-format files are
// passed over and re-produced rather than served. Superseded files orphan and
// are reclaimed by the remux cache janitor.
//
// v2 (mkvgo 0.7.2): subtitle tracks now ride along - SRT/WebVTT as tx3g by
// default, ASS/SSA flattened to tx3g - so a v1 file (no subs) must be
// re-produced rather than served subtitle-less.
// v3 (mkvgo 0.38.0): a video track whose first two blocks share a timecode
// (interlaced fields stored one per sample) was timed on the audio lace grid
// and came out four times too slow; a v2 file may carry that.
const remuxCacheFormat = "remux-v3"

// remuxCacheName derives a collision-free, traversal-proof cache filename from
// the item identity, the cache-format version, and the SOURCE FINGERPRINT
// (size+mtime - the scanner's own change-detection identity, MediaFile.Fingerprint).
// Keying on the fingerprint is the correctness guarantee: a replaced or
// re-scanned source yields a NEW filename, so a stale remux is never served
// (the artwork-freeze class of bug, this time for video bytes). The sha256 hex
// has no path separators, so filepath.Join can't escape the cache dir whatever
// the id contains - that is the path-traversal guard, by construction.
func remuxCacheName(itemID, fingerprint string) string {
	sum := sha256.Sum256([]byte(itemID + "\x00" + remuxCacheFormat + "\x00" + fingerprint))
	return remuxCacheFormat + "-" + hex.EncodeToString(sum[:]) + ".mp4"
}

// Remuxer repackages an EBML (Matroska/WebM) source into a fast-start MP4
// WITHOUT re-encoding. Satisfied by the mkvgo driven adapter.
type Remuxer interface {
	// CanRemux reports whether the source at path can be repackaged (EBML only).
	CanRemux(path string) bool
	// RemuxToMP4 writes a fast-start (moov-first) MP4 at dstPath from srcPath.
	RemuxToMP4(ctx context.Context, srcPath, dstPath string) error
}

// remuxServe serves GET /api/v1/media/{id}/remux.mp4 - a progressive, seekable,
// fast-start MP4 repackaged once from a browser-incompatible EBML container
// (e.g. h264/aac in MKV that a browser can't demux). The companion to the HLS
// copy rung: same "copy, don't re-encode" idea, but a single self-contained
// file (download / cast / simple clients) instead of HLS segments. The result
// is cached on disk and reused; concurrent first-hit requests collapse to one
// remux via singleflight.
type remuxServe struct {
	leases        *apptranscoding.FileLeases
	libSvc        MediaItemGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	libLister     LibraryLister
	remuxer       Remuxer
	cacheDir      string // <transcoding cache>/remux
	baseURL       string
	group         singleflight.Group
}

func remuxHandler(libSvc MediaItemGetter, libraryAccess LibraryAccessChecker, parental ParentalChecker, libLister LibraryLister, remuxer Remuxer, cacheRoot, baseURL string, leases *apptranscoding.FileLeases) http.HandlerFunc {
	h := &remuxServe{
		leases:        leases,
		libSvc:        libSvc,
		libraryAccess: libraryAccess,
		parental:      parental,
		libLister:     libLister,
		remuxer:       remuxer,
		cacheDir:      filepath.Join(cacheRoot, "remux"),
		baseURL:       baseURL,
	}
	return h.handle
}

func (h *remuxServe) handle(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.Remux")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	// Mirror handleStream: an opaque stream token is scoped to a single media id.
	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "media item lookup failed")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return
	}
	if !checkPlayable(w, item) {
		span.SetStatus(codes.Error, "file needs remux")
		return
	}
	if item.File == nil || item.File.Path == "" {
		span.SetStatus(codes.Error, "media file missing")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// Same confinement as handleStream: a DB-poisoned path must not turn this
	// into an arbitrary-file read. Fail-open on unknown roots (no libLister).
	if h.libLister != nil && !pathWithinRoots(mediaRoots(r.Context(), h.libLister), item.File.Path) {
		span.SetStatus(codes.Error, "file outside library roots")
		slog.WarnContext(r.Context(), "remux: file path outside configured library roots",
			slog.String("media_id", mediaID), slog.String("path", item.File.Path))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// Only EBML sources can be repackaged. mp4 is already direct-played; other
	// containers (.avi, .ts, …) need a transcode, not a remux.
	if !h.remuxer.CanRemux(item.File.Path) {
		span.SetStatus(codes.Error, "source not remuxable")
		http.Error(w, `{"message":"unsupported source container"}`, http.StatusUnsupportedMediaType)
		return
	}

	// Source fingerprint (size+mtime) keys the cache so a replaced/re-scanned
	// file invalidates the remux. Prefer the scanner's stored Fingerprint (kept
	// in sync with stream probing); fall back to a runtime stat if it's unset so
	// the key still tracks the file version rather than collapsing to id-only.
	fingerprint := item.File.Fingerprint
	if fingerprint == "" {
		if fi, statErr := os.Stat(item.File.Path); statErr == nil {
			fingerprint = fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	outPath := filepath.Join(h.cacheDir, remuxCacheName(string(item.ID), fingerprint))
	if produceErr := h.ensureRemuxed(ctx, item.File.Path, outPath); produceErr != nil {
		span.RecordError(produceErr)
		span.SetStatus(codes.Error, "remux failed")
		slog.ErrorContext(r.Context(), "remux: produce failed", slog.String("media_id", mediaID), slog.Any("err", produceErr))
		http.Error(w, `{"message":"remux failed"}`, http.StatusInternalServerError)
		return
	}

	// Held while this range is read and a while after: the budget never
	// evicts the file between two ranges of a playing player.
	release := h.leases.Hold(outPath)
	defer release()

	f, err := os.Open(outPath)
	if err != nil {
		span.RecordError(err)
		logInternal(r.Context(), "remux: open output", err)
		http.Error(w, `{"message":"not found"}`, http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	// The cache budget judges this file by its mtime: a serve keeps it warm,
	// so a player (cast, offline audio) ranging through it is not evicted
	// under it as the oldest entry. Best effort - a failed touch only ages it.
	now := time.Now()
	//nolint:errcheck // best-effort touch; a failure only ages the file in the budget's LRU
	_ = os.Chtimes(outPath, now, now)

	setCORSHeaders(w, r, h.baseURL)
	w.Header().Set("Content-Type", "video/mp4")
	// The validator is the cache name (source fingerprint), not the mtime the
	// touch moves: an If-Range against a moving Last-Modified would turn every
	// range into a full 200.
	w.Header().Set("ETag", `"`+strings.TrimSuffix(filepath.Base(outPath), ".mp4")+`"`)
	// ServeContent handles Range requests, conditional GETs and seeking - the
	// fast-start (moov-first) layout lets a player start without reading to EOF.
	http.ServeContent(w, r, mediaID+".mp4", time.Time{}, f)
}

// ensureRemuxed guarantees outPath holds a complete remuxed MP4, producing it
// at most once. The on-disk file IS the cache: a present non-empty file is
// reused as-is. Concurrent first-hit requests collapse to a single producer via
// singleflight, and the remux runs on a context detached from the triggering
// request so one client disconnecting mid-remux doesn't abort the work the
// others are blocked on. Publication is atomic (temp + rename) so a half-written
// file is never served.
func (h *remuxServe) ensureRemuxed(ctx context.Context, srcPath, outPath string) error {
	if fi, err := os.Stat(outPath); err == nil && fi.Size() > 0 {
		return nil
	}
	_, err, _ := h.group.Do(outPath, func() (any, error) {
		// Re-check under the single-flight: a prior winner may have produced it
		// while we queued.
		if fi, statErr := os.Stat(outPath); statErr == nil && fi.Size() > 0 {
			return nil, nil
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return nil, err
		}
		tmp := outPath + ".tmp"
		bestEffortRemove(tmp)
		if err := h.remuxer.RemuxToMP4(context.WithoutCancel(ctx), srcPath, tmp); err != nil {
			bestEffortRemove(tmp)
			return nil, err
		}
		if err := os.Rename(tmp, outPath); err != nil {
			bestEffortRemove(tmp)
			return nil, err
		}
		return nil, nil
	})
	return err
}

// bestEffortRemove deletes path, tolerating "already gone". Used on cleanup
// paths where the caller is already returning the real error, so a failed
// cleanup is logged but not surfaced.
func bestEffortRemove(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("remux: temp cleanup failed", slog.String("path", path), slog.Any("err", err))
	}
}
