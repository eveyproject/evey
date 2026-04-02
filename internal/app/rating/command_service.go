package rating

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/rating"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	repo rating.Repository
	bus  shared.EventBus
}

func NewCommandService(repo rating.Repository, bus shared.EventBus) *CommandService {
	return &CommandService{repo: repo, bus: bus}
}

func (s *CommandService) Rate(ctx context.Context, mediaItemID, profileID string, value int) error {
	score, err := rating.NewRatingValue(value)
	if err != nil {
		return fmt.Errorf("CommandService.Rate: %w", err)
	}
	now := time.Now()
	if err := s.repo.Upsert(ctx, rating.Rating{
		MediaItemID: mediaItemID,
		ProfileID:   profileID,
		Value:       score,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return fmt.Errorf("CommandService.Rate: %w", err)
	}
	if pubErr := s.bus.Publish(ctx, rating.RatingSet{MediaID: mediaItemID, UserID: profileID, Score: score.Int(), OccurredAt: now}); pubErr != nil {
		slog.Warn("rating: event publish failed", slog.String("mediaID", mediaItemID), slog.Any("err", pubErr))
	}
	return nil
}

func (s *CommandService) DeleteRating(ctx context.Context, mediaItemID, profileID string) error {
	if err := s.repo.Delete(ctx, mediaItemID, profileID); err != nil {
		return fmt.Errorf("CommandService.DeleteRating: %w", err)
	}
	if pubErr := s.bus.Publish(ctx, rating.RatingRemoved{MediaID: mediaItemID, UserID: profileID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("rating: RatingRemoved publish failed", slog.String("mediaID", mediaItemID), slog.Any("err", pubErr))
	}
	return nil
}
