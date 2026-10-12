package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/activity"
)

type FakeAnalyticsReader struct {
	Data *activity.AnalyticsData
	Err  error
}

func NewFakeAnalyticsReader() *FakeAnalyticsReader {
	return &FakeAnalyticsReader{
		Data: &activity.AnalyticsData{
			TopMedia:       []activity.TopMediaItem{},
			RecentActivity: []activity.RecentActivityEntry{},
		},
	}
}

func (r *FakeAnalyticsReader) GetAnalytics(_ context.Context) (*activity.AnalyticsData, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return r.Data, nil
}
