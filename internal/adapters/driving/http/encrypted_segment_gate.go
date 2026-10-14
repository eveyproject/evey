package httpapi

import (
	"regexp"
	"sync"
)

// maxEncryptedSegmentsPerClient bounds the encrypted on-demand media segments
// one client has in flight. An encrypted segment is never streamed: mkvgo
// builds and encrypts it whole for each request, and a Range in its middle
// costs the whole encryption again, AES-CBC chaining from the start. A player
// holds one video and one audio segment at a time; 4 leaves room for a second
// device on the same profile, and keeps one client's Range requests from
// taking every core.
const maxEncryptedSegmentsPerClient = 4

// planMediaSegmentRe matches the packager's media segment names: the video's
// "seg00001.m4s" and an audio rendition's "seg_a1_1.m4s". Playlists, inits,
// subtitles and the key are not encrypted segments.
var planMediaSegmentRe = regexp.MustCompile(`^seg(\d{5}|_a\d+_\d+)\.m4s$`)

func isPlanMediaSegment(name string) bool { return planMediaSegmentRe.MatchString(name) }

// clientSegmentGate counts the requests each client (streamingRateLimitKey)
// has in flight and refuses one past the limit. It never waits: a refused
// request is answered 503 + Retry-After, which hls.js and Media3 retry.
type clientSegmentGate struct {
	max      int
	mu       sync.Mutex
	inflight map[string]int
}

func newClientSegmentGate(maxInFlight int) *clientSegmentGate {
	return &clientSegmentGate{max: maxInFlight, inflight: map[string]int{}}
}

// acquire takes a slot for key; ok is false when key already holds max. The
// release it returns frees the slot, once.
func (g *clientSegmentGate) acquire(key string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight[key] >= g.max {
		return nil, false
	}
	g.inflight[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.inflight[key]--; g.inflight[key] <= 0 {
				delete(g.inflight, key)
			}
		})
	}, true
}
