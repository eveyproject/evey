package httpapi

// subtitle_prewarm.go - extracting a film's subtitle tracks BEFORE anyone asks
// for them.
//
// Producing one costs a full read of the container: 82 seconds measured on a
// 15 GB file, and the same again for every other track. The cache next door
// means that price is paid once - but somebody still pays it, and it is whoever
// presses "subtitles" first. From their seat the button simply does nothing.
//
// So the work moves off the viewer's request and onto playback start: by the
// time anyone opens the subtitle menu, the tracks are already on disk. This is
// what makes Jellyfin look fast at this - not a quicker extractor (ffmpeg takes
// 79s where mkvgo takes 89s on the same file), but extracting earlier.
//
// Two rules govern it, both from the operator of a weak NAS:
//
//   - the picture comes first. Nothing starts until the stream has had a head
//     start, and every extraction runs in procfork's Scan class: ionice idle
//     plus nice 19, so it yields the disk to the segments being served;
//   - one at a time, in order. Extractions are serialized on a single worker,
//     because two concurrent full reads of the same file on a spinning NAS are
//     slower than one after the other, and would compete with playback twice.

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/library"
)

// subtitlePrewarmDelay is the head start the picture gets. A player spends the
// first seconds filling its buffer, which is exactly when the disk must be left
// alone; nobody opens the subtitle menu in that window either.
const subtitlePrewarmDelay = 20 * time.Second

// subtitlePrewarmQueue bounds what is waiting. Deliberately small: this is a
// convenience, and a backlog of films nobody is watching any more is not worth
// keeping. A full queue drops the job - the on-demand path still works.
const subtitlePrewarmQueue = 32

type prewarmJob struct {
	mediaID  string
	filePath string
	tracks   []prewarmTrack
}

type prewarmTrack struct {
	// index is the SUBTITLE-relative ordinal, the one the route and the cache
	// key count in.
	index    int
	codec    string
	language string
}

// subtitlePrewarmer extracts a media's text subtitle tracks in the background.
// The zero value is not usable; see newSubtitlePrewarmer.
type subtitlePrewarmer struct {
	cache   subtitleCache
	indexes subtitleIndexCache
	jobs    chan prewarmJob
	// seen keeps a media from being queued again for the life of the process:
	// a player re-requests the master playlist on every seek, quality change
	// and token refresh, and each one would otherwise enqueue the same film.
	mu   sync.Mutex
	seen map[string]struct{}
	// delay is a field so tests do not wait 20 seconds.
	delay time.Duration
}

func newSubtitlePrewarmer(cache subtitleCache, indexes subtitleIndexCache) *subtitlePrewarmer {
	return &subtitlePrewarmer{
		cache:   cache,
		indexes: indexes,
		jobs:    make(chan prewarmJob, subtitlePrewarmQueue),
		seen:    make(map[string]struct{}),
		delay:   subtitlePrewarmDelay,
	}
}

// Start runs the single worker until ctx is done.
func (p *subtitlePrewarmer) Start(ctx context.Context) {
	if p == nil || p.cache.dir == "" {
		// No data dir means no cache to fill: extracting would burn a full read
		// of the film and throw the result away.
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-p.jobs:
				p.run(ctx, job)
			}
		}
	}()
}

// Enqueue schedules a media's text subtitle tracks, in the order given. Never
// blocks: a full queue drops the job rather than hold up the request that
// triggered it.
//
// preferredLang, when known, is extracted first - it is the one most likely to
// be asked for. Everything else follows in file order, which is the order the
// menu lists them in.
func (p *subtitlePrewarmer) Enqueue(mediaID, filePath string, streams []*library.MediaStream, preferredLang string) {
	if p == nil || p.cache.dir == "" || mediaID == "" || filePath == "" {
		return
	}
	p.mu.Lock()
	if _, dup := p.seen[mediaID]; dup {
		p.mu.Unlock()
		return
	}
	p.seen[mediaID] = struct{}{}
	p.mu.Unlock()

	tracks := textSubtitleTracks(streams, preferredLang)
	if len(tracks) == 0 {
		return
	}
	select {
	case p.jobs <- prewarmJob{mediaID: mediaID, filePath: filePath, tracks: tracks}:
	default:
		slog.Debug("subtitle prewarm: queue full, skipping", slog.String("media_id", mediaID))
	}
}

// textSubtitleTracks lists the SUBTITLE-relative indices worth extracting, the
// preferred language first.
//
// Image tracks are skipped: they are never served as text (the route answers
// 422), so extracting them would read the film for nothing.
func textSubtitleTracks(streams []*library.MediaStream, preferredLang string) []prewarmTrack {
	var out []prewarmTrack
	idx := 0
	for _, s := range streams {
		if s == nil || s.StreamType != library.StreamTypeSubtitle {
			continue
		}
		cur := idx
		idx++
		if s.IsExternal {
			// Already a file on disk; the route serves it without extracting.
			continue
		}
		// A bitmap track used to be skipped outright: it could only be burnt
		// into the video, so extracting it ahead of time warmed nothing. It is
		// now served as decoded images, so it belongs here - and it is the track
		// that most needs the head start, being the one a retail rip flags
		// DEFAULT. VobSub stays out: nothing decodes it (see pgsServable).
		if apptranscoding.IsImageSubtitle(s.Codec) && !pgsServable(s.Codec) {
			continue
		}
		out = append(out, prewarmTrack{index: cur, codec: s.Codec, language: s.Language})
	}
	if preferredLang == "" || len(out) < 2 {
		return out
	}
	// Stable partition: the preferred language first, everything else in file
	// order behind it.
	var first, rest []prewarmTrack
	for _, t := range out {
		if subtitleLanguageMatches(preferredLang, t.language) {
			first = append(first, t)
		} else {
			rest = append(rest, t)
		}
	}
	return append(first, rest...)
}

func (p *subtitlePrewarmer) run(ctx context.Context, job prewarmJob) {
	// The picture first: let the stream fill its buffer before touching the
	// same disk.
	select {
	case <-ctx.Done():
		return
	case <-time.After(p.delay):
	}

	// ONE walk for the whole file, before extracting anything. Without it each
	// track costs its own full read: four tracks meant four traversals of the
	// container for what is one pass worth of information. With it, the walk
	// happens once and every track after is a handful of seeks.
	//
	// Matroska only - an MP4's sample table already is this index, and .avi
	// never reaches mkvgo at all.
	//
	// Only from the SECOND track on. The walk is itself a full pass, so building
	// it to serve a single extraction pays a whole traversal to save none - and
	// the extraction that follows would have read the file anyway. Measured on
	// this library (1503 files): 918 carry two text tracks or more and gain,
	// 227 carry exactly one and were paying an extra pass for nothing.
	if indexWorthBuilding(job) {
		p.ensureIndex(ctx, job)
	}

	for _, t := range job.tracks {
		if ctx.Err() != nil {
			return
		}
		bitmap := apptranscoding.IsImageSubtitle(t.codec)
		kind := subtitleKindVTT
		if bitmap {
			kind = subtitleKindPGS
		}
		key := p.cache.keyForKind(job.filePath, t.index, kind)
		if key == "" {
			continue
		}
		if _, ok := p.cache.get(key); ok {
			continue // already paid for
		}
		started := time.Now()
		// Same singleflight as the on-demand path: a viewer who asks for this
		// exact track while we are extracting it waits for THIS run instead of
		// starting a second full read of the film. The flight key carries the
		// kind through the cache key, so a text and a bitmap track of the same
		// ordinal never share a flight.
		_, err, _ := subtitleExtractions.Do(subtitleFlightKey(key, job.filePath), func() (any, error) {
			if data, ok := p.cache.get(key); ok {
				return data, nil
			}
			var (
				data []byte
				perr error
			)
			if bitmap {
				data, perr = p.extractBitmap(ctx, job, t.index)
			} else {
				data, perr = extractSubtitleVTT(ctx, job.filePath, t.index, t.codec, t.language)
			}
			if perr != nil {
				return nil, perr
			}
			return data, p.cache.put(key, data)
		})
		if err != nil {
			slog.WarnContext(ctx, "subtitle prewarm: extraction failed",
				slog.String("media_id", job.mediaID), slog.Int("track", t.index), slog.Any("err", err))
			continue
		}
		slog.InfoContext(ctx, "subtitle prewarm: track ready",
			slog.String("media_id", job.mediaID),
			slog.Int("track", t.index),
			slog.String("lang", t.language),
			slog.Duration("took", time.Since(started)))
	}
}

// startSubtitlePrewarmer builds the prewarmer and starts its worker. Returns nil
// when there is no cache to fill, so the caller's field stays nil and Enqueue is
// a no-op rather than a queue nobody drains.
func startSubtitlePrewarmer(ctx context.Context, cache subtitleCache, indexes subtitleIndexCache) *subtitlePrewarmer {
	if cache.dir == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p := newSubtitlePrewarmer(cache, indexes)
	p.Start(ctx)
	return p
}

// indexWorthBuilding reports whether this job extracts enough tracks for the
// index to pay for itself. The walk is a full pass of the container, so building
// it to serve ONE extraction pays a whole traversal to save none - the single
// extraction would have read the file anyway.
//
// Measured on a real library (1503 files): 918 carry two text tracks or more and
// gain from the index, 227 carry exactly one and were paying an extra pass for
// nothing. The remaining 358 have no text track, so no job is queued at all.
func indexWorthBuilding(job prewarmJob) bool {
	return len(job.tracks) >= 2
}

// ensureIndex builds and stores the file's subtitle index if it is missing.
// Best-effort: a failure leaves every extraction on the slow path, which is what
// happened before the index existed.
func (p *subtitlePrewarmer) ensureIndex(ctx context.Context, job prewarmJob) {
	if p.indexes.dir == "" {
		return
	}
	ext := strings.ToLower(filepath.Ext(job.filePath))
	if !isMkvgoSubtitleFastPath(ext) || isMkvgoMP4Ext(ext) {
		return
	}
	if _, ok := p.indexes.get(job.filePath); ok {
		return
	}
	ids, err := mkvgoTextSubtitleTrackIDs(ctx, ext, job.filePath)
	if err != nil {
		slog.WarnContext(ctx, "subtitle prewarm: cannot read track ids for the index",
			slog.String("media_id", job.mediaID), slog.Any("err", err))
		return
	}
	// Bitmap tracks are indexed too, now that they are served as decoded images
	// rather than burnt in. Leaving them out would build an index that the very
	// tracks needing it most cannot use, and each would then walk the file on its
	// own - the cost this index exists to pay once.
	imgIDs, imgErr := mkvgoImageSubtitleTrackIDs(ctx, ext, job.filePath)
	if imgErr != nil {
		slog.WarnContext(ctx, "subtitle prewarm: cannot read bitmap track ids for the index",
			slog.String("media_id", job.mediaID), slog.Any("err", imgErr))
	}
	// Only the tracks this job will actually extract: an index entry for a track
	// nobody reads is a walk paid for nothing.
	var trackIDs []uint64
	for _, t := range job.tracks {
		if id, ok := ids[t.index]; ok {
			trackIDs = append(trackIDs, id)
			continue
		}
		if id, ok := imgIDs[t.index]; ok {
			trackIDs = append(trackIDs, id)
		}
	}
	if len(trackIDs) == 0 {
		return
	}
	started := time.Now()
	if _, err := p.indexes.buildAndStore(ctx, job.filePath, trackIDs); err != nil {
		slog.WarnContext(ctx, "subtitle prewarm: index build failed",
			slog.String("media_id", job.mediaID), slog.Any("err", err))
		return
	}
	slog.InfoContext(ctx, "subtitle prewarm: index ready",
		slog.String("media_id", job.mediaID),
		slog.Int("tracks", len(trackIDs)),
		slog.Duration("took", time.Since(started)))
}

// extractBitmap decodes one bitmap track into the blob the route serves,
// reusing the file's subtitle index when the job built one.
//
// The index covers every track it was built for, bitmap included, so a film
// with several PGS tracks pays one walk instead of one per track - which is the
// whole reason ensureIndex runs before this loop.
func (p *subtitlePrewarmer) extractBitmap(ctx context.Context, job prewarmJob, trackIndex int) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(job.filePath))
	if !isMkvgoSubtitleFastPath(ext) || isMkvgoMP4Ext(ext) {
		return nil, fmt.Errorf("bitmap subtitles are only served from Matroska (got %q)", ext)
	}
	ids, err := mkvgoImageSubtitleTrackIDs(ctx, ext, job.filePath)
	if err != nil {
		return nil, err
	}
	trackID, ok := ids[trackIndex]
	if !ok {
		return nil, fmt.Errorf("no bitmap subtitle at ordinal %d", trackIndex)
	}
	// A missing index is not a failure: mkvgo walks the file itself. It is what
	// a single-track job deliberately does, since the walk would save nothing.
	ix, _ := p.indexes.get(job.filePath)
	return extractPGSBlob(ctx, job.filePath, trackID, ix)
}
