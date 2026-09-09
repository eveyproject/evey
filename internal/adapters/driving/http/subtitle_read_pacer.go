package httpapi

// subtitle_read_pacer.go - a subtitle extraction reads the whole film; while
// somebody is watching one, it must not read it as fast as the disk allows.
//
// Every extraction here (PGS decode, WebVTT, the subtitle index) is a full
// sequential pass over the container, done in-process by mkvgo. The prewarmer
// gives the picture a head start and runs one extraction at a time, but nothing
// bounded the READ RATE of an extraction once it ran - and the on-demand path
// (a player asking for the track flagged DEFAULT the moment playback starts)
// does not even wait for the head start. Prod, 2026-09-09: a 70 GB 4K source
// being read flat out for its PGS track next to the viewer's GPU rafale and two
// audio encoders, on spinning disks behind mergerfs - 87 % iowait, segment
// timeouts, a minute of spinner on an audio-track switch. ionice would not have
// helped: behind a FUSE mount the kernel sees the daemon's I/O, not ours.
//
// So the pacing is in the reader. mkvgo opens the source through an injectable
// FS; the one handed here wraps the file in a reader that, WHILE PLAYBACK IS
// ACTIVE, is held to subtitleReadRateWhilePlaying. Idle box: unpaced, the
// extraction takes what the disk gives.
//
// "Playback is active" is read off the streaming byte meter: bytes served by
// the streaming routes over the last sampling window. It covers every way a
// film leaves this server - ffmpeg HLS, the mkvgo on-demand plan, direct play,
// downloads - without any of them having to report anything.

import (
	"context"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"

	"github.com/evey-project/evey/internal/pkg/netmeter"
)

// subtitleReadPacer is the process-wide pacer, set at wiring time next to
// subtitleIndexes. nil (tests, no meter) = unpaced, byte-identical to before.
var subtitleReadPacer *mediaReadPacer

// subtitleReadRateWhilePlaying is what an extraction may read per second while
// a stream is being served. Sized for spinning disks: a 4K HDR source plays at
// ~9 MB/s and its GPU rafale reads at twice that, the on-demand audio rendition
// another share, and a mergerfs of HDDs delivers ~60-80 MB/s to several
// sequential readers at once. 24 MB/s leaves the picture the larger half and
// still finishes a 15 GB 1080p remux in ~10 minutes; the deadline scales with
// the file (subtitleExtractionDeadline) so a 70 GB film is not cut off.
const subtitleReadRateWhilePlaying = 24 << 20

// subtitleReadBurst is the token-bucket depth: one large read must never wait
// for more than a burst's worth, and mkvgo reads in bounded chunks.
const subtitleReadBurst = 4 << 20

// streamActiveThreshold is the served-bytes rate above which the box counts as
// playing something. Well under the leanest stream this server emits (a 720p
// rung is ~250 KB/s) and well above the trickle of playlist refreshes.
const streamActiveThreshold = 128 << 10 // bytes per second

// streamSampleInterval is how often the meter is re-read. The decision only
// has to track "is someone watching", which changes on a human time scale -
// and the window must span more than one segment fetch, or a player pausing
// its fetches on a full buffer would read as idle between two segments.
const streamSampleInterval = 4 * time.Second

type mediaReadPacer struct {
	meter   *netmeter.Meter
	limiter *rate.Limiter

	mu             sync.Mutex
	sampleInterval time.Duration
	sampledAt      time.Time
	sampledTotal   int64
	active         bool
}

// newMediaReadPacer builds a pacer over the streaming meter. A nil meter
// yields a pacer that never paces.
func newMediaReadPacer(meter *netmeter.Meter, bytesPerSec int) *mediaReadPacer {
	return &mediaReadPacer{
		meter:          meter,
		limiter:        rate.NewLimiter(rate.Limit(bytesPerSec), subtitleReadBurst),
		sampleInterval: streamSampleInterval,
	}
}

// playbackActive reports whether the streaming routes served bytes at a
// playback-like rate over the last sampling window. The first call after
// construction only takes a baseline and answers false.
func (p *mediaReadPacer) playbackActive(now time.Time) bool {
	if p == nil || p.meter == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sampledAt.IsZero() {
		p.sampledAt, p.sampledTotal = now, p.meter.Total()
		return false
	}
	elapsed := now.Sub(p.sampledAt)
	if elapsed < p.sampleInterval {
		return p.active
	}
	total := p.meter.Total()
	p.active = float64(total-p.sampledTotal)/elapsed.Seconds() >= streamActiveThreshold
	p.sampledAt, p.sampledTotal = now, total
	return p.active
}

// throttle holds the caller until n more bytes fit the rate, when playback is
// active. A cancelled ctx stops the wait; the read already happened, and the
// caller's next mkvgo step observes the cancellation itself.
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

// options is what every mkvgo extraction call receives: an FS whose reads go
// through the pacer. A nil pacer hands back nothing, and mkvgo opens the file
// itself exactly as before.
func (p *mediaReadPacer) options(ctx context.Context) []matroska.Options {
	if p == nil {
		return nil
	}
	return []matroska.Options{{FS: &extmkv.FS{
		Open: func(path string) (extmkv.ReadSeekCloser, error) {
			f, err := os.Open(path) //nolint:gosec // the path is the library file mkvgo was asked to read
			if err != nil {
				return nil, err
			}
			return &pacedReader{ReadSeekCloser: f, pacer: p, ctx: ctx}, nil
		},
	}}}
}

// subtitleReadOptions is the call-site helper: the process-wide pacer's
// options, or none.
func subtitleReadOptions(ctx context.Context) []matroska.Options {
	return subtitleReadPacer.options(ctx)
}

type pacedReader struct {
	extmkv.ReadSeekCloser
	pacer *mediaReadPacer
	ctx   context.Context
}

func (r *pacedReader) Read(b []byte) (int, error) {
	n, err := r.ReadSeekCloser.Read(b)
	if n > 0 {
		r.pacer.throttle(r.ctx, n)
	}
	return n, err
}
