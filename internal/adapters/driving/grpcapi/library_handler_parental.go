package grpcapi

import (
	"context"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/ratings"
	"github.com/eveyproject/evey/internal/pkg/region"
)

// library_handler_parental.go holds the parental content filter resolved from
// the caller's profile and applied to every media-item response.

type parentalFilter struct {
	allowedRatings []string // SQL pre-filter: all ratings (across countries) allowed under maxAge
	maxAge         int      // -1 = no age filter
	region         string   // profile's resolved region for per-country cert lookup
	blockedTags    []string
	locale         string // profile locale, used to resolve region when not set
	// blockAll is the unreadable-profile answer: the parental restriction
	// lives on a profile we failed to load, so nothing passes. See the
	// resolver for why this is not "age 0".
	blockAll bool
}

// enforced reports whether this filter actually restricts anything (an age cap
// or at least one blocked tag). A zero-value / no-op filter (maxAge == -1, no
// tags) returns false.
func (pf parentalFilter) enforced() bool {
	return pf.blockAll || pf.maxAge >= 0 || len(pf.blockedTags) > 0
}

func (h *libraryHandler) resolveParentalFilter(ctx context.Context) parentalFilter {
	return resolveParentalFilterFromUserQuery(ctx, h.userQuery)
}

// resolveParentalFilterFromUserQuery is the package-level entry point used
// by every handler that needs to filter results by the active profile's
// parental settings. Returns a "no-op" filter (maxAge=-1, no blocked tags)
// when the caller is unauthenticated or the profile has no effective
// enforcement (i.e. is not a kids profile); a failed profile lookup fails
// closed with the most restrictive filter.
func resolveParentalFilterFromUserQuery(ctx context.Context, userQuery *appuser.QueryService) parentalFilter {
	if userQuery == nil {
		return parentalFilter{maxAge: -1}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" {
		return parentalFilter{maxAge: -1}
	}
	// Admins manage parental settings on other profiles; they must not be
	// subject to one themselves, even when their own profile somehow ended
	// up with IsKids=true (UI typo, migration default flip, single-user
	// servers reusing the admin profile for kids browsing, etc.). Bypassing
	// the filter here is the right layer: SQL pre-filter and post-pagination
	// strict check both consult this resolver.
	if claims.IsAdmin {
		return parentalFilter{maxAge: -1}
	}
	profile, err := userQuery.GetProfileByID(ctx, claims.ProfileID)
	if err != nil || profile == nil {
		// Fail closed: the parental restriction lives on the profile we just
		// failed to load. A no-op filter here would let a kids profile browse
		// the full catalogue on a transient DB error, so return the most
		// restrictive filter instead (admins were already exempted above).
		//
		// Nothing passes, which is what the HTTP side (parental.go's
		// BlockedByParental) has always answered. Until 2026-08-05 this side
		// returned an age-0 filter instead, so a G-rated title still showed
		// through: the same unreadable profile saw different catalogues
		// depending on which door it came in by. Unifying was a product
		// decision (what may an unidentified viewer see - the safe subset, or
		// nothing) and the answer is nothing.
		// TestResolveParentalFilter_FailsClosedWhenTheProfileCannotBeLoaded
		// pins this side.
		return parentalFilter{allowedRatings: ratings.AllowedRatingsForAge(0), maxAge: 0, blockAll: true}
	}
	ps := profile.EffectiveParental()
	if ps == nil {
		return parentalFilter{maxAge: -1}
	}
	age := -1
	if ps.MaxAge != nil {
		age = *ps.MaxAge
	}
	return parentalFilter{
		allowedRatings: ratings.AllowedRatingsForAge(age),
		maxAge:         age,
		region:         region.Resolve(profile.Region, "", profile.Locale),
		locale:         profile.Locale,
		blockedTags:    ps.BlockedTags,
	}
}

// mediaItemBlockedByParental returns true if the given item violates the
// resolved parental filter. Conservative for kids: when no rating is
// resolvable (no certifications anywhere, no official rating mapped),
// the item is blocked rather than silently allowed - otherwise a movie
// like Outbreak with only "AU: MA 15+" / "GB: 15" certifications would
// slip past a profile region'd to FR because there is no FR cert and
// the global ratings.IsAllowedByAge defaults to permissive.
//
// We pick the worst-case (highest) age cap across:
//  1. the profile's region cert (if present)
//  2. the OfficialRating field
//  3. every other country cert we can map
//
// If none of those map to a known min-age, we fail closed for the kids
// profile.
func mediaItemBlockedByParental(pf parentalFilter, item *library.MediaItem) bool {
	if item == nil {
		return false
	}
	if !pf.enforced() {
		return false
	}
	if pf.blockAll {
		return true
	}
	if pf.maxAge >= 0 {
		var certByCountry map[string]string
		var officialRating string
		if item.Classification != nil {
			certByCountry = item.Classification.CertificationsByCountry
			officialRating = item.Classification.OfficialRating
		}
		// Worst-case across the region cert, the OfficialRating and every known
		// foreign cert. A kid profile in FR shouldn't be the place to discover
		// that an MA 15+ Australian rating existed all along.
		worstAge := ratings.WorstCaseMinAge(pf.region, officialRating, certByCountry)
		if worstAge < 0 || worstAge > pf.maxAge {
			// Unresolvable rating fails closed for kids - admin can expose the
			// item explicitly via a metadata override if it really is suitable.
			return true
		}
	}
	if ratings.HasBlockedTag(itemTags(item), pf.blockedTags) {
		return true
	}
	return false
}

func (h *libraryHandler) isBlockedByParental(ctx context.Context, item *library.MediaItem) bool {
	return mediaItemBlockedByParental(h.resolveParentalFilter(ctx), item)
}

// filterOutParentalBlocked drops the items the resolved parental filter blocks,
// reusing the backing array. A no-op (returns the slice unchanged) when the
// filter is not enforced.
func filterOutParentalBlocked(pf parentalFilter, items []*library.MediaItem) []*library.MediaItem {
	if !pf.enforced() {
		return items
	}
	kept := items[:0]
	for _, it := range items {
		if !mediaItemBlockedByParental(pf, it) {
			kept = append(kept, it)
		}
	}
	return kept
}

func itemTags(item *library.MediaItem) []string {
	if item == nil || item.Catalog == nil {
		return nil
	}
	return item.Catalog.Tags
}
