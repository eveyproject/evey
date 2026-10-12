package user

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// setBitrateLimit is the fetch-mutate-save sequence shared by
// UserService.SetUserBitrateLimit and BitrateLimitSetter below, so the two
// call sites can't drift into two different write behaviours over time.
func setBitrateLimit(ctx context.Context, repo user.UserRepository, userID string, maxBitrateKbps int) error {
	u, err := repo.GetByID(ctx, shared.UserID(userID))
	if err != nil {
		return err
	}
	u.MaxBitrateKbps = maxBitrateKbps
	u.UpdatedAt = time.Now()
	return repo.Update(ctx, u)
}

// BitrateLimitSetter sets a user's per-account max streaming bitrate (kbps)
// against a bare UserRepository. It exists so the HTTP admin bitrate-limit
// route can reuse the UserRepository that's already wired into the http
// package (used by the parental routes) without needing a full UserService -
// which also depends on a refresh-token repo, event bus and unit-of-work that
// this single-field write never touches.
type BitrateLimitSetter struct {
	Repo user.UserRepository
}

func NewBitrateLimitSetter(repo user.UserRepository) *BitrateLimitSetter {
	return &BitrateLimitSetter{Repo: repo}
}

func (s *BitrateLimitSetter) SetUserBitrateLimit(ctx context.Context, userID string, maxBitrateKbps int) error {
	return setBitrateLimit(ctx, s.Repo, userID, maxBitrateKbps)
}
