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

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// TIDBClient is an HTTP client for theintrodb.org's `/v2/media` endpoint.
//
// The API documents per-source rate limits we honour conservatively:
//   - 30 req / 10s burst on /media → enforce per-second pacing client-side
//   - 100/day anonymous, 500/day with Bearer auth → emit a metric on 429
//
// On 429 we honour Retry-After when present; otherwise back off with the
// X-RateLimit-Reset header. No automatic retry — caller can retry the
// next call cycle. Shielding the daily quota matters more than the burst.
type TIDBClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// TIDBOption configures the client at construction time.
type TIDBOption func(*TIDBClient)

// WithTIDBHTTPClient swaps the default *http.Client. Tests use this to
// inject a httptest.Server-backed transport.
func WithTIDBHTTPClient(c *http.Client) TIDBOption {
	return func(t *TIDBClient) { t.httpClient = c }
}

// WithTIDBBaseURL overrides the API base, e.g. a httptest server.
func WithTIDBBaseURL(u string) TIDBOption {
	return func(t *TIDBClient) { t.baseURL = strings.TrimRight(u, "/") }
}

// NewTIDBClient returns a client targeted at theintrodb.org. apiKey may be
// empty (anonymous mode, lower daily quota).
func NewTIDBClient(apiKey string, opts ...TIDBOption) *TIDBClient {
	c := &TIDBClient{
		baseURL:    "https://api.theintrodb.org/v2",
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Source is the MediaSegment.Source value the cache and admin UI use to
// audit which provider produced each segment.
func (c *TIDBClient) Source() string { return "tidb" }

// tidbSegment matches the API's segment shape — start/end are nullable
// per the OpenAPI: intro start may be null when "< 5s threshold", etc.
// The doc says start_ms/end_ms are integers in ms.
type tidbSegment struct {
	StartMs *int64 `json:"start_ms"`
	EndMs   *int64 `json:"end_ms"`
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
// — chain continues). 429 returns a typed error (the caller backs off
// for the rest of the run).
func (c *TIDBClient) Fetch(ctx context.Context, lookup Lookup) ([]domainlibrary.MediaSegment, error) {
	if lookup.TmdbID == 0 {
		// TIDB requires tmdb_id for canonical lookups; the imdb_id fallback
		// adds latency and unreliable mapping per the OpenAPI doc — skip
		// and let IntroDB handle imdb-only items.
		return nil, nil
	}
	q := url.Values{}
	q.Set("tmdb_id", strconv.Itoa(lookup.TmdbID))
	if !lookup.IsMovie {
		q.Set("season", strconv.Itoa(lookup.Season))
		q.Set("episode", strconv.Itoa(lookup.Episode))
	}
	reqURL := c.baseURL + "/media?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tidb: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidb: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
	segs = appendTIDBSegments(segs, payload.Intro, "intro", c.Source())
	segs = appendTIDBSegments(segs, payload.Recap, "recap", c.Source())
	segs = appendTIDBSegments(segs, payload.Credits, "end_credits", c.Source())
	// preview = next-episode preview reel; surface as "preview" so the
	// player can offer Skip Preview (admin can hide the type if unwanted).
	segs = appendTIDBSegments(segs, payload.Preview, "preview", c.Source())

	if len(segs) == 0 {
		// 200 with empty arrays → service knew the item but has no
		// validated submissions. Same fall-through semantics as 404.
		return nil, nil
	}
	return segs, nil
}

func appendTIDBSegments(out []domainlibrary.MediaSegment, raw []tidbSegment, segType, source string) []domainlibrary.MediaSegment {
	for _, s := range raw {
		// Per the OpenAPI: a nil start means "starts at 0" (< 5s threshold
		// collapse) — clamp to 0 so the player can render the segment from
		// the very beginning.
		var startMs int64
		if s.StartMs != nil {
			startMs = *s.StartMs
		}
		// nil end_ms is intentionally rejected: a half-open segment can't be
		// rendered as a skip CTA. Movies' credits often carry end=null
		// ("until end of file"); the player would need media.duration to
		// resolve it, which the cache layer doesn't carry. Defer to local
		// detection in that case.
		if s.EndMs == nil {
			slog.Debug("tidb: dropping segment with null end_ms", slog.String("type", segType))
			continue
		}
		if startMs < 0 || *s.EndMs <= startMs {
			continue
		}
		out = append(out, domainlibrary.MediaSegment{
			Type:       segType,
			StartMs:    startMs,
			EndMs:      *s.EndMs,
			Source:     source,
			Confidence: 0.97, // upstream is community-validated; matches our pHash+Chromaprint primary
		})
	}
	return out
}

// ErrRateLimited is returned by upstream clients when they hit a 429.
// Chain provider treats it as a transient terminal error for the run —
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

func parseRetryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}
