package httpapi

import (
	"context"
	"fmt"
	pkgcache "github.com/eveyproject/evey/internal/pkg/cache"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// logDownloadRefusal records a download the server turned away.
//
// Every refusal below used to set an OpenTelemetry span status and nothing
// else, so a user reporting "the download does nothing" left no trace at all in
// the logs an operator can actually search - the same blind spot that made the
// 2026-07-29 remote-access lockout take a full day to pin down. Spans are not a
// substitute: tracing is sampled and is not what an admin greps.
//
// WARN, not INFO: a refusal on a route the client only reaches after minting a
// scoped token is never routine.
func logDownloadRefusal(r *http.Request, mediaID, reason string, claims *domainuser.Claims) {
	attrs := []any{
		slog.String("component", "download"),
		slog.String("media_id", mediaID),
		slog.String("reason", reason),
		slog.String("client_ip", ratelimit.ClientIP(r)),
	}
	if claims != nil {
		attrs = append(attrs, slog.String("user_id", claims.UserID), slog.String("profile_id", claims.ProfileID))
	}
	slog.WarnContext(r.Context(), "download refused", attrs...)
}

// --- mediaHandler: stream + download ---

type mediaServe struct {
	libSvc MediaItemGetter
	// itemLookup is the short-TTL cache of the direct-play hot path: a
	// browser or a television fetches a film as a run of Range requests, and
	// each used to re-read the item (six index walks) before serving bytes.
	// Same 10 s staleness as the HLS media lookup; the access and parental
	// checks below always run live.
	itemLookup    *pkgcache.TTLCache[*library.MediaItem]
	artworkRepo   ArtworkReader
	artworkWriter ArtworkWebPWriter
	imgConverter  ImageConverter
	baseURL       string
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	libLister     LibraryLister // optional: confines streamed paths to library roots
	// activity reports direct-play reads to the anti-starvation guards. Direct
	// play is a bare ServeFile - no ffmpeg session exists for the producer
	// counters to see - so without this a direct-play viewer reads as an idle
	// server. nil = not wired (no behaviour change).
	activity *apptranscoding.PlaybackActivity
	// castProgress derives watch progress from a cast receiver's Range fetches
	// of a direct-play file (see cast_observe.go). nil = not wired.
	castProgress CastFetchObserver
}

// ArtworkWebPWriter updates the WebP path on an artwork record.
type ArtworkWebPWriter interface {
	UpdateWebPPath(ctx context.Context, id string, webpPath string) error
}

type mediaServeOption func(*mediaServe)

func withImageConverter(conv ImageConverter, writer ArtworkWebPWriter) mediaServeOption {
	return func(h *mediaServe) {
		h.imgConverter = conv
		h.artworkWriter = writer
	}
}

// withParental wires the parental gate onto the stream handler. Supplied via an
// option so the many mediaHandler test call sites stay unchanged.
func withParental(pc ParentalChecker) mediaServeOption {
	return func(h *mediaServe) { h.parental = pc }
}

// withPlaybackActivity wires the direct-play read into the "someone is
// watching" signal every background worker yields on.
func withPlaybackActivity(a *apptranscoding.PlaybackActivity) mediaServeOption {
	return func(h *mediaServe) { h.activity = a }
}

// withLibraryLister wires library-root path confinement onto the stream
// handler (the same defense-in-depth serveDownload applies): a poisoned DB
// path must not turn the stream endpoint into an arbitrary-file read.
//
// This comment used to sit on top of withPlaybackActivity, one function up,
// leaving the security note attached to the wrong thing and this one bare.
func withLibraryLister(ll LibraryLister) mediaServeOption {
	return func(h *mediaServe) { h.libLister = ll }
}

// withCastProgress wires the cast progress tracker onto the direct-play
// stream handler (see cast_observe.go).
func withCastProgress(obs CastFetchObserver) mediaServeOption {
	return func(h *mediaServe) { h.castProgress = obs }
}

func mediaHandler(libSvc MediaItemGetter, artworkRepo ArtworkReader, baseURL string, libraryAccess LibraryAccessChecker, opts ...mediaServeOption) http.HandlerFunc {
	h := &mediaServe{libSvc: libSvc, artworkRepo: artworkRepo, baseURL: baseURL, libraryAccess: libraryAccess}
	h.itemLookup, _ = pkgcache.NewWithMaxSize[*library.MediaItem](0, mediaLookupMax)
	for _, o := range opts {
		o(h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		artType := r.PathValue("artType")
		if artType != "" {
			h.handleArtwork(w, r)
		} else {
			h.handleStream(w, r)
		}
	}
}

// lookupItem reads the item through itemLookup; a miss or an error asks the
// service, and only a found item is cached.
func (h *mediaServe) lookupItem(ctx context.Context, mediaID string) (*library.MediaItem, error) {
	if h.itemLookup != nil {
		if item, ok := h.itemLookup.Get(mediaID); ok {
			return item, nil
		}
	}
	item, err := h.libSvc.GetMediaItem(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if h.itemLookup != nil {
		h.itemLookup.Set(mediaID, item, mediaLookupTTL)
	}
	return item, nil
}

func (h *mediaServe) handleStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.HandleStream")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	item, err := h.lookupItem(r.Context(), mediaID)
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
	// Mirror serveDownload's confinement: only files under the configured
	// library roots are served, so a DB-poisoned path can't exfiltrate
	// arbitrary host files. Same fail-open semantics on unknown roots.
	if h.libLister != nil && !pathWithinRoots(mediaRoots(r.Context(), h.libLister), item.File.Path) {
		span.SetStatus(codes.Error, "file outside library roots")
		slog.WarnContext(r.Context(), "stream: file path outside configured library roots",
			slog.String("media_id", mediaID), slog.String("path", item.File.Path))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	setCORSHeaders(w, r, h.baseURL)
	// Begin, not Touch: ServeFile can block for the whole film on a single
	// request, so the media must count as active for as long as this call runs,
	// not for a TTL measured from its start. Placed after every access gate so a
	// refused request never registers as playback.
	//
	// Streaming only, deliberately - serveDownload is NOT tracked. The guards
	// exist so a viewer doesn't stutter; a download doesn't stutter, and a 40 GB
	// remux would otherwise hold every background worker off the disk for the
	// hour it takes to transfer.
	defer h.activity.Begin(mediaID)()
	// Cast progress: a direct-play receiver reads the file progressively with
	// Range requests, so the byte offset maps to a position by simple ratio.
	// VBR skews it (dense scenes understate, credits overstate), but a
	// margin-trimmed estimate beats the zero reporting it replaces once the
	// sender's browser is gone (see cast_observe.go).
	if durMs := item.File.Duration.Milliseconds(); durMs > 0 && item.File.Size > 0 {
		if start, ok := parseRangeStart(r.Header.Get("Range")); ok && start > 0 {
			observeCastFetch(r, h.castProgress, durMs*start/item.File.Size)
		}
	}
	serveFileLogged(w, r, mediaID, item.File.Path)
}

func downloadHandler(libSvc MediaArchiveSource, baseURL string, libraryAccess LibraryAccessChecker, parental ParentalChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.media.Download")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			// A syncplay guest may stream but never download the shared media.
			if st.Scope == shared.ScopeSyncplayGuest {
				span.SetStatus(codes.Error, "syncplay guest token cannot download")
				logDownloadRefusal(r, mediaID, "syncplay_guest_token", claims)
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(codes.Error, "stream token media mismatch")
				logDownloadRefusal(r, mediaID, "stream_token_media_mismatch", claims)
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}

		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(codes.Error, "media item lookup failed")
			logDownloadRefusal(r, mediaID, "media_item_lookup_failed", claims)
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(codes.Error, "library access denied")
			// The response is a deliberate 404 (an existence oracle would let a
			// profile enumerate other libraries' media), which is exactly why the
			// LOG has to say what really happened - otherwise the operator sees
			// the same "not found" as a genuinely missing item.
			logDownloadRefusal(r, mediaID, "library_access_denied", claims)
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(codes.Error, "parental restricted")
			logDownloadRefusal(r, mediaID, "parental_restricted", claims)
			return
		}
		if !checkPlayable(w, item) {
			span.SetStatus(codes.Error, "file needs remux")
			return
		}

		// Single-file items (movie, episode, track) stream the raw file.
		if item.File != nil && item.File.Path != "" {
			serveDownload(w, r, item, baseURL, mediaRoots(r.Context(), libSvc))
			return
		}

		// Container items are bundled into a ZIP on the fly: a season packs its
		// episodes, a series packs every episode under per-season folders.
		switch item.Type {
		case library.Season:
			serveSeasonArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc), parentalAllowFunc(r.Context(), claims, parental))
		case library.Series:
			serveSeriesArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc), parentalAllowFunc(r.Context(), claims, parental))
		default:
			span.SetStatus(codes.Error, "no downloadable file")
			logDownloadRefusal(r, mediaID, "no_downloadable_file", claims)
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		}
	}
}

func serveDownload(w http.ResponseWriter, r *http.Request, item *library.MediaItem, baseURL string, roots []string) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ServeDownload")
	defer span.End()
	r = r.WithContext(ctx)
	if item.File == nil {
		span.SetStatus(codes.Error, "media file missing")
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	// Defense-in-depth: confine path to known library roots so a poisoned
	// media_files.path cannot exfiltrate an arbitrary file. Fails open when
	// roots are unknown (transient list error) - confinement is NOT the
	// access gate; access was already checked by the caller.
	if len(roots) > 0 && !pathWithinRoots(roots, item.File.Path) {
		slog.WarnContext(r.Context(), "download: path outside library roots, blocked",
			slog.String("item_id", string(item.ID)),
			slog.String("path", item.File.Path),
		)
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	fi, err := os.Stat(item.File.Path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "stat failed")
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	filename := sanitizeFilename(filepath.Base(item.File.Path))
	// One line per accepted download, at INFO. Without it a download is
	// invisible server-side even when it succeeds: HTTP routes are not
	// access-logged here, only failures write, so "the client says nothing
	// happened" and "the server never heard from the client" look identical.
	// bytes is the SOURCE size, which is also the answer to "why did my phone
	// refuse it" - a 19 GB remux is not a client bug.
	slog.InfoContext(r.Context(), "download served",
		slog.String("component", "download"),
		slog.String("media_id", string(item.ID)),
		slog.String("filename", filename),
		slog.Int64("bytes", fi.Size()),
		// Present when the client resumes or chunks; empty on a plain one-shot
		// GET. It is what tells a stalled transfer apart from a retried one.
		slog.String("range", r.Header.Get("Range")),
		slog.String("client_ip", ratelimit.ClientIP(r)),
	)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	setCORSHeaders(w, r, baseURL)
	http.ServeFile(w, r, item.File.Path)
}

// archiveEntry is one file to place inside a download ZIP. name may contain a
// "/" to nest the file under a per-season folder.
type archiveEntry struct {
	path string
	name string
}

// serveSeasonArchive streams a ZIP of every episode file of a season, flat.
func serveSeasonArchive(w http.ResponseWriter, r *http.Request, libSvc MediaArchiveSource, season *library.MediaItem, baseURL string, roots []string, allowParental func(*library.MediaItem) bool) {
	episodes := sortedChildren(r.Context(), libSvc, string(season.ID), season.LibraryID)
	var entries []archiveEntry
	for _, ep := range episodes {
		// Per episode: clearing the season container does not vouch for the
		// episodes inside it, each carries its own rating.
		if !archivableFile(ep, roots) || !allowParental(ep) {
			continue
		}
		entries = append(entries, archiveEntry{path: ep.File.Path, name: episodeEntryName(ep)})
	}
	serveMediaArchive(w, r, season.Title, entries, baseURL)
}

// serveSeriesArchive streams a ZIP of every episode of every season, nested
// under per-season folders ("Season 01/E01 - Title.mkv").
func serveSeriesArchive(w http.ResponseWriter, r *http.Request, libSvc MediaArchiveSource, series *library.MediaItem, baseURL string, roots []string, allowParental func(*library.MediaItem) bool) {
	seasons := sortedChildren(r.Context(), libSvc, string(series.ID), series.LibraryID)
	var entries []archiveEntry
	for _, season := range seasons {
		if !allowParental(season) {
			continue
		}
		folder := seasonFolderName(season)
		for _, ep := range sortedChildren(r.Context(), libSvc, string(season.ID), series.LibraryID) {
			if !archivableFile(ep, roots) || !allowParental(ep) {
				continue
			}
			entries = append(entries, archiveEntry{path: ep.File.Path, name: folder + "/" + episodeEntryName(ep)})
		}
	}
	serveMediaArchive(w, r, series.Title, entries, baseURL)
}

// archivableFile reports whether an item has an on-disk file safe to bundle:
// it must exist and (defense-in-depth) sit under a configured library root, so
// a poisoned media_files.path can't exfiltrate an arbitrary file via the ZIP.
func archivableFile(it *library.MediaItem, roots []string) bool {
	if it == nil || it.File == nil || it.File.Path == "" {
		return false
	}
	if !pathWithinRoots(roots, it.File.Path) {
		slog.Warn("download archive: path outside library roots, skipped", slog.String("item_id", string(it.ID)), slog.String("path", it.File.Path))
		return false
	}
	return true
}

// mediaRoots returns the cleaned set of configured library paths. Returns nil
// on error or when none are configured - callers treat nil as "unknown" and
// fall open (the path confinement is defense-in-depth, not the access gate).
func mediaRoots(ctx context.Context, libSvc LibraryLister) []string {
	libs, err := libSvc.ListLibraries(ctx)
	if err != nil {
		slog.WarnContext(ctx, "download archive: list libraries for path confinement", slog.Any("err", err))
		return nil
	}
	var roots []string
	for _, l := range libs {
		for _, p := range l.Paths {
			if p != "" {
				roots = append(roots, p)
			}
		}
	}
	return roots
}

// pathWithinRoots reports whether path sits under one of roots. Empty roots
// means "unknown" and returns true (fail open) so a transient library-list
// failure never blocks an otherwise-authorised download.
func pathWithinRoots(roots []string, path string) bool {
	if len(roots) == 0 {
		return true
	}
	for _, root := range roots {
		if isWithinDir(root, path) {
			return true
		}
	}
	return false
}

// sortedChildren lists the children of parentID, ordered by index number, and
// restricted to the parent's library. Defense-in-depth: the parent's access
// was already authorised, so a child in another library must not be bundled in.
func sortedChildren(ctx context.Context, libSvc MediaArchiveSource, parentID string, libraryID shared.LibraryID) []*library.MediaItem {
	items, err := libSvc.ListMediaItems(ctx, library.ListMediaItemsFilter{ParentID: &parentID})
	if err != nil {
		slog.WarnContext(ctx, "download archive: list children", slog.String("parent_id", parentID), slog.Any("err", err))
		return nil
	}
	out := make([]*library.MediaItem, 0, len(items))
	for _, it := range items {
		if it.LibraryID != libraryID {
			slog.WarnContext(ctx, "download archive: cross-library child skipped",
				slog.String("item_id", string(it.ID)), slog.String("expected_lib", string(libraryID)), slog.String("got_lib", string(it.LibraryID)))
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return indexNumber(out[i]) < indexNumber(out[j]) })
	return out
}

func indexNumber(it *library.MediaItem) int {
	if it.IndexNumber != nil {
		return *it.IndexNumber
	}
	return 0
}

func episodeEntryName(ep *library.MediaItem) string {
	ext := filepath.Ext(ep.File.Path)
	base := sanitizeFilename(ep.Title)
	if ep.IndexNumber != nil {
		base = fmt.Sprintf("E%02d - %s", *ep.IndexNumber, base)
	}
	return base + ext
}

func seasonFolderName(season *library.MediaItem) string {
	if season.IndexNumber != nil {
		return fmt.Sprintf("Season %02d", *season.IndexNumber)
	}
	return sanitizeFilename(season.Title)
}

// serveMediaArchive streams the given entries as a ZIP. Files are stored
// uncompressed (zip.Store): media is already compressed, so deflate would burn
// CPU for no size gain. Duplicate entry names are disambiguated with a suffix.
func serveMediaArchive(w http.ResponseWriter, r *http.Request, title string, entries []archiveEntry, baseURL string) {
	_, span := tracing.StartSpan(r.Context(), "http.media.ServeArchive")
	defer span.End()
	layout, skipped := newZipLayout(dedupArchiveNames(entries))
	for _, err := range skipped {
		slog.WarnContext(r.Context(), "download archive: entry skipped", slog.Any("err", err))
	}
	if len(layout.files) == 0 {
		span.SetStatus(codes.Error, "no downloadable files")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	zipName := sanitizeFilename(title) + ".zip"
	slog.InfoContext(r.Context(), "download archive served",
		slog.String("component", "download"),
		slog.String("filename", zipName),
		slog.Int("files", len(layout.files)),
		slog.Int64("bytes", layout.size),
		slog.String("range", r.Header.Get("Range")),
		slog.String("client_ip", ratelimit.ClientIP(r)),
	)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": zipName}))
	w.Header().Set("ETag", layout.etag)
	setCORSHeaders(w, r, baseURL)
	// A large archive can take longer than the global write timeout to stream.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(r.Context(), "set write deadline", slog.Any("err", err))
	}
	zs := newZipStream(layout)
	defer zs.Close() //nolint:errcheck // read-only file handle
	// ServeContent answers Content-Length, Range, If-Range and HEAD from the
	// laid-out archive, as it would for a file on disk.
	http.ServeContent(w, r, zipName, time.Time{}, zs)
}

// dedupArchiveNames renames colliding entries "X (2).ext", "X (3).ext"...
// The loop registers every step, so an entry whose name literally equals a
// renamed form (e.g. "X (2).ext") still lands on an unused one.
func dedupArchiveNames(entries []archiveEntry) []archiveEntry {
	out := make([]archiveEntry, 0, len(entries))
	seen := make(map[string]int)
	for _, e := range entries {
		name := e.name
		for {
			n, ok := seen[name]
			if !ok {
				seen[name] = 1
				break
			}
			ext := filepath.Ext(name)
			seen[name] = n + 1
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n+1, ext)
		}
		out = append(out, archiveEntry{path: e.path, name: name})
	}
	return out
}

// buildMediaArchiveEntries flattens a heterogeneous list of media items (as
// found in a collection or a playlist) into ZIP entries: a single file for
// movies/episodes/tracks, and a per-title folder of episodes for series
// (nested by season) and seasons. Items the caller can't access, or whose file
// sits outside the library roots, are dropped.
// buildMediaArchiveEntries flattens items into ZIP entries, dropping anything
// the caller may not have. allowParental is applied per item AND per episode:
// the streaming routes answer 404 for a parentally-blocked title, so the
// archive routes must not hand the same file over in a bundle. A season or
// series container that passes the gate does not vouch for its episodes -
// each one is rated on its own.
func buildMediaArchiveEntries(ctx context.Context, libSvc MediaArchiveSource, items []*library.MediaItem, canAccess func(shared.LibraryID) bool, allowParental func(*library.MediaItem) bool, roots []string) []archiveEntry {
	var entries []archiveEntry
	for _, it := range items {
		if it == nil || !canAccess(it.LibraryID) || !allowParental(it) {
			continue
		}
		switch {
		case it.File != nil && it.File.Path != "":
			if !archivableFile(it, roots) {
				continue
			}
			entries = append(entries, archiveEntry{path: it.File.Path, name: singleFileEntryName(it)})
		case it.Type == library.Series:
			folder := sanitizeFilename(it.Title)
			for _, season := range sortedChildren(ctx, libSvc, string(it.ID), it.LibraryID) {
				if !allowParental(season) {
					continue
				}
				sub := folder + "/" + seasonFolderName(season)
				for _, ep := range sortedChildren(ctx, libSvc, string(season.ID), it.LibraryID) {
					if !archivableFile(ep, roots) || !allowParental(ep) {
						continue
					}
					entries = append(entries, archiveEntry{path: ep.File.Path, name: sub + "/" + episodeEntryName(ep)})
				}
			}
		case it.Type == library.Season:
			folder := sanitizeFilename(it.Title)
			for _, ep := range sortedChildren(ctx, libSvc, string(it.ID), it.LibraryID) {
				if !archivableFile(ep, roots) || !allowParental(ep) {
					continue
				}
				entries = append(entries, archiveEntry{path: ep.File.Path, name: folder + "/" + episodeEntryName(ep)})
			}
		}
	}
	return entries
}

// parentalAllowFunc mirrors libraryAccessFunc for the parental gate: a
// per-item predicate for the archive paths, which must silently drop what they
// may not include rather than fail the whole download. Admins and an unwired
// checker short-circuit to "allow", exactly like checkParental.
//
// The checker reloads the profile per call. That is one small query per archive
// member, next to reading and streaming the media files themselves - not worth
// a memoisation layer here, and correctness on a security gate comes first.
func parentalAllowFunc(ctx context.Context, claims *domainuser.Claims, pc ParentalChecker) func(*library.MediaItem) bool {
	if pc == nil || claims == nil || claims.IsAdmin {
		return func(*library.MediaItem) bool { return true }
	}
	profileID := claims.ProfileID
	return func(it *library.MediaItem) bool {
		return !pc.BlockedByParental(ctx, profileID, it)
	}
}

func singleFileEntryName(it *library.MediaItem) string {
	return sanitizeFilename(it.Title) + filepath.Ext(it.File.Path)
}

// libraryAccessFunc returns a per-library access predicate for archive builds.
// Admins pass through; other users are checked against the LibraryAccessChecker,
// memoised per library. A nil caller (shouldn't happen behind auth) is denied.
func libraryAccessFunc(ctx context.Context, claims *domainuser.Claims, checker LibraryAccessChecker) func(shared.LibraryID) bool {
	isAdmin := claims != nil && claims.IsAdmin
	var userID shared.UserID
	if claims != nil {
		userID = shared.UserID(claims.UserID)
	}
	cache := make(map[shared.LibraryID]bool)
	return func(lib shared.LibraryID) bool {
		if isAdmin {
			return true
		}
		// Mirror checkLibraryAccess: an unwired checker denies rather than
		// dereferencing nil. This one runs per archive member mid-response, so
		// a panic here would tear down an in-flight ZIP instead of returning.
		if claims == nil || checker == nil {
			return false
		}
		if v, ok := cache[lib]; ok {
			return v
		}
		ok, err := checker.HasAccess(ctx, userID, lib)
		v := err == nil && ok
		cache[lib] = v
		return v
	}
}
