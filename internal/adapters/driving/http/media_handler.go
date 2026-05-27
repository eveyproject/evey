package httpapi

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
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

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// filterMasterPlaylist removes #EXT-X-STREAM-INF variants exceeding maxBps (bps).
// If all variants exceed the limit, keeps the one with the lowest BANDWIDTH.
func filterMasterPlaylist(content string, maxBps int64) string {
	if maxBps <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	type variant struct {
		inf string
		uri string
		bw  int64
	}
	var header []string
	var variants []variant
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			bw := int64(0)
			attrs := strings.TrimPrefix(line, "#EXT-X-STREAM-INF:")
			if _, after, ok := strings.Cut(attrs, "BANDWIDTH="); ok {
				if _, err := fmt.Sscanf(after, "%d", &bw); err != nil {
					slog.Warn("parse BANDWIDTH", slog.Any("err", err))
				}
			}
			uri := ""
			if i+1 < len(lines) {
				uri = lines[i+1]
				i++
			}
			variants = append(variants, variant{inf: line, uri: uri, bw: bw})
		} else {
			header = append(header, line)
		}
		i++
	}

	var allowed []variant
	for _, v := range variants {
		if v.bw <= maxBps {
			allowed = append(allowed, v)
		}
	}
	if len(allowed) == 0 && len(variants) > 0 {
		lowest := variants[0]
		for _, v := range variants[1:] {
			if v.bw < lowest.bw {
				lowest = v
			}
		}
		allowed = []variant{lowest}
	}

	var out []string
	out = append(out, header...)
	for _, v := range allowed {
		out = append(out, v.inf, v.uri)
	}
	return strings.Join(out, "\n")
}

// --- mediaHandler: stream + download ---

type mediaServe struct {
	tokens        TokenVerifier
	libSvc        MediaItemGetter
	artworkRepo   ArtworkReader
	artworkWriter ArtworkWebPWriter
	imgConverter  ImageConverter
	baseURL       string
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
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

func mediaHandler(tokens TokenVerifier, libSvc MediaItemGetter, artworkRepo ArtworkReader, baseURL string, libraryAccess LibraryAccessChecker, opts ...mediaServeOption) http.HandlerFunc {
	h := &mediaServe{tokens: tokens, libSvc: libSvc, artworkRepo: artworkRepo, baseURL: baseURL, libraryAccess: libraryAccess}
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
	if item.File == nil || item.File.Path == "" {
		span.SetStatus(codes.Error, "media file missing")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	setCORSHeaders(w, r, h.baseURL)
	http.ServeFile(w, r, item.File.Path)
}

func downloadHandler(tokens TokenVerifier, libSvc MediaArchiveSource, baseURL string, libraryAccess LibraryAccessChecker, parental ParentalChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.media.Download")
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

		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(codes.Error, "media item lookup failed")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(codes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(codes.Error, "parental restricted")
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
			serveSeasonArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc))
		case library.Series:
			serveSeriesArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc))
		default:
			span.SetStatus(codes.Error, "no downloadable file")
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
	// roots are unknown (transient list error) — confinement is NOT the
	// access gate; access was already checked by the caller.
	if len(roots) > 0 && !pathWithinRoots(roots, item.File.Path) {
		slog.Warn("download: path outside library roots, blocked",
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
func serveSeasonArchive(w http.ResponseWriter, r *http.Request, libSvc MediaArchiveSource, season *library.MediaItem, baseURL string, roots []string) {
	episodes := sortedChildren(r.Context(), libSvc, string(season.ID), season.LibraryID)
	var entries []archiveEntry
	for _, ep := range episodes {
		if !archivableFile(ep, roots) {
			continue
		}
		entries = append(entries, archiveEntry{path: ep.File.Path, name: episodeEntryName(ep)})
	}
	serveMediaArchive(w, r, season.Title, entries, baseURL)
}

// serveSeriesArchive streams a ZIP of every episode of every season, nested
// under per-season folders ("Season 01/E01 - Title.mkv").
func serveSeriesArchive(w http.ResponseWriter, r *http.Request, libSvc MediaArchiveSource, series *library.MediaItem, baseURL string, roots []string) {
	seasons := sortedChildren(r.Context(), libSvc, string(series.ID), series.LibraryID)
	var entries []archiveEntry
	for _, season := range seasons {
		folder := seasonFolderName(season)
		for _, ep := range sortedChildren(r.Context(), libSvc, string(season.ID), series.LibraryID) {
			if !archivableFile(ep, roots) {
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
// on error or when none are configured — callers treat nil as "unknown" and
// fall open (the path confinement is defense-in-depth, not the access gate).
func mediaRoots(ctx context.Context, libSvc MediaArchiveSource) []string {
	libs, err := libSvc.ListLibraries(ctx)
	if err != nil {
		slog.Warn("download archive: list libraries for path confinement", slog.Any("err", err))
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
		slog.Warn("download archive: list children", slog.String("parent_id", parentID), slog.Any("err", err))
		return nil
	}
	out := make([]*library.MediaItem, 0, len(items))
	for _, it := range items {
		if it.LibraryID != libraryID {
			slog.Warn("download archive: cross-library child skipped",
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
	if len(entries) == 0 {
		span.SetStatus(codes.Error, "no downloadable files")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	zipName := sanitizeFilename(title) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": zipName}))
	setCORSHeaders(w, r, baseURL)
	// A large archive can take longer than the global write timeout to stream.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	zw := zip.NewWriter(w)
	defer func() {
		if err := zw.Close(); err != nil {
			slog.Warn("download archive: zip close", slog.Any("err", err))
		}
	}()
	seen := make(map[string]int)
	for _, e := range entries {
		name := e.name
		if n, ok := seen[name]; ok {
			ext := filepath.Ext(name)
			seen[e.name] = n + 1
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n+1, ext)
		} else {
			seen[name] = 1
		}
		if err := addFileToZip(zw, e.path, name); err != nil {
			slog.Warn("download archive: add file", slog.String("path", e.path), slog.Any("err", err))
		}
	}
}

func addFileToZip(zw *zip.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zfw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(zfw, f)
	return err
}

// buildMediaArchiveEntries flattens a heterogeneous list of media items (as
// found in a collection or a playlist) into ZIP entries: a single file for
// movies/episodes/tracks, and a per-title folder of episodes for series
// (nested by season) and seasons. Items the caller can't access, or whose file
// sits outside the library roots, are dropped.
func buildMediaArchiveEntries(ctx context.Context, libSvc MediaArchiveSource, items []*library.MediaItem, canAccess func(shared.LibraryID) bool, roots []string) []archiveEntry {
	var entries []archiveEntry
	for _, it := range items {
		if it == nil || !canAccess(it.LibraryID) {
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
				sub := folder + "/" + seasonFolderName(season)
				for _, ep := range sortedChildren(ctx, libSvc, string(season.ID), it.LibraryID) {
					if !archivableFile(ep, roots) {
						continue
					}
					entries = append(entries, archiveEntry{path: ep.File.Path, name: sub + "/" + episodeEntryName(ep)})
				}
			}
		case it.Type == library.Season:
			folder := sanitizeFilename(it.Title)
			for _, ep := range sortedChildren(ctx, libSvc, string(it.ID), it.LibraryID) {
				if !archivableFile(ep, roots) {
					continue
				}
				entries = append(entries, archiveEntry{path: ep.File.Path, name: folder + "/" + episodeEntryName(ep)})
			}
		}
	}
	return entries
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
		if claims == nil {
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
