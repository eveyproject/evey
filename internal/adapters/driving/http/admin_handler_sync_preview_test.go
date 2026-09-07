package httpapi

import (
	"testing"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// The sync preview is used in a loop - watch a clip, nudge the candidate
// offset, watch again - which is why it stopped sharing AdminHeavyRL: ten
// renders a minute ran out mid-judgement and, being a shared bucket, took
// trickplay generation and the search reindex down with it. Its own bucket
// must be preferred, and the route must never end up unlimited: every render
// forks an ffmpeg.
func TestSyncPreviewLimiterPrefersItsOwnBucket(t *testing.T) {
	own := ratelimit.NewWithBurst(40, 30.0)
	defer own.Stop()
	heavy := ratelimit.NewWithBurst(10, 10.0)
	defer heavy.Stop()

	s := &HTTPServer{AdminHeavyRL: heavy, SyncPreviewRL: own}
	if got := s.syncPreviewLimiter(); got != own {
		t.Fatalf("wired preview limiter not used: got %p, want %p", got, own)
	}

	// An embedder that never set the field falls back to the shared heavy one
	// rather than to nil - a nil limiter is an unlimited ffmpeg spawn route.
	s = &HTTPServer{AdminHeavyRL: heavy}
	if got := s.syncPreviewLimiter(); got != heavy {
		t.Fatalf("fallback limiter: got %p, want %p", got, heavy)
	}
}
