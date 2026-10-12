package httpapi

import (
	"net/http"

	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// streamingRateLimitKey picks the rate-limit bucket for a streaming request,
// in order of preference: the playback PROFILE (matches the back-office
// per-profile screen/bitrate model - a household's multiple profiles each get
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
// stalls playback. The limiter is deliberately generous - the goal is to cut a
// cheap-request FLOOD (404s, cache hits) hammering the box, never to throttle
// legitimate viewing (even a profile streaming on several devices with heavy
// seeking stays well under the ceiling). nil limiter = disabled (pass-through),
// so tests and a no-RL deployment behave exactly as before.
//
// loadTestActive suspends it. The built-in load test authenticates every one of
// its simulated viewers with the SAME credential, so they all land in ONE
// profile's bucket and throttle each other - the run would measure this limiter
// rather than the server, reporting stalls and 503s that a real audience (one
// bucket per profile) would never see. Suspending it costs no protection: the
// load test is admin-only and refuses to start while anything real is playing.
func streamingRateLimit(limiter *ratelimit.Limiter, loadTestActive func() bool, next http.HandlerFunc) http.HandlerFunc {
	if limiter == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if loadTestActive != nil && loadTestActive() {
			next(w, r)
			return
		}
		if key := streamingRateLimitKey(r); !limiter.Allow(key) {
			ratelimit.RefuseStreaming(w, r, 2, key)
			return
		}
		next(w, r)
	}
}
