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
// resolved parental filter: ratings.Blocks, the worst case across
// the region's reading of the official rating and every country certification,
// an unresolvable rating blocked for kids (an admin can expose the item with a
// metadata override if it really is suitable). An unreadable profile blocks
// everything.
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
	return parentalRuleBlocks(*pf.rule(), item)
}

// parentalRuleBlocks applies ratings.Blocks to an item under rule, a season
// or an episode with what it inherits from its series.
func parentalRuleBlocks(rule library.ParentalRule, item *library.MediaItem) bool {
	own, series := ratedForParental(item)
	return ratings.Blocks(rule.MaxAge, rule.Region, rule.BlockedTags, own, series)
}

// ratedForParental is what ratings.Blocks reads off an item and, for a season
// or an episode, off its series (nil otherwise).
func ratedForParental(item *library.MediaItem) (ratings.Rated, *ratings.Rated) {
	own := ratings.Rated{Tags: itemTags(item)}
	if item.Classification != nil {
		own.OfficialRating = item.Classification.OfficialRating
		own.Certifications = item.Classification.CertificationsByCountry
	}
	if sp := item.SeriesParental; sp != nil {
		return own, &ratings.Rated{OfficialRating: sp.OfficialRating, Certifications: sp.CertificationsByCountry, Tags: sp.Tags}
	}
	return own, nil
}

// rule is the filter as a list query applies it (ListMediaItemsFilter.Parental).
func (pf parentalFilter) rule() *library.ParentalRule {
	return &library.ParentalRule{MaxAge: pf.maxAge, Region: pf.region, BlockedTags: pf.blockedTags}
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
