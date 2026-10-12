package introproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// IntroDBClient is an HTTP client for introdb.app. It only exposes a TV
// endpoint, lookup keyed on (imdb_id, season, episode); movies are
// silently skipped (return nil, nil).
//
// Rate limits are vague in the upstream ToS ("reasonable"); empirically
// 60 req in a row come back 200 with no X-RateLimit-* header. The
// chain layer paces calls at >100ms anyway via the cache; that's
// well under any conceivable threshold.
type IntroDBClient struct {
	baseURL    string
	httpClient *http.Client
}

// IntroDBOption is the option pattern.
type IntroDBOption func(*IntroDBClient)

// WithIntroDBBaseURL overrides the API base URL.
func WithIntroDBBaseURL(u string) IntroDBOption {
	return func(t *IntroDBClient) { t.baseURL = strings.TrimRight(u, "/") }
}

// NewIntroDBClient returns a client targeted at api.introdb.app.
func NewIntroDBClient(opts ...IntroDBOption) *IntroDBClient {
	c := &IntroDBClient{
		baseURL:    "https://api.introdb.app",
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Source is the MediaSegment.Source identifier.
func (c *IntroDBClient) Source() string { return "introdb" }

// introDBSegment is the per-type element shape used by /segments. Both
// start_ms and end_ms are required (per the OpenAPI), so both fields
// are non-pointer here.
type introDBSegment struct {
	StartMs         int64   `json:"start_ms"`
	EndMs           int64   `json:"end_ms"`
	Confidence      float64 `json:"confidence"`
	SubmissionCount int     `json:"submission_count"`
}

type introDBSegmentsResponse struct {
	ImdbID  string          `json:"imdb_id"`
	Season  int             `json:"season"`
	Episode int             `json:"episode"`
	Intro   *introDBSegment `json:"intro"`
	Recap   *introDBSegment `json:"recap"`
	Outro   *introDBSegment `json:"outro"`
}

// Fetch performs GET /segments with imdb_id+season+episode. Returns
// nil, nil for movies (no movie endpoint), missing imdb_id, or 404.
func (c *IntroDBClient) Fetch(ctx context.Context, lookup Lookup) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "introproviders.introdb.Fetch")
	defer span.End()

	if lookup.IsMovie {
		return nil, nil
	}
	if lookup.ImdbID == "" || lookup.Season == 0 || lookup.Episode == 0 {
		return nil, nil
	}

	q := url.Values{}
	q.Set("imdb_id", lookup.ImdbID)
	q.Set("season", strconv.Itoa(lookup.Season))
	q.Set("episode", strconv.Itoa(lookup.Episode))
	reqURL := c.baseURL + "/segments?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("introdb: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introdb: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, nil
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited{Source: c.Source(), RetryAfter: parseRetryAfter(resp.Header)}
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("introdb: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload introDBSegmentsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("introdb: decode body: %w", err)
	}

	segs := make([]domainlibrary.MediaSegment, 0, 3)
	if seg := convertIntroDBSegment(payload.Intro, "intro", c.Source()); seg != nil {
		segs = append(segs, *seg)
	}
	if seg := convertIntroDBSegment(payload.Recap, "recap", c.Source()); seg != nil {
		segs = append(segs, *seg)
	}
	if seg := convertIntroDBSegment(payload.Outro, "end_credits", c.Source()); seg != nil {
		segs = append(segs, *seg)
	}
	if len(segs) == 0 {
		return nil, nil
	}
	return segs, nil
}

func convertIntroDBSegment(raw *introDBSegment, segType, source string) *domainlibrary.MediaSegment {
	if raw == nil {
		return nil
	}
	if raw.StartMs < 0 || raw.EndMs <= raw.StartMs {
		return nil
	}
	// Use the upstream confidence directly (already 0..1); fall back to
	// 0.95 if absent to keep the segment above the in-house heuristic
	// floor. SubmissionCount=1 alone shouldn't push to 0.97 - TIDB has
	// stricter validation, IntroDB is lighter.
	conf := raw.Confidence
	if conf <= 0 {
		conf = 0.95
	}
	if conf > 1 {
		conf = 1
	}
	return &domainlibrary.MediaSegment{
		Type:       segType,
		StartMs:    raw.StartMs,
		EndMs:      raw.EndMs,
		Source:     source,
		Confidence: conf,
	}
}
