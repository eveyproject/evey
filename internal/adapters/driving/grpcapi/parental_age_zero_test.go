package grpcapi

import (
	"context"
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// A cap of 0 is a real setting ("only content rated for all ages",
// user.ParentalSettings): it must filter like any other age, not read as "no
// cap". And a profile that only blocks tags carries no age cap at all. Both
// survived mutation testing on 2026-10-18 (`maxAge >= 0` -> `> 0` let an R
// title through an all-ages profile; `age := -1` -> `1` age-filtered a
// tags-only profile) with the whole suite green.
func TestResolveParentalFilter_AgeZeroAndTagsOnly(t *testing.T) {
	zero := 0
	profiles := userfakes.NewFakeProfileRepository()
	for _, p := range []*user.Profile{
		{ID: "p-allages", UserID: "user-1", IsKids: true, Parental: &user.ParentalSettings{MaxAge: &zero}},
		{ID: "p-tags", UserID: "user-1", Parental: &user.ParentalSettings{Enabled: true, BlockedTags: []string{"horror"}}},
	} {
		if err := profiles.Create(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)
	rated := func(cert string) *library.MediaItem {
		return &library.MediaItem{
			Title:          cert + "-rated",
			Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": cert}},
		}
	}

	allAges := resolveParentalFilterFromUserQuery(ctxWithClaims("user-1", string(shared.ProfileID("p-allages")), false), userQuery)
	if !allAges.enforced() {
		t.Fatal("a 0 age cap reads as no restriction")
	}
	if mediaItemBlockedByParental(allAges, rated("G")) {
		t.Error("an all-ages profile was refused a G title")
	}
	if !mediaItemBlockedByParental(allAges, rated("R")) {
		t.Error("an all-ages profile was shown an R title")
	}

	tagsOnly := resolveParentalFilterFromUserQuery(ctxWithClaims("user-1", "p-tags", false), userQuery)
	if tagsOnly.maxAge != -1 {
		t.Errorf("tags-only profile maxAge = %d, want -1 (no age cap)", tagsOnly.maxAge)
	}
	if mediaItemBlockedByParental(tagsOnly, rated("R")) {
		t.Error("a profile that only blocks tags was age-filtered")
	}
	horror := rated("R")
	horror.Catalog = &library.MediaCatalog{Tags: []string{"horror"}}
	if !mediaItemBlockedByParental(tagsOnly, horror) {
		t.Error("a blocked tag passed")
	}
}
