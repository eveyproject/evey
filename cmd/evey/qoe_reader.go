package main

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	appqoe "github.com/eveyproject/evey/internal/app/qoe"
)

// qoeReaderAdapter bridges the postgres reader (returns postgres-typed rows)
// to the app/qoe.Reader port (returns app-typed rows). Keeps the postgres
// package free of any app/qoe import, and the app package free of postgres.
type qoeReaderAdapter struct {
	reader *postgres.PlaybackQoEReader
}

func (a qoeReaderAdapter) GetSummary(ctx context.Context, window time.Duration) (*appqoe.Summary, error) {
	row, err := a.reader.GetSummary(ctx, window)
	if err != nil {
		return nil, err
	}
	return &appqoe.Summary{
		WindowLabel:    row.WindowLabel,
		TotalEvents:    row.TotalEvents,
		StarvedEvents:  row.StarvedEvents,
		StarvedRatio:   row.StarvedRatio,
		UniqueSessions: row.UniqueSessions,
		UniqueMedia:    row.UniqueMedia,
	}, nil
}

func (a qoeReaderAdapter) GetTopStarvedTitles(ctx context.Context, window time.Duration, limit int) ([]appqoe.StarvedTitle, error) {
	rows, err := a.reader.GetTopStarvedTitles(ctx, window, limit)
	if err != nil {
		return nil, err
	}
	out := make([]appqoe.StarvedTitle, 0, len(rows))
	for _, r := range rows {
		out = append(out, appqoe.StarvedTitle{
			MediaItemID:   r.MediaItemID,
			Title:         r.Title,
			FilePath:      r.FilePath,
			StarvedEvents: r.StarvedEvents,
			TotalEvents:   r.TotalEvents,
			StarvedRatio:  r.StarvedRatio,
			LastStarvedAt: r.LastStarvedAt,
		})
	}
	return out, nil
}

func (a qoeReaderAdapter) GetByDeviceClass(ctx context.Context, window time.Duration) ([]appqoe.DeviceBreakdown, error) {
	rows, err := a.reader.GetByDeviceClass(ctx, window)
	if err != nil {
		return nil, err
	}
	out := make([]appqoe.DeviceBreakdown, 0, len(rows))
	for _, r := range rows {
		out = append(out, appqoe.DeviceBreakdown{
			DeviceClass:    r.DeviceClass,
			TotalEvents:    r.TotalEvents,
			StarvedEvents:  r.StarvedEvents,
			StarvedRatio:   r.StarvedRatio,
			AvgBufferMs:    r.AvgBufferMs,
			AvgEncodedKbps: r.AvgEncodedKbps,
		})
	}
	return out, nil
}
