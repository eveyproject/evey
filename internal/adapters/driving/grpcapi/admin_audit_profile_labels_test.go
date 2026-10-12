package grpcapi

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// The music "now playing" registry only knows a profile id; the panel needs
// the profile's name and its account. One read for all sessions, and any
// failure degrades to "unlabeled", never to a failed panel.
func TestProfileLabels(t *testing.T) {
	ctx := context.Background()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	require.NoError(t, userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice"}))
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Salon"}))
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "p2", UserID: "ghost", Name: "Orphan"}))
	query := appuser.NewQueryService(userRepo, profileRepo, userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository())

	assert.Nil(t, (&adminSessionHandler{}).profileLabels(ctx), "no query port = no labels, no panic")

	d := &adminSessionHandler{userQuery: query}
	labels := d.profileLabels(ctx)
	assert.Equal(t, profileLabel{name: "Salon", userID: "u1", username: "alice"}, labels["p1"])
	assert.Equal(t, profileLabel{name: "Orphan", userID: "ghost"}, labels["p2"], "an account that vanished leaves the username blank, not the profile")
	assert.Equal(t, profileLabel{}, labels["unknown"], "a missing id reads as the zero label the caller falls back from")

	// Users unreadable: profiles still labeled by name, accounts blank.
	userRepo.Err = errors.New("db down")
	labels = d.profileLabels(ctx)
	assert.Equal(t, profileLabel{name: "Salon", userID: "u1"}, labels["p1"])

	// Profiles unreadable: nothing labeled, but a map lookup stays safe.
	profileRepo.Err = errors.New("db down")
	labels = d.profileLabels(ctx)
	assert.Nil(t, labels)
	assert.Empty(t, labels["p1"].name)
}
