package transcoding

import (
	"context"
	"strings"
	"time"
)

// CapOverride is a CMCD-feedback-driven downward override on the
// benchmark-computed cap. The OverrideWorker writes one per source
// class when sustained starvation is observed; the HLS-VOD handler
// applies effective_cap = MIN(benchmark_cap, override_cap) at
// request time.
//
// SourceClass is the bucket the worker recognises. Format is
// "<codec>/<height>" — e.g. "hevc/2160p", "h264/1080p", "av1/1440p".
// Codec defaults to "any" when the source codec couldn't be
// determined; legacy rows from Phase C MVP (plain height like "1080p")
// still parse via the SourceClassParse helper for backward compat
// during the rolling upgrade window. Heights below 1080p don't get a
// bucket — we never cap below 720p in the handler.
type CapOverride struct {
	SourceClass  string    `json:"source_class"`
	MaxHeight    int       `json:"max_height"`
	Reason       string    `json:"reason"`
	StarvedRatio float64   `json:"starved_ratio"`
	SamplesCount int       `json:"samples_count"`
	AppliedAt    time.Time `json:"applied_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// SourceClassFor maps a (codec, height) pair to the cap-bucket the
// override system reasons about. Heights below 1080p don't get a
// bucket — we never cap below 720p in the handler so there's nothing
// to override. Codec is normalised to lowercase; empty/unknown
// codecs use the "any" sentinel so legacy callers and broken probes
// still produce a stable key.
//
// Examples:
//
//	SourceClassFor("hevc", 2160) → "hevc/2160p"
//	SourceClassFor("h264", 1080) → "h264/1080p"
//	SourceClassFor("",     1440) → "any/1440p"
//	SourceClassFor("hevc", 720)  → ""           (below 1080p floor)
func SourceClassFor(sourceCodec string, sourceHeight int) string {
	heightBucket := ""
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

// SourceClassMatchesLegacy returns true when class is a pre-codec
// Phase C MVP row ("1080p", "1440p", "2160p"). Migration helper: the
// handler can still apply legacy overrides during the rolling
// upgrade window before the worker re-aggregates with codec info.
func SourceClassMatchesLegacy(class string) bool {
	switch class {
	case "1080p", "1440p", "2160p":
		return true
	}
	return false
}

func normaliseSourceCodec(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	switch c {
	case "h264", "avc", "avc1":
		return "h264"
	case "hevc", "h265", "hev1", "hvc1":
		return "hevc"
	case "av1", "av01":
		return "av1"
	case "vp9":
		return "vp9"
	case "":
		return "any"
	default:
		return "any" // unrecognised codec → don't pollute the bucket space
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
	// Delete removes a specific source-class override. Used by the
	// admin "reset cap" button. No-op when the source class has no
	// override (already empty). Returns (true, nil) when a row was
	// actually removed so the handler can distinguish 200 vs 404.
	Delete(ctx context.Context, sourceClass string) (bool, error)
}
