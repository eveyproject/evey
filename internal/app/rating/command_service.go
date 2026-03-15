package rating

import (
	"context"
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
		return err
	}
	now := time.Now()
	if err := s.repo.Upsert(ctx, rating.Rating{
		MediaItemID: mediaItemID,
		ProfileID:   profileID,
		Value:       score,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}
	if pubErr := s.bus.Publish(ctx, rating.MediaRated{UserID: profileID, MediaID: mediaItemID, Rating: score.Int(), OccurredAt: now}); pubErr != nil {
		slog.Warn("rating: event publish failed", slog.String("mediaID", mediaItemID), slog.Any("err", pubErr))
	}
	return nil
}

func (s *CommandService) DeleteRating(ctx context.Context, mediaItemID, profileID string) error {
	if err := s.repo.Delete(ctx, mediaItemID, profileID); err != nil {
		return err
	}
	if pubErr := s.bus.Publish(ctx, rating.MediaRatingDeleted{UserID: profileID, MediaID: mediaItemID, OccurredAt: time.Now()}); pubErr != nil {
		slog.Warn("rating: MediaRatingDeleted publish failed", slog.String("mediaID", mediaItemID), slog.Any("err", pubErr))
	}
	return nil
}
