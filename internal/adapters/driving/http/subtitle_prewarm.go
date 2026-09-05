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
	"log/slog"
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
	cache subtitleCache
	jobs  chan prewarmJob
	// seen keeps a media from being queued again for the life of the process:
	// a player re-requests the master playlist on every seek, quality change
	// and token refresh, and each one would otherwise enqueue the same film.
	mu   sync.Mutex
	seen map[string]struct{}
	// delay is a field so tests do not wait 20 seconds.
	delay time.Duration
}

func newSubtitlePrewarmer(cache subtitleCache) *subtitlePrewarmer {
	return &subtitlePrewarmer{
		cache: cache,
		jobs:  make(chan prewarmJob, subtitlePrewarmQueue),
		seen:  make(map[string]struct{}),
		delay: subtitlePrewarmDelay,
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
		if apptranscoding.IsImageSubtitle(s.Codec) {
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

	for _, t := range job.tracks {
		if ctx.Err() != nil {
			return
		}
		key := p.cache.keyFor(job.filePath, t.index)
		if key == "" {
			continue
		}
		if _, ok := p.cache.get(key); ok {
			continue // already paid for
		}
		started := time.Now()
		// Same singleflight as the on-demand path: a viewer who asks for this
		// exact track while we are extracting it waits for THIS run instead of
		// starting a second full read of the film.
		_, err, _ := subtitleExtractions.Do(key+"|"+job.filePath, func() (any, error) {
			if data, ok := p.cache.get(key); ok {
				return data, nil
			}
			data, perr := extractSubtitleVTT(ctx, job.filePath, t.index, t.codec, t.language)
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
func startSubtitlePrewarmer(ctx context.Context, cache subtitleCache) *subtitlePrewarmer {
	if cache.dir == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p := newSubtitlePrewarmer(cache)
	p.Start(ctx)
	return p
}
