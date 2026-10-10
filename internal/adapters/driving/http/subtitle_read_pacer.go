package httpapi

// A subtitle extraction reads the whole film. While somebody is watching
// (ProducerSelector.ActivePlaybackCount), mkvgo's reads go through a paced
// reader (injected FS); idle, they are unpaced. ionice cannot do this behind a
// FUSE mount. Prod 2026-09-09: a 70 GB source read flat out next to the rafale.

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

// subtitleReadPacer is set by the VOD wiring; nil = unpaced.
var subtitleReadPacer *mediaReadPacer

// 24 MB/s leaves a 4K rafale (~18 MB/s) the larger half of a mergerfs of HDDs
// (~60-80 MB/s shared); the extraction deadline scales with the file size.
const subtitleReadRateWhilePlaying = 24 << 20

const subtitleReadBurst = 4 << 20 // token-bucket depth, above mkvgo's read chunks

// The activity answer is sampled, not asked per read: it sweeps a map under a lock.
const playbackSampleInterval = 2 * time.Second

type mediaReadPacer struct {
	active  func() bool
	limiter *rate.Limiter

	mu             sync.Mutex
	sampleInterval time.Duration
	sampledAt      time.Time
	sampled        bool
}

// newMediaReadPacer: a nil active func never paces.
func newMediaReadPacer(active func() bool, bytesPerSec int) *mediaReadPacer {
	return &mediaReadPacer{
		active:         active,
		limiter:        rate.NewLimiter(rate.Limit(bytesPerSec), subtitleReadBurst),
		sampleInterval: playbackSampleInterval,
	}
}

func (p *mediaReadPacer) playbackActive(now time.Time) bool {
	if p == nil || p.active == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.sampledAt.IsZero() && now.Sub(p.sampledAt) < p.sampleInterval {
		return p.sampled
	}
	p.sampled, p.sampledAt = p.active(), now
	return p.sampled
}

// throttle holds the caller until n bytes fit the rate, while playback is
// active. A cancelled ctx ends the wait; mkvgo observes the cancellation next.
func (p *mediaReadPacer) throttle(ctx context.Context, n int) {
	if p == nil || !p.playbackActive(time.Now()) {
		return
	}
	for n > 0 {
		chunk := min(n, subtitleReadBurst)
		if err := p.limiter.WaitN(ctx, chunk); err != nil {
			return
		}
		n -= chunk
	}
}

// options: an mkvgo FS whose reads go through the pacer; nil pacer = none.
func (p *mediaReadPacer) options(ctx context.Context) []matroska.Options {
	if p == nil {
		return nil
	}
	return readOptions(ctx, p)
}

// subtitleReadOptions: paced while a stream plays (nil pacer = unpaced), and
// reporting its offsets to the running flight when the context carries one,
// so a poll's Retry-After follows the walk's real pace. No pacer and no
// flight means mkvgo opens the file itself.
func subtitleReadOptions(ctx context.Context) []matroska.Options {
	if subtitleReadPacer == nil && subtitleFlightFromContext(ctx) == nil {
		return nil
	}
	return readOptions(ctx, subtitleReadPacer)
}

func readOptions(ctx context.Context, p *mediaReadPacer) []matroska.Options {
	fl := subtitleFlightFromContext(ctx)
	return []matroska.Options{{FS: &extmkv.FS{
		Open: func(path string) (extmkv.ReadSeekCloser, error) {
			f, err := os.Open(path) //nolint:gosec // the path is the library file mkvgo was asked to read
			if err != nil {
				return nil, err
			}
			return &pacedReader{ReadSeekCloser: f, pacer: p, ctx: ctx, flight: fl, handle: fl.open()}, nil
		},
	}}}
}

// pacedReader throttles the reads (nil pacer: never) and tells the flight how
// far into the file the reader is, after every read and seek.
type pacedReader struct {
	extmkv.ReadSeekCloser
	pacer  *mediaReadPacer
	ctx    context.Context
	flight *subtitleFlight
	handle int64
	pos    int64
}

func (r *pacedReader) Read(b []byte) (int, error) {
	n, err := r.ReadSeekCloser.Read(b)
	if n > 0 {
		r.pos += int64(n)
		r.flight.advance(r.handle, r.pos)
		r.pacer.throttle(r.ctx, n)
	}
	return n, err
}

func (r *pacedReader) Seek(off int64, whence int) (int64, error) {
	p, err := r.ReadSeekCloser.Seek(off, whence)
	if err == nil {
		r.pos = p
		// A probe to the end (io.SeekEnd) is not progress; only SeekStart
		// targets are where the walk really lands.
		if whence == io.SeekStart {
			r.flight.advance(r.handle, p)
		}
	}
	return p, err
}
