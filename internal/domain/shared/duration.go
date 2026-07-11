package shared

import (
	"errors"
	"math"
	"time"
)

// ErrNegativeDuration is returned when a Duration constructor receives a
// negative value. Durations model elapsed time and are non-negative by
// construction; a negative input is a programmer error at the boundary
// (HTTP/SQL/probe), not a domain state.
var ErrNegativeDuration = errors.New("duration must be non-negative")

// ErrDurationOverflow is returned when an arithmetic operation on Duration
// would overflow int64 milliseconds. Adding two near-max durations is the
// realistic trigger; ~292M years is the int64-ms ceiling so this is a
// defensive check rather than a likely runtime path.
var ErrDurationOverflow = errors.New("duration arithmetic overflow")

// Duration is a non-negative duration stored as integer milliseconds.
//
// Replaces ad-hoc `DurationMs int64` fields that proliferated across the
// transcoding, playback and library packages with their own conversion
// helpers. The struct (rather than a typedef on int64) hides the unit so
// callers cannot accidentally mix milliseconds and seconds at the type
// system level - they must go through Milliseconds() / Seconds() / Time().
//
// The zero value is a valid 0-duration; IsZero() reports it.
type Duration struct {
	ms int64
}

// NewDurationMs builds a Duration from a millisecond count. Negative input
// returns ErrNegativeDuration. The full int64 positive range is accepted
// (~292 million years), so callers don't need to pre-clamp.
func NewDurationMs(ms int64) (Duration, error) {
	if ms < 0 {
		return Duration{}, ErrNegativeDuration
	}
	return Duration{ms: ms}, nil
}

// NewDurationFromTime converts a time.Duration to a Duration. Sub-millisecond
// nanoseconds are rounded to the nearest millisecond (banker-free, math.Round
// semantics: .5 rounds away from zero) so that round-tripping a probed
// duration doesn't silently truncate. Negative input returns
// ErrNegativeDuration.
func NewDurationFromTime(d time.Duration) (Duration, error) {
	if d < 0 {
		return Duration{}, ErrNegativeDuration
	}
	// Round to nearest ms. d is non-negative so float64 conversion is safe
	// for any realistic duration (time.Duration max ~292 years << 2^53 ns).
	ms := int64(math.Round(float64(d) / float64(time.Millisecond)))
	return Duration{ms: ms}, nil
}

// Milliseconds returns the duration as integer milliseconds. Always >= 0.
func (d Duration) Milliseconds() int64 { return d.ms }

// Seconds returns the duration as fractional seconds. Lossy for sub-ms
// precision (none is stored), exact for ms-aligned values up to 2^53 ms.
func (d Duration) Seconds() float64 { return float64(d.ms) / 1000.0 }

// Time returns the duration as a time.Duration (nanosecond resolution).
// Safe for any value built via the constructors: ms * 1_000_000 fits in
// int64 for any positive ms up to ~292 years (well above any media use case).
func (d Duration) Time() time.Duration { return time.Duration(d.ms) * time.Millisecond }

// IsZero reports whether the duration is exactly 0 ms.
func (d Duration) IsZero() bool { return d.ms == 0 }

// Equal reports whether two durations are exactly equal in milliseconds.
func (d Duration) Equal(other Duration) bool { return d.ms == other.ms }

// Less reports whether d is strictly shorter than other.
func (d Duration) Less(other Duration) bool { return d.ms < other.ms }

// Add returns d + other. Returns ErrDurationOverflow if the sum would
// exceed int64 milliseconds. Both operands are non-negative by construction
// so only positive overflow is possible.
func (d Duration) Add(other Duration) (Duration, error) {
	if other.ms > math.MaxInt64-d.ms {
		return Duration{}, ErrDurationOverflow
	}
	return Duration{ms: d.ms + other.ms}, nil
}

// String returns a human-readable form via time.Duration.String() (e.g.
// "1h23m45s"). Sub-second components only appear when the stored value has
// non-zero ms below the second boundary.
func (d Duration) String() string { return d.Time().String() }
