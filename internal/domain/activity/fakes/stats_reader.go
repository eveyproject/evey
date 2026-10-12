package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakePlaySessionStatsReader struct {
	UserStatsResult   *activity.UserStats
	ServerStatsResult *activity.ServerStatsResult
	MostPopular       []activity.MediaPlayCount
	ActiveSessionsBps int64
	Err               error

	// LastMostPopularLimit is the limit GetMostPopular was last called with.
	LastMostPopularLimit int
}

func (r *FakePlaySessionStatsReader) GetActiveSessionsBitrate(_ context.Context) (int64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	return r.ActiveSessionsBps, nil
}

func NewFakePlaySessionStatsReader() *FakePlaySessionStatsReader {
	return &FakePlaySessionStatsReader{
		UserStatsResult:   &activity.UserStats{},
		ServerStatsResult: &activity.ServerStatsResult{},
	}
}

func (r *FakePlaySessionStatsReader) GetUserStats(_ context.Context, _ shared.ProfileID) (*activity.UserStats, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return r.UserStatsResult, nil
}

func (r *FakePlaySessionStatsReader) GetServerStats(_ context.Context) (*activity.ServerStatsResult, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return r.ServerStatsResult, nil
}

func (r *FakePlaySessionStatsReader) GetMostPopular(_ context.Context, _ string, limit int) ([]activity.MediaPlayCount, error) {
	// Recorded so a caller's limit policy (e.g. "0 means the default 20") is
	// assertable. It was not, so the test named after that default could only
	// check the call did not error.
	r.LastMostPopularLimit = limit
	if r.Err != nil {
		return nil, r.Err
	}
	result := r.MostPopular
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}
