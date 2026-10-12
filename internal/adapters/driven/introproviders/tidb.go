package introproviders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// tidbUserAgent identifies Evey to the upstream so anonymous traffic is
// attributable and not lumped in with generic bots - the convention the
// service's existing clients follow.
const tidbUserAgent = "Evey/intro-detection (+https://github.com/eveyproject/evey)"

// TIDBClient is an HTTP client for theintrodb.org's `/v3/media` endpoint.
//
// v3 keeps the v2 response shape (tmdb_id/type/season/episode + intro/recap/
// credits/preview arrays of {start_ms,end_ms}) and adds: a first-class
// imdb_id query fallback, an optional duration_ms hint, and optional per-
// segment confidence/submission_count. It is queried by tmdb_id when known,
// else by imdb_id.
//
// The API exposes three independent rate-limit tiers via response headers:
//   - X-RateLimit-* - 30 req / 10s burst. The headerRateLimiter observes
//     Remaining/Reset and proactively pauses before the next request when
//     Remaining < 3, avoiding 429s on bulk scans.
//   - X-UsageLimit-* - 100/day anonymous, 500/day with Bearer auth. Not
//     paced proactively (the reset window can be hours); we let a daily-cap
//     429 terminate the run and resume next cycle, shielded by the cache.
//   - X-UsageLimit-SpecificMedia-* - per-media cap (~1000). Same treatment.
//
// On 429 we honour the longest of Retry-After / X-UsageLimit-Reset /
// X-RateLimit-Reset (parseRetryAfter) so we never retry inside a still-open
// window. No automatic retry - the caller resumes next cycle.
type TIDBClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	rl         *headerRateLimiter
}

// TIDBOption configures the client at construction time.
type TIDBOption func(*TIDBClient)

// WithTIDBBaseURL overrides the API base, e.g. a httptest server.
func WithTIDBBaseURL(u string) TIDBOption {
	return func(t *TIDBClient) { t.baseURL = strings.TrimRight(u, "/") }
}

// NewTIDBClient returns a client targeted at theintrodb.org. apiKey may be
// empty (anonymous mode, lower daily quota).
func NewTIDBClient(apiKey string, opts ...TIDBOption) *TIDBClient {
	c := &TIDBClient{
		baseURL:    "https://api.theintrodb.org/v3",
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		rl:         newHeaderRateLimiter(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Source is the MediaSegment.Source value the cache and admin UI use to
// audit which provider produced each segment.
func (c *TIDBClient) Source() string { return "tidb" }

// tidbSegment matches the API's segment shape - start/end are nullable.
// For intro/recap a null start means "starts at 0"; for credits/preview a
// null end means "runs to end of media". Confidence/SubmissionCount are v3
// additions, optional and frequently absent (the official client ignores
// them); we use Confidence when present and fall back to a fixed floor.
type tidbSegment struct {
	StartMs         *int64   `json:"start_ms"`
	EndMs           *int64   `json:"end_ms"`
	Confidence      *float64 `json:"confidence,omitempty"`
	SubmissionCount *int     `json:"submission_count,omitempty"`
}

type tidbMediaResponse struct {
	TmdbID  int           `json:"tmdb_id"`
	Type    string        `json:"type"` // "movie" | "tv"
	Season  int           `json:"season,omitempty"`
	Episode int           `json:"episode,omitempty"`
	Intro   []tidbSegment `json:"intro,omitempty"`
	Recap   []tidbSegment `json:"recap,omitempty"`
	Credits []tidbSegment `json:"credits,omitempty"`
	Preview []tidbSegment `json:"preview,omitempty"`
}

// Fetch performs a single GET /v2/media call and maps the response to
// domain MediaSegments. 404 = no data for this item (returns nil, nil
// - chain continues). 429 returns a typed error (the caller backs off
// for the rest of the run).
func (c *TIDBClient) Fetch(ctx context.Context, lookup Lookup) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "introproviders.tidb.Fetch")
	defer span.End()

	q := url.Values{}
	switch {
	case lookup.TmdbID != 0:
		q.Set("tmdb_id", strconv.Itoa(lookup.TmdbID))
	case lookup.ImdbID != "":
		// v3 accepts imdb_id directly (movies and series), so TIDB now covers
		// the imdb-only items that previously fell straight through to IntroDB.
		q.Set("imdb_id", lookup.ImdbID)
	default:
		return nil, nil // no canonical identifier - nothing to query
	}
	if !lookup.IsMovie {
		if lookup.Season == 0 || lookup.Episode == 0 {
			return nil, nil // TV item without a resolvable season/episode
		}
		q.Set("season", strconv.Itoa(lookup.Season))
		q.Set("episode", strconv.Itoa(lookup.Episode))
	}
	if lookup.DurationMs > 0 {
		// Helps the server pick the matching release cut and lets it resolve
		// open-ended credits server-side. Safe to omit when unknown.
		q.Set("duration_ms", strconv.FormatInt(lookup.DurationMs, 10))
	}
	reqURL := c.baseURL + "/media?" + q.Encode()

	if err := c.rl.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tidb: rate-limit wait cancelled: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tidb: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", tidbUserAgent)
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidb: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Always update the rate-limiter so the next call benefits from the
	// window information, even for non-200 responses.
	c.rl.Update(resp.Header)

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decode
	case http.StatusNotFound:
		return nil, nil // no data is the common case, not an error
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited{
			Source:     c.Source(),
			RetryAfter: parseRetryAfter(resp.Header),
		}
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tidb: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload tidbMediaResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("tidb: decode body: %w", err)
	}

	segs := make([]domainlibrary.MediaSegment, 0, 4)
	// intro/recap require an explicit end (null end is meaningless). credits/
	// preview may carry a null end meaning "to end of media" - resolved
	// against lookup.DurationMs when known.
	segs = appendTIDBSegments(segs, payload.Intro, "intro", c.Source(), true, lookup.DurationMs)
	segs = appendTIDBSegments(segs, payload.Recap, "recap", c.Source(), true, lookup.DurationMs)
	segs = appendTIDBSegments(segs, payload.Credits, "end_credits", c.Source(), false, lookup.DurationMs)
	// preview = next-episode preview reel; surface as "preview" so the
	// player can offer Skip Preview (admin can hide the type if unwanted).
	segs = appendTIDBSegments(segs, payload.Preview, "preview", c.Source(), false, lookup.DurationMs)

	if len(segs) == 0 {
		// 200 with empty arrays → service knew the item but has no
		// validated submissions. Same fall-through semantics as 404.
		return nil, nil
	}
	return segs, nil
}

// appendTIDBSegments maps raw upstream segments to domain segments.
//
// endRequired distinguishes the two segment families: intro/recap (true)
// must carry an explicit end_ms; credits/preview (false) may carry end_ms
// == null meaning "to the end of media", which we close against durationMs
// (the item's runtime). When the end is open and durationMs is unknown the
// segment is dropped - a half-open range can't be rendered as a skip CTA.
func appendTIDBSegments(out []domainlibrary.MediaSegment, raw []tidbSegment, segType, source string, endRequired bool, durationMs int64) []domainlibrary.MediaSegment {
	for _, s := range raw {
		// A nil start means "starts at 0" - clamp so the player renders the
		// segment from the very beginning.
		var startMs int64
		if s.StartMs != nil {
			startMs = *s.StartMs
		}

		var endMs int64
		switch {
		case s.EndMs != nil:
			endMs = *s.EndMs
		case !endRequired && durationMs > 0:
			endMs = durationMs // open-ended credits/preview → run to media end
		default:
			slog.Debug("tidb: dropping segment with unresolvable end_ms",
				slog.String("type", segType), slog.Bool("end_required", endRequired))
			continue
		}

		if startMs < 0 || endMs <= startMs {
			continue
		}
		out = append(out, domainlibrary.MediaSegment{
			Type:       segType,
			StartMs:    startMs,
			EndMs:      endMs,
			Source:     source,
			Confidence: tidbConfidence(s),
		})
	}
	return out
}

// tidbConfidence uses the upstream per-segment confidence when present
// (clamped to 0..1), else a 0.97 floor - community-validated data should
// stay competitive with our pHash+Chromaprint primary.
func tidbConfidence(s tidbSegment) float64 {
	if s.Confidence != nil && *s.Confidence > 0 {
		if *s.Confidence > 1 {
			return 1
		}
		return *s.Confidence
	}
	return 0.97
}

// ErrRateLimited is returned by upstream clients when they hit a 429.
// Chain provider treats it as a transient terminal error for the run -
// the cache hides the user from repeated failures across calls.
type ErrRateLimited struct {
	Source     string
	RetryAfter time.Duration
}

func (e ErrRateLimited) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("introproviders: %s rate-limited, retry after %s", e.Source, e.RetryAfter)
	}
	return fmt.Sprintf("introproviders: %s rate-limited", e.Source)
}

// IsRateLimited unwraps ErrRateLimited from any error chain.
func IsRateLimited(err error) bool {
	var rl ErrRateLimited
	return errors.As(err, &rl)
}

// parseRetryAfter returns the longest reset window advertised by the
// response. A 429 may be a burst hit (short X-RateLimit-Reset) or a daily/
// per-media cap hit (long X-UsageLimit-Reset); taking the max means we never
// retry inside a window that is still closed. IntroDB sends none of these,
// so it falls back to 0.
func parseRetryAfter(h http.Header) time.Duration {
	var maxSecs int
	for _, name := range []string{"Retry-After", "X-UsageLimit-Reset", "X-RateLimit-Reset"} {
		if v := h.Get(name); v != "" {
			if secs, err := strconv.Atoi(v); err == nil && secs > maxSecs {
				maxSecs = secs
			}
		}
	}
	return time.Duration(maxSecs) * time.Second
}
