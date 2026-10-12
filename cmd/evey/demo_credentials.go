package main

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// demoCredentials hands the demo bootstrap the two stores of credentials a
// demo account can hold, so boot and reset purge them (appdemo.Credentials).
type demoCredentials struct {
	keys    user.APIKeyRepository
	invites user.InviteLinkRepository
}

func (c demoCredentials) ListAPIKeys(ctx context.Context, userID shared.UserID) ([]*user.APIKeyModel, error) {
	return c.keys.ListByUserID(ctx, userID)
}

func (c demoCredentials) DeleteAPIKey(ctx context.Context, id string) error {
	return c.keys.Delete(ctx, id)
}

func (c demoCredentials) ListInvites(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	return c.invites.ListByCreator(ctx, creatorID)
}

func (c demoCredentials) DeleteInvite(ctx context.Context, id string) error {
	return c.invites.Delete(ctx, id)
}
