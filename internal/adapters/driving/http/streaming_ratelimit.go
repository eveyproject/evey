package httpapi

import (
	"net/http"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// streamingRateLimitKey picks the rate-limit bucket for a streaming request,
// in order of preference: the playback PROFILE (matches the back-office
// per-profile screen/bitrate model — a household's multiple profiles each get
// their own budget behind one shared IP), then the user (share-less tokens
// that predate the ProfileID field), then the client IP (share links carry no
// identity). The "p:"/"u:"/"ip:" prefixes keep the three key spaces disjoint
// so a profile id can never collide with a user id or an IP.
func streamingRateLimitKey(r *http.Request) string {
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
		if claims.ProfileID != "" {
			return "p:" + claims.ProfileID
		}
		if claims.UserID != "" {
			return "u:" + claims.UserID
		}
	}
	return "ip:" + ratelimit.ClientIP(r)
}

// streamingRateLimit wraps a streaming handler with a per-profile request-rate
// limiter. On exhaustion it returns 503 + Retry-After (NOT 429): hls.js retries
// a 503 transparently, whereas a 429 surfaces as a fatal network error and
// stalls playback. The limiter is deliberately generous — the goal is to cut a
// cheap-request FLOOD (404s, cache hits) hammering the box, never to throttle
// legitimate viewing (even a profile streaming on several devices with heavy
// seeking stays well under the ceiling). nil limiter = disabled (pass-through),
// so tests and a no-RL deployment behave exactly as before.
func streamingRateLimit(limiter *ratelimit.Limiter, next http.HandlerFunc) http.HandlerFunc {
	if limiter == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(streamingRateLimitKey(r)) {
			w.Header().Set("Retry-After", "2")
			http.Error(w, `{"message":"streaming rate limit exceeded, retry"}`, http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}
