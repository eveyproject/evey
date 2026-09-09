package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/pkg/langcode"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	mkvsubtitle "github.com/gravity-zero/mkvgo/mkv/subtitle"
	mkvgomp4 "github.com/gravity-zero/mkvgo/mp4"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/gravity-zero/mkvgo/matroska"
)

// subtitleService is the subset of appsubtitle.Service used by HTTP routes.
type subtitleService interface {
	Search(ctx context.Context, mediaItemID, language string) ([]domainsubtitle.SearchResult, error)
	Download(ctx context.Context, mediaItemID string, fileID int, language, format string) (*domainsubtitle.SubtitleDownload, error)
	List(ctx context.Context, mediaItemID string) ([]*domainsubtitle.SubtitleDownload, error)
	Delete(ctx context.Context, subtitleID string) error
}

func subtitleHandler(libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker, parental ParentalChecker, cache subtitleCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Serve")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
		span.SetAttributes(
			attribute.String("media_item_id", mediaID),
			attribute.String("track", trackStr),
		)
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(otelcodes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}
		trackIndex, err := strconv.Atoi(trackStr)
		if err != nil || trackIndex < 0 {
			span.SetStatus(otelcodes.Error, "invalid track index")
			http.Error(w, `{"message":"invalid track index"}`, http.StatusBadRequest)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.File == nil || item.File.Path == "" {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}
		filePath := item.File.Path

		streams, serr := libSvc.GetMediaStreams(r.Context(), mediaID)
		if serr != nil {
			// Fail closed (and visibly): without the stream list we can't tell a
			// valid text track from a forged ?track= index or a bitmap subtitle.
			// Silently falling through used to spawn ffmpeg on the unvalidated
			// index, emitting a broken/empty stream with no error signal.
			slog.WarnContext(r.Context(), "subtitle: get media streams", slog.String("media_id", mediaID), slog.Any("err", serr))
			span.SetStatus(otelcodes.Error, "media streams unavailable")
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		subIdx := 0
		matched := false
		imageSub := false
		var matchedCodec, matchedLanguage string
		for _, s := range streams {
			if s.StreamType != "subtitle" {
				continue
			}
			if subIdx == trackIndex {
				if s.IsExternal && s.ExternalPath != nil {
					serveExternalSubtitle(w, r, *s.ExternalPath, filepath.Dir(filePath), baseURL)
					return
				}
				matched = true
				imageSub = apptranscoding.IsImageSubtitle(s.Codec)
				matchedCodec = s.Codec
				matchedLanguage = s.Language
				break
			}
			subIdx++
		}
		// We successfully enumerated streams, so the requested track must be a
		// real embedded subtitle. An out-of-range ?track= would otherwise become
		// `-map 0:s:99` and fail the ffmpeg spawn per request (forged deep index).
		if !matched {
			span.SetStatus(otelcodes.Error, "subtitle track out of range")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		// A bitmap subtitle carries no text, so WebVTT is not an option: `-f
		// webvtt` on one produces an empty or failed stream. It is served as
		// DECODED IMAGES instead (see subtitle_pgs.go), which is what lets the
		// player draw it without the server burning it into the video - and a
		// burn-in is what denies the copy rung and turns a direct-playing remux
		// into a full transcode.
		//
		// PGS only: VobSub is a bitmap too, but nothing here decodes it, and
		// answering 200 with an empty cue list would put a subtitle in the menu
		// that draws nothing. It keeps the old refusal.
		if imageSub {
			if !pgsServable(matchedCodec) {
				span.SetStatus(otelcodes.Error, "bitmap subtitle not decodable here")
				http.Error(w, `{"message":"subtitle not available as text"}`, http.StatusUnprocessableEntity)
				return
			}
			serveBitmapSubtitle(w, r, span, cache, filePath, trackIndex, mediaID)
			return
		}

		// Cached? Producing this costs a full read of the film - 71s measured on
		// a real server for 7 KB of text - and the answer never changes while
		// the file does not. See subtitle_cache.go.
		cacheKey := cache.keyFor(filePath, trackIndex)
		if data, ok := cache.get(cacheKey); ok {
			writeSubtitleVTT(w, r, baseURL, data)
			return
		}

		// One extraction at a time per track. A player that retries what it
		// believes is a hung request - which a 71-second wait invites - would
		// otherwise start a second full read of the same film.
		produced, err := runSharedSubtitleExtraction(ctx, subtitleFlightKey(cacheKey, filePath), filePath, func(ctx context.Context) ([]byte, error) {
			// Re-check under the flight: the request we queued behind may have
			// just filled the cache.
			if data, ok := cache.get(cacheKey); ok {
				return data, nil
			}
			data, perr := extractSubtitleVTT(ctx, filePath, trackIndex, matchedCodec, matchedLanguage)
			if perr != nil {
				return nil, perr
			}
			if cerr := cache.put(cacheKey, data); cerr != nil {
				// Not fatal: the viewer gets their subtitle, the next one pays
				// for it again. Logged because a cache that never stores turns
				// every request back into a 71-second wait.
				slog.WarnContext(ctx, "subtitle: cache store failed",
					slog.String("media_id", mediaID), slog.Any("err", cerr))
			}
			return data, nil
		})
		if err == nil && len(produced) > 0 {
			writeSubtitleVTT(w, r, baseURL, produced)
			return
		}
		slog.WarnContext(ctx, "subtitle: extraction failed",
			slog.String("media_id", mediaID), slog.Int("track", trackIndex), slog.Any("err", err))
		http.Error(w, `{"message":"could not extract this subtitle"}`, http.StatusInternalServerError)
	}
}

// writeSubtitleVTT sends an already-produced WebVTT body.
func writeSubtitleVTT(w http.ResponseWriter, r *http.Request, baseURL string, data []byte) {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	// The bytes are pinned to the source file's size and mtime by the cache key,
	// so a client may keep them: re-fetching costs a full read of the film.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	setCORSHeaders(w, r, baseURL)
	if _, err := w.Write(data); err != nil {
		slog.WarnContext(r.Context(), "subtitle: response write failed", slog.Any("err", err))
	}
}

// extractSubtitleVTT produces the WebVTT for one embedded track, mkvgo first and
// ffmpeg as the fallback. Buffered, never streamed: a failed extraction must
// leave no partial response, and the result has to be whole before it can be
// cached.
func extractSubtitleVTT(ctx context.Context, filePath string, trackIndex int, matchedCodec, matchedLanguage string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if isMkvgoSubtitleFastPath(ext) {
		if data, ok := tryMKVGoSubtitleBytes(ctx, ext, filePath, trackIndex, matchedCodec, matchedLanguage); ok {
			return data, nil
		}
	}
	return extractSubtitleWithFFmpeg(ctx, filePath, trackIndex)
}

// extractSubtitleWithFFmpeg is the fallback for containers mkvgo does not read
// (MPEG-TS) and for any file its guards refuse.
func extractSubtitleWithFFmpeg(ctx context.Context, filePath string, trackIndex int) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Interactive, Site: "http.subtitle", NoNice: true}, "ffmpeg",
		"-i", filePath,
		"-map", fmt.Sprintf("0:s:%d", trackIndex),
		"-f", "webvtt",
		"-",
	)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg subtitle refused: %w", err)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg subtitle start: %w", err)
	}
	if err := finish(cmd.Wait(), nil); err != nil {
		return nil, fmt.Errorf("ffmpeg subtitle: %w", err)
	}
	return buf.Bytes(), nil
}

// isMkvgoSubtitleFastPath reports whether ext (already lower-cased by the caller
// via strings.ToLower) is a container for which mkvgo can extract embedded text
// subtitle tracks in-process (no ffmpeg fork): EBML (.mkv/.webm) via the
// reader/ops packages, and ISO-BMFF (.mp4/.m4v/.mov) tx3g/wvtt via the mp4
// package (.mov since mkvgo v0.12.0's QuickTime support - the codec guard plus
// buffer-first extraction means any parse failure falls back to ffmpeg). .ts is
// out of mkvgo's scope (MPEG-TS is neither EBML nor ISO-BMFF) and stays on the
// ffmpeg fork.
func isMkvgoSubtitleFastPath(ext string) bool {
	switch ext {
	case ".mkv", ".webm", ".mp4", ".m4v", ".mov":
		return true
	default:
		return false
	}
}

// isMkvgoMP4Ext reports whether ext is an ISO-BMFF container served by the mkvgo
// mp4 package rather than the EBML reader/ops packages.
func isMkvgoMP4Ext(ext string) bool {
	return ext == ".mp4" || ext == ".m4v" || ext == ".mov"
}

// openMkvgoSubtitleContainer opens filePath head-only through the mkvgo package
// matching ext, returning the Matroska-shaped container both packages produce.
func openMkvgoSubtitleContainer(ctx context.Context, ext, filePath string) (*extmkv.Container, error) {
	if isMkvgoMP4Ext(ext) {
		c, _, err := mkvgomp4.OpenMeta(ctx, filePath)
		return c, err
	}
	return reader.OpenMeta(ctx, filePath)
}

// extractMkvgoSubtitleWebVTT extracts the subtitle track identified by trackID
// (a 1-based Container.Tracks ID) as WebVTT, routing to the mp4 or EBML package
// per ext.
func extractMkvgoSubtitleWebVTT(ctx context.Context, ext, filePath string, trackID uint64, w io.Writer) error {
	if isMkvgoMP4Ext(ext) {
		// MP4 needs no index: its sample table already is one, which is why
		// this path answers in milliseconds where Matroska takes a full read.
		return mkvgomp4.ExtractSubtitleWebVTT(ctx, filePath, trackID, w)
	}
	// A stored index turns the walk into a handful of seeks. A stale or absent
	// one just means the slow path, which is the correct answer either way -
	// mkvgo refuses an index that does not match this exact file rather than
	// read cues out of the wrong bytes.
	// Paced while a stream is being served - see subtitle_read_pacer.go.
	opts := subtitleReadOptions(ctx)
	if ix, ok := subtitleIndexes.get(filePath); ok {
		if err := matroska.ExtractSubtitleWebVTTFrom(ctx, filePath, trackID, ix, w, opts...); err == nil {
			return nil
		}
	}
	return matroska.ExtractSubtitleWebVTT(ctx, filePath, trackID, w, opts...)
}

// mkvgoTextSubtitleTrackIDs returns the mkvgo track IDs of the TEXT subtitle
// tracks, in declaration order, alongside their subtitle-relative ordinal.
//
// Two numbering schemes meet here: the route and the cache count subtitles among
// themselves (0, 1, 2...), mkvgo addresses them by container track ID. The index
// is built from the latter.
func mkvgoTextSubtitleTrackIDs(ctx context.Context, ext, filePath string) (map[int]uint64, error) {
	c, err := openMkvgoSubtitleContainer(ctx, ext, filePath)
	if err != nil {
		return nil, fmt.Errorf("open container: %w", err)
	}
	out := make(map[int]uint64)
	subIdx := 0
	for i := range c.Tracks {
		if c.Tracks[i].Type != extmkv.SubtitleTrack {
			continue
		}
		cur := subIdx
		subIdx++
		if apptranscoding.IsImageSubtitle(extmkv.FFprobeCodecName(c.Tracks[i].Codec)) {
			// Never served as text, so never worth an index entry.
			continue
		}
		out[cur] = c.Tracks[i].ID
	}
	return out, nil
}

// subtitleLanguageMatches reports whether the DB's language tag and mkvgo's can
// denote the same track language. The two sides speak different conventions -
// the DB stores ffprobe's BCP-47-ish tag ("fr-FR"), a Matroska language element
// is usually a bare ISO 639-2 code ("fre") - so raw equality rejected the track
// on EVERY playback of such a file (prod 2026-07-26: db_lang=fr-FR vs
// mkvgo_lang=fre, permanent silent fallback to the ffmpeg fork). Primary
// subtags fold through langcode.Normalize; region subtags only disqualify when
// BOTH sides carry one, so a genuine ordinal drift between fr-FR and fr-CA
// tracks in the same file is still caught while a region-less "fre" matches
// either. Empty on either side means "unknown" and never rejects.
func subtitleLanguageMatches(dbLang, mkvLang string) bool {
	if dbLang == "" || mkvLang == "" {
		return true
	}
	dbPrimary, dbRegion := splitLangTag(dbLang)
	mkvPrimary, mkvRegion := splitLangTag(mkvLang)
	if langcode.Normalize(dbPrimary) != langcode.Normalize(mkvPrimary) {
		return false
	}
	return dbRegion == "" || mkvRegion == "" || dbRegion == mkvRegion
}

// splitLangTag lower-cases tag and splits it at the first '-'/'_' into the
// primary language subtag and the rest (region/script/variant, compared as an
// opaque suffix by subtitleLanguageMatches).
func splitLangTag(tag string) (primary, rest string) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		return tag[:i], tag[i+1:]
	}
	return tag, ""
}

// tryMKVGoSubtitle attempts to serve embedded subtitle trackIndex from the
// container at filePath (an EBML or ISO-BMFF file, per ext) as WebVTT, in-process
// via mkvgo. It returns true only when the subtitle was successfully buffered and
// written to w; false in every other case (caller should fall back to ffmpeg). It
// never writes a partial body: extraction into a local bytes.Buffer happens first,
// and w is only touched on success. The codec guard (DB value matchedCodec vs
// mkvgo FFprobeCodecName) ensures we never serve a track whose identity has
// drifted from the DB record.
func tryMKVGoSubtitleBytes(ctx context.Context,
	ext, filePath string, trackIndex int, matchedCodec, matchedLanguage string) ([]byte, bool) {

	c, err := openMkvgoSubtitleContainer(ctx, ext, filePath)
	if err != nil {
		slog.WarnContext(ctx, "subtitle: mkvgo open failed, falling back to ffmpeg",
			slog.String("path", filePath), slog.Any("err", err))
		return nil, false
	}

	// Enumerate subtitle tracks in declaration order; pick the trackIndex-th.
	subIdx := 0
	var mkvTrack *extmkv.Track
	for i := range c.Tracks {
		if c.Tracks[i].Type != extmkv.SubtitleTrack {
			continue
		}
		if subIdx == trackIndex {
			mkvTrack = &c.Tracks[i]
			break
		}
		subIdx++
	}
	if mkvTrack == nil {
		slog.WarnContext(ctx, "subtitle: mkvgo subtitle track index out of range, falling back to ffmpeg",
			slog.String("path", filePath), slog.Int("track_index", trackIndex))
		return nil, false
	}

	// Codec guard: the DB stores the ffprobe codec_name; mkvgo uses its own short
	// name. FFprobeCodecName translates the short name to the ffprobe form. A
	// mismatch means the DB ordinal has drifted from mkvgo's ordinal (e.g. the
	// file was edited and the library was not re-scanned). Falling back to ffmpeg
	// is the safe option - never serve a track that doesn't match the DB entry.
	mkvCodec := extmkv.FFprobeCodecName(mkvTrack.Codec)
	if mkvCodec != matchedCodec {
		slog.WarnContext(ctx, "subtitle: mkvgo codec mismatch vs DB, falling back to ffmpeg",
			slog.String("path", filePath),
			slog.String("db_codec", matchedCodec),
			slog.String("mkvgo_codec", mkvCodec))
		return nil, false
	}

	// Language guard: only enforced when both sides carry a non-empty tag.
	// Empty language on either side means "unknown" - don't reject.
	if !subtitleLanguageMatches(matchedLanguage, mkvTrack.Language) {
		slog.WarnContext(ctx, "subtitle: mkvgo language mismatch vs DB, falling back to ffmpeg",
			slog.String("path", filePath),
			slog.String("db_lang", matchedLanguage),
			slog.String("mkvgo_lang", mkvTrack.Language))
		return nil, false
	}

	// Buffer-first: populate a local buffer before touching w. A failed extraction
	// therefore produces no partial response.
	var buf bytes.Buffer
	if err := extractMkvgoSubtitleWebVTT(ctx, ext, filePath, mkvTrack.ID, &buf); err != nil {
		slog.WarnContext(ctx, "subtitle: mkvgo WebVTT extraction failed, falling back to ffmpeg",
			slog.String("path", filePath), slog.Any("err", err))
		return nil, false
	}

	return buf.Bytes(), true
}

func serveExternalSubtitle(w http.ResponseWriter, r *http.Request, path, baseDir, baseURL string) {
	safePath, err := safeJoin(baseDir, filepath.Base(path))
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	ext := strings.ToLower(filepath.Ext(safePath))
	setCORSHeaders(w, r, baseURL)
	w.Header().Set("Cache-Control", "no-cache")
	if ext == ".vtt" {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		http.ServeFile(w, r, safePath)
		return
	}
	// .srt, .ass and .ssa convert in-process via mkvgo - no ffmpeg fork per
	// request. Buffer the result first so a parse error can still fall back to
	// ffmpeg without having already written a partial body.
	if ext == ".srt" || ext == ".ass" || ext == ".ssa" {
		var buf bytes.Buffer
		mkvErr := mkvsubtitle.FileToWebVTT(safePath, &buf)
		if mkvErr == nil {
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			_, _ = io.Copy(w, &buf)
			return
		}
		slog.WarnContext(r.Context(), "mkvgo subtitle->webvtt failed, falling back to ffmpeg",
			slog.String("path", safePath), slog.String("ext", ext), slog.Any("err", mkvErr))
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Interactive, Site: "http.external_subtitle", NoNice: true}, "ffmpeg", "-i", safePath, "-f", "webvtt", "-")
	if err != nil {
		slog.WarnContext(r.Context(), "ffmpeg external subtitle refused", slog.Any("err", err))
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		slog.ErrorContext(r.Context(), "ffmpeg external subtitle", slog.Any("err", err))
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered in external subtitle ffmpeg cancel", "error", r, "stack", string(debug.Stack()))
			}
		}()
		<-r.Context().Done()
		cancel()
	}()
	if err := finish(cmd.Wait(), nil); err != nil && ctx.Err() == nil {
		slog.WarnContext(r.Context(), "ffmpeg external subtitle convert", slog.String("path", safePath), slog.Any("err", err))
	}
}

// --- registerSubtitleRoutes ---

type subtitleRoutes struct {
	subtitleSvc   subtitleService
	tokens        TokenVerifier
	mediaGetter   MediaItemGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
}

// providerRL rate-limits the two routes that reach the subtitle provider on the
// operator's own API key. Unlike the local list/delete routes, these spend a
// finite third-party budget - OpenSubtitles caps downloads per key per day - so
// an authenticated user could otherwise drain the whole allowance for everyone
// on the server. nil leaves the routes unlimited (tests).
// parental is a required positional parameter on purpose. Making it variadic
// would let a future call site omit it and silently serve an ungated route -
// which is precisely the forgotten-twin failure this gate exists to close, and
// it has already bitten this package more than once. A caller with no checker
// must pass nil explicitly, so the omission is a deliberate, visible choice.
func registerSubtitleRoutes(mux *http.ServeMux, subtitleSvc *appsubtitle.Service, tokenGen TokenVerifier, mediaGetter MediaItemGetter, libraryAccess LibraryAccessChecker, providerRL *ratelimit.Limiter, parental ParentalChecker) {
	h := &subtitleRoutes{subtitleSvc: subtitleSvc, tokens: tokenGen, mediaGetter: mediaGetter, libraryAccess: libraryAccess, parental: parental}
	limited := func(next http.HandlerFunc) http.HandlerFunc {
		if providerRL == nil {
			return next
		}
		return ratelimit.MiddlewareFunc(providerRL, next)
	}
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", limited(h.search))
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", limited(h.download))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)
}

func (h *subtitleRoutes) requireAuth(w http.ResponseWriter, r *http.Request) (*domainuser.Claims, bool) {
	tok := r.Header.Get("Authorization")
	if strings.HasPrefix(tok, "Bearer ") {
		tok = tok[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		tok = t
	} else {
		tok = ""
	}
	if tok == "" {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokens.ValidateAccessToken(tok)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) requireMediaAccess(w http.ResponseWriter, r *http.Request, mediaID string) (*domainuser.Claims, bool) {
	claims, ok := h.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	item, err := h.mediaGetter.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return nil, false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) search(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Search")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	language := r.URL.Query().Get("language")
	span.SetAttributes(attribute.String("language", language))
	results, err := h.subtitleSvc.Search(r.Context(), mediaID, language)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "search failed")
		slog.ErrorContext(r.Context(), "subtitle search", slog.Any("err", err))
		http.Error(w, `{"message":"search failed"}`, http.StatusInternalServerError)
		return
	}
	type resultJSON struct {
		FileID   int     `json:"fileId"`
		FileName string  `json:"fileName"`
		Language string  `json:"language"`
		Format   string  `json:"format"`
		Rating   float64 `json:"rating"`
	}
	out := make([]resultJSON, 0, len(results))
	for _, res := range results {
		out = append(out, resultJSON{
			FileID:   res.FileID,
			FileName: res.FileName,
			Language: res.Language,
			Format:   res.Format,
			Rating:   res.Rating,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"results": out}); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) download(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Download")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	claims, ok := h.requireMediaAccess(w, r, mediaID)
	if !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	var body struct {
		FileID   int    `json:"fileId"`
		Language string `json:"language"`
		Format   string `json:"format"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "bad request")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.FileID == 0 {
		span.SetStatus(otelcodes.Error, "fileId required")
		http.Error(w, `{"message":"fileId required"}`, http.StatusBadRequest)
		return
	}
	sub, err := h.subtitleSvc.Download(r.Context(), mediaID, body.FileID, body.Language, body.Format)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "download failed")
		slog.ErrorContext(r.Context(), "subtitle download", slog.Any("err", err))
		http.Error(w, `{"message":"download failed"}`, http.StatusInternalServerError)
		return
	}
	out := map[string]any{
		"id":           sub.ID,
		"mediaItemId":  sub.MediaItemID,
		"language":     sub.Language,
		"format":       sub.Format,
		"source":       sub.Source,
		"downloadedAt": sub.DownloadedAt.UTC().Format(time.RFC3339),
	}
	// filePath is a server-side filesystem path; only admins may see it.
	if claims != nil && claims.IsAdmin {
		out["filePath"] = sub.FilePath
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) list(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.List")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	claims, ok := h.requireMediaAccess(w, r, mediaID)
	if !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list failed")
		slog.ErrorContext(r.Context(), "subtitle list", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type subJSON struct {
		ID           string `json:"id"`
		MediaItemID  string `json:"mediaItemId"`
		Language     string `json:"language"`
		Format       string `json:"format"`
		FilePath     string `json:"filePath,omitempty"`
		Source       string `json:"source"`
		DownloadedAt string `json:"downloadedAt"`
	}
	// filePath is a server-side filesystem path; only expose it to admins.
	isAdmin := claims != nil && claims.IsAdmin
	out := make([]subJSON, 0, len(subs))
	for _, s := range subs {
		j := subJSON{
			ID:           s.ID,
			MediaItemID:  string(s.MediaItemID),
			Language:     s.Language,
			Format:       s.Format,
			Source:       s.Source,
			DownloadedAt: s.DownloadedAt.UTC().Format(time.RFC3339),
		}
		if isAdmin {
			j.FilePath = s.FilePath
		}
		out = append(out, j)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"subtitles": out}); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) delete(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Delete")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	subtitleID := r.PathValue("subtitleId")
	span.SetAttributes(
		attribute.String("media_item_id", mediaID),
		attribute.String("subtitle_id", subtitleID),
	)
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list failed")
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	found := false
	for _, s := range subs {
		if s.ID == subtitleID {
			found = true
			break
		}
	}
	if !found {
		span.SetStatus(otelcodes.Error, "subtitle not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if err := h.subtitleSvc.Delete(r.Context(), subtitleID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "delete failed")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
