package qoe

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Reader is the narrow port the query service needs from the storage layer.
// Mirrors the postgres PlaybackQoEReader 1:1 so the adapter can satisfy it
// without any glue in between.
type Reader interface {
	GetSummary(ctx context.Context, window time.Duration) (*Summary, error)
	GetTopStarvedTitles(ctx context.Context, window time.Duration, limit int) ([]StarvedTitle, error)
	GetByDeviceClass(ctx context.Context, window time.Duration) ([]DeviceBreakdown, error)
}

// Summary is the admin top-bar view: how much telemetry came in, how much
// of it represented a poor experience, and over how many sessions / titles.
type Summary struct {
	WindowLabel    string  `json:"window"`
	TotalEvents    int64   `json:"total_events"`
	StarvedEvents  int64   `json:"starved_events"`
	StarvedRatio   float64 `json:"starved_ratio"`
	UniqueSessions int64   `json:"unique_sessions"`
	UniqueMedia    int64   `json:"unique_media"`
}

// StarvedTitle is the "viewers struggled here" leaderboard.
type StarvedTitle struct {
	MediaItemID   string    `json:"media_item_id"`
	StarvedEvents int64     `json:"starved_events"`
	TotalEvents   int64     `json:"total_events"`
	StarvedRatio  float64   `json:"starved_ratio"`
	LastStarvedAt time.Time `json:"last_starved_at"`
}

// DeviceBreakdown is the "where is it bad" slice. Pure player-side device
// class — no IP geo, no fingerprinting.
type DeviceBreakdown struct {
	DeviceClass    string  `json:"device_class"`
	TotalEvents    int64   `json:"total_events"`
	StarvedEvents  int64   `json:"starved_events"`
	StarvedRatio   float64 `json:"starved_ratio"`
	AvgBufferMs    float64 `json:"avg_buffer_ms"`
	AvgEncodedKbps float64 `json:"avg_encoded_kbps"`
}

// Dashboard is the bundle the admin endpoint returns in one shot.
type Dashboard struct {
	Summary       *Summary          `json:"summary"`
	TopStarved    []StarvedTitle    `json:"top_starved_titles"`
	ByDevice      []DeviceBreakdown `json:"by_device"`
}

// QueryService composes the reader calls into a single dashboard view. All
// three queries fan out in parallel so the admin page stays under one
// round-trip latency even on a cold cache.
type QueryService struct {
	reader Reader
}

func NewQueryService(reader Reader) *QueryService {
	return &QueryService{reader: reader}
}

// GetDashboard returns the full admin view for the given window. Default
// limit on top-starved is 10. A failed sub-query bubbles up; we'd rather
// surface a real error in the admin UI than show partial data that lies.
func (s *QueryService) GetDashboard(ctx context.Context, window time.Duration) (*Dashboard, error) {
	if s.reader == nil {
		return nil, fmt.Errorf("qoe: no reader wired (Phase 1 storage not yet configured)")
	}

	var (
		summary *Summary
		top     []StarvedTitle
		dev     []DeviceBreakdown
		errs    [3]error
		wg      sync.WaitGroup
	)
	wg.Add(3)

	go func() {
		defer wg.Done()
		summary, errs[0] = s.reader.GetSummary(ctx, window)
	}()
	go func() {
		defer wg.Done()
		top, errs[1] = s.reader.GetTopStarvedTitles(ctx, window, 10)
	}()
	go func() {
		defer wg.Done()
		dev, errs[2] = s.reader.GetByDeviceClass(ctx, window)
	}()

	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return &Dashboard{
		Summary:    summary,
		TopStarved: top,
		ByDevice:   dev,
	}, nil
}
