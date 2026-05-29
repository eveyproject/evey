package qoe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReader satisfies the Reader port with canned values per method. Errors
// are configurable per call to prove the service surfaces partial-fan-out
// failures correctly.
type fakeReader struct {
	summary       *Summary
	top           []StarvedTitle
	dev           []DeviceBreakdown
	errSummary    error
	errTop        error
	errDev        error
}

func (f *fakeReader) GetSummary(_ context.Context, _ time.Duration) (*Summary, error) {
	return f.summary, f.errSummary
}
func (f *fakeReader) GetTopStarvedTitles(_ context.Context, _ time.Duration, _ int) ([]StarvedTitle, error) {
	return f.top, f.errTop
}
func (f *fakeReader) GetByDeviceClass(_ context.Context, _ time.Duration) ([]DeviceBreakdown, error) {
	return f.dev, f.errDev
}

func TestQueryService_FansOutSubQueries(t *testing.T) {
	reader := &fakeReader{
		summary: &Summary{WindowLabel: "24h", TotalEvents: 100, StarvedEvents: 7, StarvedRatio: 0.07},
		top: []StarvedTitle{
			{MediaItemID: "movie-1", StarvedEvents: 5, TotalEvents: 60, StarvedRatio: 0.0833},
		},
		dev: []DeviceBreakdown{
			{DeviceClass: "ios", TotalEvents: 80, StarvedRatio: 0.05},
		},
	}
	svc := NewQueryService(reader)
	dash, err := svc.GetDashboard(context.Background(), 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(100), dash.Summary.TotalEvents)
	assert.Len(t, dash.TopStarved, 1)
	assert.Len(t, dash.ByDevice, 1)
}

func TestQueryService_SurfaceErrorFromAnySubQuery(t *testing.T) {
	cases := []struct {
		name string
		r    *fakeReader
	}{
		{"summary err", &fakeReader{errSummary: errors.New("db: timeout")}},
		{"top err", &fakeReader{summary: &Summary{}, errTop: errors.New("db: bad sql")}},
		{"device err", &fakeReader{summary: &Summary{}, errDev: errors.New("db: deadlock")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewQueryService(c.r)
			_, err := svc.GetDashboard(context.Background(), 24*time.Hour)
			assert.Error(t, err, "partial failure must propagate, not silently return empty")
		})
	}
}

func TestQueryService_NilReaderFailsCleanly(t *testing.T) {
	svc := NewQueryService(nil)
	_, err := svc.GetDashboard(context.Background(), 24*time.Hour)
	assert.Error(t, err)
}
