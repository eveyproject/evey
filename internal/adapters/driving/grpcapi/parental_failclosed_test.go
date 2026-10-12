package grpcapi

import (
	"errors"
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// The parental restriction lives on the profile. When that profile cannot be
// loaded - a transient DB error, a row that vanished - the resolver must not
// hand back a no-op filter, or a kids profile browses the whole catalogue for
// as long as the database hiccups.
//
// Nothing pinned this branch: no test in this package ever made
// GetProfileByID fail, so a change returning `parentalFilter{maxAge: -1}`
// (the "no restriction" value used two lines below for admins) would have
// been silently less restrictive and stayed green. Given the history of this
// area - the same guard placed on one path and forgotten on its twin, six
// times over - the branch deserved a test of its own.
func TestResolveParentalFilter_FailsClosedWhenTheProfileCannotBeLoaded(t *testing.T) {
	profiles := userfakes.NewFakeProfileRepository()
	profiles.Err = errors.New("database is having a bad day")
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)

	got := resolveParentalFilterFromUserQuery(ctxWithClaims("user-1", "profile-1", false), userQuery)

	if got.maxAge != 0 {
		t.Errorf("maxAge = %d, want 0 (the most restrictive age); -1 would mean no restriction at all", got.maxAge)
	}
	if len(got.allowedRatings) == 0 {
		t.Error("allowedRatings is empty: an empty allow-list is read as 'no filter' downstream, not as 'allow nothing'")
	}
	// Unified with the HTTP side on 2026-08-05: an unreadable profile sees
	// NOTHING, not the age-0 subset. Without blockAll a G-rated title still
	// reached a viewer whose parental settings could not be read.
	if !got.blockAll {
		t.Error("blockAll = false: an unreadable profile must be blocked outright, like the HTTP side")
	}
	// A certification that resolves to age 0: maxAge 0 alone lets it through,
	// so only blockAll can stop it. A bare "G" with no country does not
	// resolve and failed closed on its own, which left blockAll untested.
	if !mediaItemBlockedByParental(got, &library.MediaItem{
		Title:          "A G-rated title",
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	}) {
		t.Error("a G-rated item passed the filter of a profile whose parental settings could not be read")
	}

	// The admin exemption above this branch must still win - an admin whose
	// profile fails to load is not silently demoted to a kids filter.
	admin := resolveParentalFilterFromUserQuery(ctxWithClaims("user-1", "profile-1", true), userQuery)
	if admin.maxAge != -1 {
		t.Errorf("admin maxAge = %d, want -1 (exempt)", admin.maxAge)
	}
}
