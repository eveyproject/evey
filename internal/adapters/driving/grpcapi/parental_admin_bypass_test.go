package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// Verifies: an admin caller must never be subject to a parental filter,
// even when (somehow) their own profile carries IsKids=true. The bug we
// fix: a "single-user-server" admin whose profile got flipped to IsKids
// would otherwise see ~95% of their library hidden behind the kids
// parental gate, because items lacking a resolvable rating are
// fail-closed for kids.
func TestResolveParentalFilter_AdminBypass(t *testing.T) {
	maxAge := 6
	profile := &user.Profile{
		ID:     "prf-admin",
		UserID: "usr-admin",
		IsKids: true,
		Parental: &user.ParentalSettings{
			Enabled: true,
			MaxAge:  &maxAge,
		},
	}

	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(context.Background(), profile))

	users := userfakes.NewFakeUserRepository()
	apiKeys := userfakes.NewFakeAPIKeyRepository()
	invites := userfakes.NewFakeInviteLinkRepository()
	qc := userfakes.NewFakeQuickConnectRepository()
	uq := appuser.NewQueryService(users, profiles, apiKeys, invites, qc)

	ctxAdmin := interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "usr-admin",
		IsAdmin:   true,
		ProfileID: string(profile.ID),
	})

	pfAdmin := resolveParentalFilterFromUserQuery(ctxAdmin, uq)
	assert.Equal(t, -1, pfAdmin.maxAge, "admin must get no-op parental filter (maxAge=-1)")
	assert.Empty(t, pfAdmin.allowedRatings, "admin filter must not constrain SQL with allowed ratings")
	assert.Empty(t, pfAdmin.blockedTags, "admin filter must not carry blocked tags")

	// Non-admin with the same kids profile MUST still be gated.
	ctxKid := interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "usr-admin",
		IsAdmin:   false,
		ProfileID: string(profile.ID),
	})
	pfKid := resolveParentalFilterFromUserQuery(ctxKid, uq)
	assert.Equal(t, maxAge, pfKid.maxAge, "non-admin kids profile keeps its parental gate")
}

// Ensures the bypass keys off claims.IsAdmin, not off the profile owner -
// the function takes a context-derived claim, not the profile.UserID.
var _ shared.ProfileID = user.Profile{}.ID

// The twin of TestResolveParentalFilter_AdminBypass, on the playback side.
//
// b3ad4ac43 ("fix(library): admins bypass parental filter") fixed the browse
// path and added the test above - but never crossed over to playback, whose
// isBlockedByParental rebuilt the same filter by hand and simply never looked
// at claims.IsAdmin. Result: an admin whose profile carries IsKids=true can
// BROWSE the whole library and then be refused playback of every title in it.
// Not a security hole - a functional lockout, and the same
// fixed-one-path-forgot-the-twin shape as the three parental bugs found on
// 2026-07-20.
func TestPlaybackParentalGate_AdminBypass(t *testing.T) {
	maxAge := 6
	profile := &user.Profile{
		ID:     "prf-admin-pb",
		UserID: "usr-admin-pb",
		IsKids: true,
		Parental: &user.ParentalSettings{
			Enabled: true,
			MaxAge:  &maxAge,
		},
	}
	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(context.Background(), profile))

	uq := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)
	h := &playbackHandler{userQuery: uq}

	ctxAdmin := interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "usr-admin-pb",
		IsAdmin:   true,
		ProfileID: string(profile.ID),
	})
	blocked, reason := h.isBlockedByParental(ctxAdmin, string(profile.ID), "mi-anything")
	assert.False(t, blocked, "an admin must never be parental-blocked on playback (reason=%q)", reason)
}
