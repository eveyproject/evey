package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakePlaySessionStatsReader struct {
	UserStatsResult   *activity.UserStats
	ServerStatsResult *activity.ServerStatsResult
	MostPopular       []activity.MediaPlayCount
	Err               error
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

func (r *FakePlaySessionStatsReader) GetMostPopular(_ context.Context, limit int) ([]activity.MediaPlayCount, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	result := r.MostPopular
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}
