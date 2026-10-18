package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/singleflight"

	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	pkgcache "github.com/eveyproject/evey/internal/pkg/cache"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"github.com/eveyproject/evey/internal/pkg/urlvalidator"
)

// artworkResizeTimeout bounds the ffmpeg call for a single poster resize.
const artworkResizeTimeout = 10 * time.Second

// artworkResizeGroup collapses concurrent requests for the SAME resized
// variant, OR the same full-resolution WebP conversion (see resolveWebP /
// ensureArtworkWebP below), into one ffmpeg invocation. A poster grid loading
// dozens of cold thumbnails, or two tabs open on the same title, otherwise
// raced two ffmpeg processes to write the identical dstPath - wasted work,
// and a torn file if one lost the race mid-write. Same motif as the remux
// handler's own singleflight.Group (remux_handler.go): keyed on the output
// path, winner's error is shared with waiters but never cached, so the next
// request retries. The two flows share this one group rather than each
// getting their own - see ensureArtworkWebP for why their keys still can't
// collide.
var artworkResizeGroup singleflight.Group

// artworkResizeMaxConcurrent bounds how many ffmpeg resize OR full-resolution
// WebP conversion processes run at once, across ALL in-flight artwork
// requests. The NAS this project targets can barely sustain the operator's
// configured HLS session count (operators are told to floor
// TranscodingConfig.MaxSessions at 1-2 on the weakest hardware, see
// config.go) - a cold poster grid must not pile a dozen extra ffmpeg
// processes on top of that and starve the transcode an actual viewer is
// waiting on. 2 matches that same floor: enough to keep a grid moving, low
// enough that it can never itself become the bottleneck. Both flows share
// this one budget - a full-res WebP conversion is exactly as expensive as a
// resize for this purpose, so there's no reason to give them separate caps.
const artworkResizeMaxConcurrent = 2

var artworkResizeSem = make(chan struct{}, artworkResizeMaxConcurrent)

// artworkMemoTTL bounds how long the parent-chain walk's lookups (below) are
// reused. Short on purpose: an admin picking new primary art, or a scan
// finding a poster, must become visible quickly - this only needs to survive
// one grid render, not outlive it.
const artworkMemoTTL = 30 * time.Second

// artworkRecordsCache and artworkParentIDCache memoise findArtworkRecords'
// parent-chain walk: a season/series with no episode-level art gets its
// GetByMediaItem+GetMediaItem pair re-run by EVERY sibling episode's artwork
// request, and a grid renders dozens of siblings at once. Keyed by the
// concrete repo/getter identity plus the looked-up id: production wires
// exactly one of each for the process lifetime, so this is a plain identity
// cache there; keying in the dependency keeps independent callers (and, in
// tests, independent fakes that reuse the same media IDs) from bleeding into
// each other.
//
// No background janitor (cleanupInterval=0): Get's own lazy-expiry plus the
// cache's LRU cap already bound memory, so there is no goroutine to shut down.
var artworkRecordsCache, _ = pkgcache.New[[]*domainmeta.ArtworkRecord](0)
var artworkParentIDCache, _ = pkgcache.New[string](0)

// artworkCacheKey scopes a memoised lookup to both the dependency instance
// (so distinct ArtworkReader/MediaItemGetter wirings never collide) and the
// looked-up id.
func artworkCacheKey(dep any, id string) string {
	return fmt.Sprintf("%p:%s", dep, id)
}

// resetArtworkMemoCaches clears the process-wide artwork parent-chain
// memoisation caches. Test-only (unexported, unused outside *_test.go): the
// production repo/getter live for the whole process, so their addresses are
// never freed and reused - but a test that constructs many short-lived fakes
// sharing a common ancestor id (e.g. "season1") in a tight loop CAN hit a
// still-live entry keyed off a just-freed fake's reused address. Calling this
// at the top of such a test sidesteps depending on GC timing.
func resetArtworkMemoCaches() {
	artworkRecordsCache, _ = pkgcache.New[[]*domainmeta.ArtworkRecord](0)
	artworkParentIDCache, _ = pkgcache.New[string](0)
}

// allowedWidths caps variant creation to a fixed set so the cache stays bounded.
// 0 is the sentinel for "no resize".
//
// The upper end matters: a request is SNAPPED to the nearest entry, so before
// 1280/1920 existed the detail-page hero asked for w=1920 and silently got the
// 780px variant stretched across a viewport up to 1920px wide - the whole
// srcSet resolved to one blurry file. Poster sizes (185/342/500) are unchanged;
// only backdrop-scale entries were missing.
var allowedWidths = []int{185, 342, 500, 780, 1280, 1920}

// parseRequestedWidth reads ?w= from the request and snaps it to the nearest
// allowed width. Returns 0 when the parameter is absent or unparseable.
func parseRequestedWidth(r *http.Request) int {
	raw := r.URL.Query().Get("w")
	if raw == "" {
		return 0
	}
	w, err := strconv.Atoi(raw)
	if err != nil || w <= 0 {
		return 0
	}
	best := 0
	bestDiff := 1 << 30
	for _, allowed := range allowedWidths {
		diff := w - allowed
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = allowed
		}
	}
	return best
}

// buildVariantPath derives the on-disk path for a resized variant, e.g.
// /data/artworks/abc/poster.jpg → /data/artworks/abc/poster_w400.webp
func buildVariantPath(srcPath string, width int) string {
	base := srcPath
	if before, _, ok := strings.CutLast(srcPath, "."); ok {
		base = before
	}
	return fmt.Sprintf("%s_w%d.webp", base, width)
}

// removeVariant best-effort deletes a stale/partial resized variant so it isn't
// served next time. A failure (other than already-gone) is logged but never
// fatal - the next request simply regenerates it.
func removeVariant(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("artwork variant cleanup", slog.String("path", path), slog.Any("err", err))
	}
}

// ensureArtworkVariant guarantees variantPath holds a valid (non-empty)
// resized WebP for srcPath, generating it at most once even when many
// concurrent requests target the SAME variant (artworkResizeGroup), and never
// running more than artworkResizeMaxConcurrent ffmpeg resizes at a time
// across DIFFERENT variants (artworkResizeSem).
func (h *mediaServe) ensureArtworkVariant(ctx context.Context, srcPath, variantPath string, width int) error {
	_, err, _ := artworkResizeGroup.Do(variantPath, func() (any, error) {
		// Re-check under the flight: a request that queued behind another one
		// targeting the same variant may find it already produced (and valid)
		// by the time it gets to run.
		if fi, statErr := os.Stat(variantPath); statErr == nil && fi.Size() > 0 {
			return nil, nil
		}
		artworkResizeSem <- struct{}{}
		defer func() { <-artworkResizeSem }()
		// Detached from the triggering request: a client that navigates away
		// mid-resize must not abort the ffmpeg run every OTHER waiter on this
		// same variant is blocked on - identical contract to the remux
		// handler's own singleflight (remux_handler.go).
		resizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artworkResizeTimeout)
		defer cancel()
		return nil, h.imgConverter.ResizeAndConvertToWebP(resizeCtx, srcPath, variantPath, width, 80)
	})
	return err
}

func (h *mediaServe) handleArtwork(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.artwork.Serve")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	artType := r.PathValue("artType")
	span.SetAttributes(
		attribute.String("media_item_id", mediaID),
		attribute.String("art_type", artType),
	)
	claims := HTTPClaimsFromContext(r.Context())

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(otelcodes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	artItem, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "media item not found")
		http.NotFound(w, r)
		return
	}
	// Artwork is public: skip library access and parental checks when no auth
	// claims. checkParental itself also no-ops for nil/admin claims, but the
	// explicit claims != nil guard here keeps this route's public contract
	// obvious without depending on that internal detail.
	if claims != nil {
		if !checkLibraryAccess(r.Context(), w, claims, artItem.LibraryID, h.libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, artItem, h.parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		span.SetStatus(otelcodes.Error, "no artwork records")
		writeNoArtwork(w)
		return
	}

	wantsWebP := strings.Contains(r.Header.Get("Accept"), "image/webp")
	w.Header().Set("Vary", "Accept")
	requestedWidth := parseRequestedWidth(r)

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			// A non-empty LocalPath may still point at a file that has vanished
			// from disk (cache cleared, manual cleanup, partial fetch). Serving
			// it would 404, and the `return` below would swallow the SourceURL
			// fallback - leaving a broken image when a remote URL is available.
			// Treat a missing file as "no local copy" and fall through.
			if rec.LocalPath != nil && *rec.LocalPath != "" && localFileExists(*rec.LocalPath) {
				if wantsWebP {
					// When a width is requested, generate (or serve from cache) a resized variant.
					if requestedWidth > 0 && h.imgConverter != nil {
						variantPath := buildVariantPath(*rec.LocalPath, requestedWidth)
						// Regenerate when the variant is missing OR a 0-byte leftover
						// from a prior failed/timed-out resize. Checking only existence
						// would serve the empty file forever, and `immutable` caching
						// then freezes the broken image in clients - the cause of a
						// poster vanishing at just one srcSet width (e.g. w=500).
						if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
							// Not cached yet - generate (single-flighted, concurrency-capped;
							// see ensureArtworkVariant) with a 10 s timeout so a slow ffmpeg
							// doesn't block the response indefinitely.
							if resErr := h.ensureArtworkVariant(r.Context(), *rec.LocalPath, variantPath, requestedWidth); resErr != nil {
								slog.WarnContext(r.Context(), "artwork resize", slog.String("src", *rec.LocalPath), slog.Int("width", requestedWidth), slog.Any("err", resErr))
								removeVariant(variantPath) // drop any partial/empty output so it isn't served next time
								variantPath = ""           // fall through to full-res WebP
							}
						}
						// Never serve an empty variant, even on the generating request
						// (the converter may exit 0 yet produce nothing).
						if variantPath != "" {
							if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
								removeVariant(variantPath)
								variantPath = ""
							}
						}
						if variantPath != "" {
							w.Header().Set("Content-Type", "image/webp")
							serveFileWithCache(w, r, variantPath, true)
							return
						}
					}
					if webp := h.resolveWebP(r.Context(), rec); webp != "" {
						w.Header().Set("Content-Type", "image/webp")
						serveFileWithCache(w, r, webp, true)
						return
					}
				}
				serveFileWithCache(w, r, *rec.LocalPath, true)
				return
			}
		}
	}
	var artURL string
	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.SourceURL != nil {
				artURL = *rec.SourceURL
				break
			}
		}
	}
	if artURL == "" {
		span.SetStatus(otelcodes.Error, "no artwork url")
		writeNoArtwork(w)
		return
	}
	if err := urlvalidator.ValidateRedirectURL(artURL); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid redirect url")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, artURL, http.StatusFound)
}

// writeNoArtwork answers an artwork request that resolved to no usable image
// (no records at all, or a record with neither a local file nor a source URL).
//
// It returns 204 No Content rather than 404 Not Found on purpose: a media item
// with no poster yet is an expected, benign state, not an error. A 404 makes the
// browser log a red network error to the console for every posterless card in a
// grid; a 204 lets the <img> load fail silently so the client shows its own
// placeholder, with no console noise. A short, MUTABLE cache keeps a scrolling
// grid from re-requesting on every render while still recovering within minutes
// once artwork is added - the URL is often unversioned for lite feed items, so
// it must never be cached immutably.
func writeNoArtwork(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusNoContent)
}

// localFileExists reports whether path resolves to an existing file on disk.
// Used to avoid serving a stale artwork LocalPath whose file has been removed.
func localFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (h *mediaServe) findArtworkRecords(ctx context.Context, mediaID, artType, parentID string) []*domainmeta.ArtworkRecord {
	hasMatchingType := func(recs []*domainmeta.ArtworkRecord) bool {
		for _, rec := range recs {
			if artType == "" || rec.Type == artType {
				return true
			}
		}
		return false
	}
	lookupID := mediaID
	nextParentID := parentID
	// Bound the parent walk: the real media hierarchy is at most
	// episode -> season -> series, so a small cap is generous. The guard stops
	// a self-referential or cyclic parent_id (poisoned/buggy row) from spinning
	// this loop forever.
	const maxParentDepth = 8
	depth := 0
	for range maxParentDepth {
		var records []*domainmeta.ArtworkRecord
		var err error
		if depth == 0 {
			// The requested item's OWN artwork is unique to this one request -
			// no sibling ever shares mediaID - so there is nothing to gain by
			// memoising it; fetch directly, exactly as before.
			records, err = h.artworkRepo.GetByMediaItem(ctx, lookupID)
		} else {
			// From here on, lookupID is an ANCESTOR (season/series) that every
			// sibling under it repeats this same walk to reach - memoise it
			// (see artworkRecordsCache).
			records, err = h.cachedArtworkRecords(ctx, lookupID)
		}
		if err != nil {
			return nil
		}
		if hasMatchingType(records) {
			return records
		}
		if nextParentID == "" {
			return nil
		}
		lookupID = nextParentID
		pid, err := h.cachedParentID(ctx, lookupID)
		if err != nil {
			return nil
		}
		nextParentID = pid
		depth++
	}
	return nil
}

// cachedArtworkRecords wraps h.artworkRepo.GetByMediaItem with the short-TTL
// memoisation described at artworkRecordsCache: a season/series artwork
// lookup that many sibling episodes share within one grid render is fetched
// once instead of once per sibling.
func (h *mediaServe) cachedArtworkRecords(ctx context.Context, id string) ([]*domainmeta.ArtworkRecord, error) {
	key := artworkCacheKey(h.artworkRepo, id)
	if recs, ok := artworkRecordsCache.Get(key); ok {
		return recs, nil
	}
	recs, err := h.artworkRepo.GetByMediaItem(ctx, id)
	if err != nil {
		return nil, err
	}
	artworkRecordsCache.Set(key, recs, artworkMemoTTL)
	return recs, nil
}

// cachedParentID wraps h.libSvc.GetMediaItem with the same short-TTL
// memoisation, returning just the ParentID the parent-chain walk needs.
func (h *mediaServe) cachedParentID(ctx context.Context, id string) (string, error) {
	key := artworkCacheKey(h.libSvc, id)
	if pid, ok := artworkParentIDCache.Get(key); ok {
		return pid, nil
	}
	parent, err := h.libSvc.GetMediaItem(ctx, id)
	if err != nil {
		return "", err
	}
	artworkParentIDCache.Set(key, parent.ParentID, artworkMemoTTL)
	return parent.ParentID, nil
}

// resolveWebP returns the path to a full-resolution WebP copy of rec's local
// artwork, generating it if needed. rec.WebPPath is read and written ONLY from
// inside ensureArtworkWebP's singleflight closure (never here directly): the
// SAME *ArtworkRecord can be shared across concurrent sibling requests (see
// cachedArtworkRecords, the ancestor/parent-chain memoisation a season's
// episodes all hit), so a bare pre-check here - before any goroutine has
// entered the singleflight call - would read that field unsynchronized with
// another goroutine's write to it, a genuine data race, not just a missed
// coalescing opportunity. h.imgConverter itself never changes after the
// handler is constructed, so the h.imgConverter == nil case is the one
// rec.WebPPath read that's safe outside the flight: no request ever mutates
// the field when there is no converter to run.
func (h *mediaServe) resolveWebP(ctx context.Context, rec *domainmeta.ArtworkRecord) string {
	if h.imgConverter == nil || rec.LocalPath == nil || *rec.LocalPath == "" {
		if rec.WebPPath != nil && *rec.WebPPath != "" {
			if _, err := os.Stat(*rec.WebPPath); err == nil {
				return *rec.WebPPath
			}
		}
		return ""
	}
	ext := ""
	if idx := strings.LastIndex(*rec.LocalPath, "."); idx >= 0 {
		ext = (*rec.LocalPath)[idx:]
	}
	webpPath := strings.TrimSuffix(*rec.LocalPath, ext) + ".webp"
	if err := h.ensureArtworkWebP(ctx, *rec.LocalPath, webpPath, rec); err != nil {
		slog.WarnContext(ctx, "webp convert", slog.String("src", *rec.LocalPath), slog.Any("err", err))
		return ""
	}
	return webpPath
}

// ensureArtworkWebP guarantees webpPath holds a valid full-resolution WebP
// conversion of srcPath, converting it at most once even when many concurrent
// requests target the SAME source image (a poster grid cold-loading, or two
// tabs on the same title, previously raced N ConvertToWebP calls onto the
// identical destination file - same defect ensureArtworkVariant already fixes
// for the resized-variant path above). Reuses that SAME artworkResizeGroup
// and artworkResizeSem rather than a second pair: it's the same limited
// ffmpeg budget on the same weak NAS, whether the call resizes or just
// re-encodes to WebP at full resolution.
//
// Keyed with a "webp:" prefix so this flow's singleflight keys can never
// collide with ensureArtworkVariant's: today a resized variant path always
// carries a "_wN" suffix a full-res webp path lacks (buildVariantPath vs the
// TrimSuffix+".webp" above), so in practice the two are already distinct
// strings - the prefix just stops that from being a silent invariant a future
// naming change could break.
//
// The rec.WebPPath check happens INSIDE the closure, not before Do() is
// called: singleflight's own internal mutex is what makes this safe across
// the concurrent-sibling-record case above (its Lock/Unlock pairs establish
// the happens-before a bare field read outside Do() would not get), and it
// also does what ensureArtworkVariant's re-check-under-flight does for the
// resize path - a request queued behind another one targeting the same image
// may find it already produced by the time it gets to run.
func (h *mediaServe) ensureArtworkWebP(ctx context.Context, srcPath, webpPath string, rec *domainmeta.ArtworkRecord) error {
	_, err, _ := artworkResizeGroup.Do("webp:"+webpPath, func() (any, error) {
		if rec.WebPPath != nil && *rec.WebPPath != "" {
			if _, statErr := os.Stat(*rec.WebPPath); statErr == nil {
				return nil, nil
			}
		}
		artworkResizeSem <- struct{}{}
		defer func() { <-artworkResizeSem }()
		// Detached from the triggering request, identical contract to
		// ensureArtworkVariant: a client that navigates away mid-convert must
		// not abort the work every OTHER waiter on this same image is blocked
		// on - including the DB persist right below, which every waiter also
		// depends on to observe rec.WebPPath as set.
		convertCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artworkResizeTimeout)
		defer cancel()
		if err := h.imgConverter.ConvertToWebP(convertCtx, srcPath, webpPath, 80); err != nil {
			return nil, err
		}
		rec.WebPPath = &webpPath
		if h.artworkWriter != nil {
			if err := h.artworkWriter.UpdateWebPPath(convertCtx, rec.ID, webpPath); err != nil {
				slog.WarnContext(convertCtx, "save webp path", slog.Any("err", err))
			}
		}
		return nil, nil
	})
	return err
}

// selectArtworkHandler changes the artwork every account sees. profiles, when
// wired, refuses it from a profile under parental control (fail-closed on an
// unreadable one), like the gRPC metadata writes.
func selectArtworkHandler(artworkRepo ArtworkRepo, libSvc MediaItemGetter, libraryAccess LibraryAccessChecker, profiles ProfileParentalGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.artwork.Select")
		defer span.End()
		r = r.WithContext(ctx)
		claims := HTTPClaimsFromContext(r.Context())
		if claims != nil && !claims.IsAdmin && claims.ProfileID != "" && profiles != nil {
			p, err := profiles.GetProfileByID(r.Context(), claims.ProfileID)
			if err != nil || p == nil || p.EffectiveParental() != nil {
				span.SetStatus(otelcodes.Error, "parental profile")
				http.Error(w, `{"message":"errors.media.restricted_profile_edit"}`, http.StatusForbidden)
				return
			}
		}
		mediaID := r.PathValue("id")
		artworkID := r.PathValue("artworkId")
		span.SetAttributes(
			attribute.String("media_item_id", mediaID),
			attribute.String("artwork_id", artworkID),
		)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "media item not found")
			http.NotFound(w, r)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
		if err != nil || len(records) == 0 {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "no artwork records")
			http.NotFound(w, r)
			return
		}
		var selected *domainmeta.ArtworkRecord
		for _, rec := range records {
			if rec.ID == artworkID {
				selected = rec
				break
			}
		}
		if selected == nil {
			span.SetStatus(otelcodes.Error, "artwork id not found")
			http.NotFound(w, r)
			return
		}
		for _, rec := range records {
			if rec.Type == selected.Type && rec.ID != selected.ID && rec.SortOrder < selected.SortOrder {
				if err := artworkRepo.UpdateSortOrder(r.Context(), rec.ID, selected.SortOrder); err != nil {
					slog.ErrorContext(r.Context(), "artwork sort update (demote)", slog.Any("err", err))
					http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
					return
				}
				break
			}
		}
		// This is the defining write - promoting `selected` to primary. Reporting
		// {"ok":true} when it fails made the UI show a new poster that reverted on
		// the next refresh.
		if err := artworkRepo.UpdateSortOrder(r.Context(), selected.ID, -1); err != nil {
			slog.ErrorContext(r.Context(), "artwork sort update (promote)", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		// The sort order just changed on disk; drop the memoised records for
		// this id (see artworkRecordsCache) so the next artwork request sees
		// the new primary immediately instead of waiting out its TTL.
		artworkRecordsCache.Delete(artworkCacheKey(artworkRepo, mediaID))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
