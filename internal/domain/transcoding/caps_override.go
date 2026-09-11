package transcoding

import (
	"context"
	"time"
)

// CapOverride is a CMCD-feedback-driven DOWNWARD override on the benchmark
// cap, one per source class ("<codec>/<height>", e.g. "hevc/2160p"), written
// by the OverrideWorker on sustained starvation; the handler applies
// MIN(benchmark_cap, override_cap). Legacy rows ("1080p") still parse.
type CapOverride struct {
	SourceClass  string    `json:"source_class"`
	MaxHeight    int       `json:"max_height"`
	Reason       string    `json:"reason"`
	StarvedRatio float64   `json:"starved_ratio"`
	SamplesCount int       `json:"samples_count"`
	AppliedAt    time.Time `json:"applied_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SourceClassFor maps (codec, height) to the override bucket: "hevc/2160p",
// "any/1440p" for an unknown codec, "" below 1080p (the handler never caps
// below 720p, so there is nothing to override).
func SourceClassFor(sourceCodec string, sourceHeight int) string {
	var heightBucket string
	switch {
	case sourceHeight >= 2160:
		heightBucket = "2160p"
	case sourceHeight >= 1440:
		heightBucket = "1440p"
	case sourceHeight >= 1080:
		heightBucket = "1080p"
	default:
		return ""
	}
	codec := normaliseSourceCodec(sourceCodec)
	return codec + "/" + heightBucket
}

// SourceClassMatchesLegacy reports a pre-codec row ("1080p", "1440p", "2160p").
func SourceClassMatchesLegacy(class string) bool {
	switch class {
	case "1080p", "1440p", "2160p":
		return true
	}
	return false
}

// normaliseSourceCodec wraps CanonicalVideoCodec with the "any" fallback so
// empty/unrecognised codecs do not each fork their own bucket.
func normaliseSourceCodec(codec string) string {
	if c := CanonicalVideoCodec(codec); c != "" {
		return c
	}
	return "any"
}

// CapOverrideRepo persists CapOverride rows.
type CapOverrideRepo interface {
	// Upsert writes/refreshes an override row. ExpiresAt determines TTL.
	Upsert(ctx context.Context, o CapOverride) error
	// List returns every non-expired override (called per request: cache it).
	List(ctx context.Context) ([]CapOverride, error)
	// DeleteExpired removes rows past expires_at.
	DeleteExpired(ctx context.Context) (int, error)
	// Delete removes one source-class override; true when a row was removed.
	Delete(ctx context.Context, sourceClass string) (bool, error)
	// AggregateStarvation counts buffer-starved vs total QoE events per source
	// class over window, keeping buckets with at least minSamples events.
	AggregateStarvation(ctx context.Context, window time.Duration, minSamples int) ([]CapAggregateRow, error)
}

// CapAggregateRow is one (source-class, starvation) bucket returned by AggregateStarvation.
type CapAggregateRow struct {
	SourceClass string
	Starved     int64
	Total       int64
}
