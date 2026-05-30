package transcoding

import (
	"context"
	"time"
)

// CapOverride is a CMCD-feedback-driven downward override on the
// benchmark-computed cap. The OverrideWorker writes one per source
// class when sustained starvation is observed; the HLS-VOD handler
// applies effective_cap = MIN(benchmark_cap, override_cap) at
// request time.
//
// SourceClass is the bucket the worker recognises — Phase C MVP uses
// height buckets only ("1080p", "1440p", "2160p"). Finer granularity
// (per-codec, per-title) is intentionally deferred until real data
// shows the coarse buckets aren't precise enough.
type CapOverride struct {
	SourceClass  string    `json:"source_class"`
	MaxHeight    int       `json:"max_height"`
	Reason       string    `json:"reason"`
	StarvedRatio float64   `json:"starved_ratio"`
	SamplesCount int       `json:"samples_count"`
	AppliedAt    time.Time `json:"applied_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SourceClassFor maps a source height to the cap-bucket the override
// system reasons about. Heights below 1080p don't get a bucket — we
// never cap below 720p in the handler so there's nothing to override.
func SourceClassFor(sourceHeight int) string {
	switch {
	case sourceHeight >= 2160:
		return "2160p"
	case sourceHeight >= 1440:
		return "1440p"
	case sourceHeight >= 1080:
		return "1080p"
	default:
		return ""
	}
}

// CapOverrideRepo persists CapOverride rows. Phase C uses postgres;
// the port keeps tests cheap (in-memory fake).
type CapOverrideRepo interface {
	// Upsert writes/refreshes an override row. ExpiresAt determines TTL.
	Upsert(ctx context.Context, o CapOverride) error
	// List returns every non-expired override. The handler calls this
	// on each request — implementations SHOULD cache + invalidate on
	// Upsert if the table grows.
	List(ctx context.Context) ([]CapOverride, error)
	// DeleteExpired removes rows past expires_at. Worker runs this
	// periodically so the table stays small.
	DeleteExpired(ctx context.Context) (int, error)
}
