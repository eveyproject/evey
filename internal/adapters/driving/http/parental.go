package httpapi

import (
	"context"
	"net/http"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/ratings"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/region"
)

// ParentalChecker decides whether the active profile may view an item under its
// parental settings. The raw byte/decision/token HTTP handlers (HLS master,
// direct stream, transcode, download, snapshot, playback-info, stream-token)
// authorise library access but historically served content without consulting
// the kids filter that lives in the gRPC layer - so a kids profile that knew an
// adult media-id out of band could still stream it. These handlers run the
// check via checkParental.
type ParentalChecker interface {
	// BlockedByParental reports whether the given profile must not view item.
	// item is the already-resolved MediaItem (carrying its Classification), so
	// no extra fetch is needed.
	BlockedByParental(ctx context.Context, profileID string, item *domainlibrary.MediaItem) bool
}

// ProfileParentalGetter resolves a profile's parental configuration. Satisfied
// by *appuser.QueryService.
type ProfileParentalGetter interface {
	GetProfileByID(ctx context.Context, profileID string) (*domainuser.Profile, error)
}

type profileParentalChecker struct {
	profiles ProfileParentalGetter
}

// NewParentalChecker builds the ParentalChecker used by the streaming handlers.
// A nil getter yields a checker that never blocks (parental simply not wired).
func NewParentalChecker(p ProfileParentalGetter) ParentalChecker {
	return &profileParentalChecker{profiles: p}
}

func (c *profileParentalChecker) BlockedByParental(ctx context.Context, profileID string, item *domainlibrary.MediaItem) bool {
	if c == nil || c.profiles == nil || profileID == "" || item == nil {
		return false
	}
	profile, err := c.profiles.GetProfileByID(ctx, profileID)
	if err != nil || profile == nil {
		// Fail closed: the restriction lives on the profile we just failed to
		// load, so a transient lookup error must not grant more than the most
		// restrictive setting - otherwise a DB blip lets a kids profile stream
		// adult content. Admins are exempted upstream (checkParental).
		//
		// This blocks EVERYTHING, including a G-rated title - the same answer
		// as the gRPC side (resolveParentalFilterFromUserQuery's blockAll,
		// unified 2026-08-05), so an unreadable profile sees nothing and can
		// play nothing, whichever door it comes in by.
		return true
	}
	ps := profile.EffectiveParental()
	if ps == nil {
		return false
	}
	// The gRPC side's rule (ratings.Blocks): worst case across the item and,
	// for a season or an episode, its series; an unresolvable rating is
	// blocked for a kids profile.
	maxAge := -1
	if ps.MaxAge != nil {
		maxAge = *ps.MaxAge
	}
	own := ratings.Rated{}
	if item.Classification != nil {
		own.OfficialRating = item.Classification.OfficialRating
		own.Certifications = item.Classification.CertificationsByCountry
	}
	if item.Catalog != nil {
		own.Tags = item.Catalog.Tags
	}
	var series *ratings.Rated
	if sp := item.SeriesParental; sp != nil {
		series = &ratings.Rated{OfficialRating: sp.OfficialRating, Certifications: sp.CertificationsByCountry, Tags: sp.Tags}
	}
	return ratings.Blocks(maxAge, region.Resolve(profile.Region, "", profile.Locale), ps.BlockedTags, own, series)
}

// checkParental writes a 404 and returns false when the active profile may not
// view item. It is a no-op (returns true) when parental is not wired, the
// caller is an admin, or there are no claims/item - mirroring checkLibraryAccess
// and the gRPC resolver which both exempt admins from a parental filter.
func checkParental(ctx context.Context, w http.ResponseWriter, claims *domainuser.Claims, item *domainlibrary.MediaItem, pc ParentalChecker) bool {
	if pc == nil || claims == nil || claims.IsAdmin || item == nil {
		return true
	}
	if pc.BlockedByParental(ctx, claims.ProfileID, item) {
		// 404, not 403: a parental-blocked adult item must be indistinguishable
		// from a non-existent one, otherwise a kids profile can enumerate which
		// media IDs are adult content by probing the byte/HLS/token endpoints.
		// Mirror checkLibraryAccess's not-found response.
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return false
	}
	return true
}
