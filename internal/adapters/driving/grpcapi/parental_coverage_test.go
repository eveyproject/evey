package grpcapi

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// This guard exists because the parental gate has been forgotten three separate
// times, always the same way: a new path that serves media is written, and
// nobody remembers that a sibling path applies a parental check. Reviews do not
// catch it because the omission looks like nothing at all.
//
// It is modelled on TestAllRegisteredMethodsAreClassified (the RBAC guard):
// enumerate the REAL generated descriptors, and refuse to pass when an RPC that
// can hand media metadata to a caller sits in none of the three buckets below
// (gated / exempt / known-gap).
//
// What it proves and what it does not: it forces every media-returning RPC to
// be classified deliberately. It does NOT prove the handler actually calls a
// gate - that would need static analysis of the call graph. The value is that a
// newly added media path cannot stay silently unclassified: the build goes red
// until someone decides, and writes down why.
//
// Why three buckets and not two: four RPCs genuinely return unfiltered media
// metadata today and that was reviewed and accepted. Filing them under "exempt"
// would have made this guard assert something false, which is worse than the
// gap it documents.

// mediaCarryingMessages are response messages that hand media to a caller, by
// name. Kept as a backstop for shapes the structural rule below would miss.
//
// Matching on names alone was the first cut and it was too narrow: FeedItem and
// FavoriteProto carry title/type/year/artwork - the very metadata a restricted
// profile must not see - without "MediaItem" anywhere in their name. A response
// type can always be named something new, so the primary rule is structural.
var mediaCarryingMessages = map[string]bool{
	"MediaItem":        true,
	"PublicMediaItem":  true,
	"PopularMediaItem": true,
}

// mediaRefFields are the structural tell: a response that points at a media
// item carries its id. That is far harder to sidestep by accident than a name,
// since any new shape describing media needs the id to be useful to a client.
//
// The set is deliberately wider than one spelling. Matching only
// "media_item_id" left SyncPlay (media_id), Collections (media_item_ids) and
// Share (media_id) structurally INVISIBLE to this guard - and that blind spot
// is exactly how a full parental bypass shipped through SyncPlay: the RPC was
// never presented for a decision, so nobody declined to gate it. A guard that
// silently skips what it was built to catch is worse than no guard.
var mediaRefFields = map[string]bool{
	"media_item_id":  true,
	"media_item_ids": true,
	"media_id":       true,
	"media_ids":      true,
}

// parentalGated lists RPCs that DO apply a parental gate, with the mechanism
// used. The value is documentation, not assertion - see the caveat above.
var parentalGated = map[string]string{
	// SyncPlay: all three go through requireMediaAccess, which applies the same
	// resolved parental filter as the library handlers. Added 2026-07-21 after
	// an audit found the gate missing entirely: library access is per USER but
	// parental is per PROFILE, so a kids profile cleared the access check for
	// the whole household library. Worse, a room laundered the media - the
	// public join-code route mints a media-scoped guest token carrying NO
	// profile, and the parental gate no-ops on an empty profile.
	"/evey.v1.SyncPlayService/CreateRoom":           "requireMediaAccess -> mediaItemBlockedByParental",
	"/evey.v1.SyncPlayService/JoinByCode":           "requireMediaAccess -> mediaItemBlockedByParental",
	"/evey.v1.SyncPlayService/GetRoom":              "requireMediaAccess -> mediaItemBlockedByParental",
	"/evey.v1.CollectionService/GetCollectionItems": "per-item mediaItemBlockedByParental with total recounted",

	// Browse / detail: the SQL pre-filter plus a strict post-pagination check.
	"/evey.v1.LibraryService/GetMediaItem":         "loadGatedMediaItem -> isBlockedByParental",
	"/evey.v1.LibraryService/ListMediaItems":       "parentalFilter SQL pre-filter + strict post-filter",
	"/evey.v1.LibraryService/SearchMedia":          "parentalFilter + per-hit mediaItemBlockedByParental (and TotalHits recounted)",
	"/evey.v1.LibraryService/FilterMediaItems":     "library access + parentalFilter",
	"/evey.v1.LibraryService/GetPublicMediaItem":   "public endpoint, still parental-filtered",
	"/evey.v1.LibraryService/ListPublicMediaItems": "public endpoint, still parental-filtered",

	// Feeds: the browse surface a kids profile actually navigates.
	"/evey.v1.FeedService/ContinueWatching": "filterAccessibleAndAllowed",
	"/evey.v1.FeedService/NextEpisode":      "filterAccessibleAndAllowed",
	"/evey.v1.FeedService/RecentlyAdded":    "filterAccessibleAndAllowed",
	"/evey.v1.FeedService/Recommended":      "filterAccessibleAndAllowed",
	"/evey.v1.FeedService/ResumePlayback":   "filterAccessibleAndAllowed",
	"/evey.v1.FeedService/SimilarItems":     "filterAccessibleAndAllowed",

	"/evey.v1.FavoriteService/ListFavorites":   "per-item mediaItemBlockedByParental",
	"/evey.v1.ActivityService/GetPopularMedia": "per-item mediaItemBlockedByParental",

	// Playback: the paths that actually hand over a stream.
	"/evey.v1.PlaybackService/GetStreamInfo":       "isBlockedByParental -> shared resolver, answers NotFound to hide existence",
	"/evey.v1.PlaybackService/StartStream":         "isBlockedByParental -> shared resolver, answers NotFound to hide existence",
	"/evey.v1.PlaybackService/GetNextUp":           "per-item mediaItemBlockedByParental",
	"/evey.v1.PlaybackService/GetSeriesPlayTarget": "target episode mediaItemBlockedByParental",
	"/evey.v1.PlaybackService/GetResumeItems":      "per-item mediaItemBlockedByParental",
}

// parentalExempt lists media-returning RPCs that legitimately need no gate,
// each with the reason it is safe. An entry here is a claim someone made on
// purpose; a wrong one is a bug, but at least it is a visible one.
var parentalExempt = map[string]string{
	// Admin-only at the RBAC interceptor. Admins are exempt from the parental
	// filter everywhere by design (they manage it), so gating these would be
	// contradictory - see resolveParentalFilterFromUserQuery.
	"/evey.v1.TranscodingService/ListActiveTranscodes":    "RoleAdmin",
	"/evey.v1.ShareService/ListShareLinks":                "RoleAdmin",
	"/evey.v1.NotificationService/ListAdminNotifications": "RoleAdmin",
	"/evey.v1.ShareService/CreateShareLink":               "RoleAdmin",
	"/evey.v1.AdminMediaService/ScanSeekIndex":            "RoleAdmin",
	"/evey.v1.AdminMediaService/GetMediaRepairHistory":    "RoleAdmin",
	"/evey.v1.AdminMediaService/ListMediaRepairs":         "RoleAdmin",
	"/evey.v1.AdminSessionService/ListActiveSessions":     "RoleAdmin",
	"/evey.v1.ActivityService/GetServerStats":             "RoleAdmin",
	"/evey.v1.ActivityService/GetAnalytics":               "admin enforced in the handler (PermissionDenied for non-admin)",

	// Operate on ONE caller-supplied id with a library-access check; they are
	// not enumeration surfaces, so they cannot be used to discover a blocked
	// title. Reaching them needs an id the caller could only get from a listing
	// path, and every listing path above is filtered.
	"/evey.v1.MetadataService/RefreshMetadata":       "per-item access check, single known id, no enumeration",
	"/evey.v1.MetadataService/SetMetadataOverride":   "per-item access check, single known id, no enumeration",
	"/evey.v1.MetadataService/ListMetadataOverrides": "per-item access check, single known id, no enumeration",
	"/evey.v1.MetadataService/ResearchMetadata":      "per-item access check, single known id, no enumeration",
	"/evey.v1.PlaybackService/GetWatchProgress":      "the caller's own progress for one known id, no enumeration",
	"/evey.v1.TranscodingService/GetTranscodeStatus": "the caller's own job (checkJobAccess), no enumeration",
	"/evey.v1.NotificationService/ListNotifications": "the caller's own notifications",
}

// parentalKnownGap is the honest third bucket: RPCs that return media metadata
// WITHOUT a parental filter, where that has been looked at and accepted rather
// than fixed. Putting these in parentalExempt would make the guard lie.
//
// All of them are scoped to the CALLER'S OWN data by resolveProfileID
// (helpers.go), which never returns another profile - so none of them leaks
// across users. A restricted title can only surface here if the rules changed
// after the fact (a parent lowers MaxAge, or an existing profile is switched to
// kids), and in that case the child had already seen the content. Triaged with
// the owner on 2026-07-20 as low priority, and a consistency question rather
// than a security one: ListFavorites does filter this case, these do not.
var parentalKnownGap = map[string]string{
	"/evey.v1.PlaybackService/GetWatchHistory":  "own history; enrichWatchHistory filters library access only, unlike GetNextUp/GetResumeItems in the same file",
	"/evey.v1.ActivityService/GetPlayHistory":   "own history; titles unfiltered, unlike GetPopularMedia in the same file",
	"/evey.v1.ActivityService/GetUserStats":     "own stats; MostPlayedItems titles unfiltered",
	"/evey.v1.PlaylistService/GetPlaylistItems": "own playlists; library access only, unlike ListFavorites",
	"/evey.v1.MediaRequestService/SearchTitles": "searches the external provider, not the library; the InLibrary flag can reveal that a title is held. Gated on the request policy (CanRequest) since 2026-07-20",
	"/evey.v1.ShareService/ValidateShareLink":   "describes a link the caller already holds the opaque token for; Peek only, no view spent. The link itself was minted by an admin (CreateShareLink is RoleAdmin), so a restricted profile cannot mint one - it can only describe a token someone handed it, which is the point of a share link",
}

// containsMedia reports whether msg transitively holds a media-carrying
// message. Depth-bounded and cycle-safe: proto messages can self-reference.
func containsMedia(msg protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool, depth int) bool {
	if msg == nil || depth > 6 || seen[msg.FullName()] {
		return false
	}
	seen[msg.FullName()] = true
	if mediaCarryingMessages[string(msg.Name())] {
		return true
	}
	fields := msg.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if mediaRefFields[string(f.Name())] {
			return true
		}
		if f.Kind() != protoreflect.MessageKind && f.Kind() != protoreflect.GroupKind {
			continue
		}
		if containsMedia(f.Message(), seen, depth+1) {
			return true
		}
	}
	return false
}

func TestAllMediaReturningRPCsAreParentalClassified(t *testing.T) {
	var unclassified []string
	var checked int
	real := make(map[string]bool)

	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if !strings.HasPrefix(string(svc.FullName()), "evey.v1.") {
				continue // ignore google.* / grpc.* infra services
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				m := methods.Get(j)
				if !containsMedia(m.Output(), map[protoreflect.FullName]bool{}, 0) {
					continue
				}
				full := "/" + string(svc.FullName()) + "/" + string(m.Name())
				real[full] = true
				checked++
				_, gated := parentalGated[full]
				_, exempt := parentalExempt[full]
				_, gap := parentalKnownGap[full]
				n := 0
				for _, in := range []bool{gated, exempt, gap} {
					if in {
						n++
					}
				}
				switch {
				case n > 1:
					t.Errorf("RPC %s is listed in more than one bucket - pick one", full)
				case n == 0:
					unclassified = append(unclassified, full)
				}
			}
		}
		return true
	})

	// Guard against a vacuous pass: if the descriptors are not registered the
	// walk finds nothing and every assertion above is skipped.
	require.Positive(t, checked, "no media-returning evey.v1 RPC discovered - proto descriptors not registered?")

	for _, m := range unclassified {
		t.Errorf("RPC %s can return media metadata but is in no parental bucket.\n"+
			"    parentalGated  - it filters restricted content (name the mechanism)\n"+
			"    parentalExempt - it genuinely cannot leak to a restricted profile (say why)\n"+
			"    parentalKnownGap - it does NOT filter and that has been accepted (say why)\n"+
			"    Do not guess: read the handler. This guard exists because the gate was\n"+
			"    forgotten on a sibling path three times.", m)
	}

	// Reverse check: no stale entry for an RPC that no longer exists or no
	// longer returns media (catches renames and typos).
	for m := range parentalGated {
		require.Truef(t, real[m], "parentalGated lists %s but no such media-returning evey.v1 RPC exists", m)
	}
	for m := range parentalExempt {
		require.Truef(t, real[m], "parentalExempt lists %s but no such media-returning evey.v1 RPC exists", m)
	}
	for m := range parentalKnownGap {
		require.Truef(t, real[m], "parentalKnownGap lists %s but no such media-returning evey.v1 RPC exists", m)
	}
}

// The age cap comparison in mediaItemBlockedByParental is `worstAge > pf.maxAge`,
// i.e. inclusive: a title rated exactly at the cap must be ALLOWED. A mutant
// flipping `>` to `>=` would over-block (e.g. reject PG-13 under a 13-year cap)
// and no existing test exercises the exact boundary - they all use a clear
// margin either side. Ratings resolved via ratings.WorstCaseMinAge (see
// internal/domain/ratings/ratings.go): US "PG-13" -> 13, "TV-14" -> 14, "G" -> 0.
func TestMediaItemBlockedByParental_AgeBoundary(t *testing.T) {
	maxAge13 := 13
	pf13 := parentalFilter{maxAge: maxAge13, region: "US"}

	pg13 := &library.MediaItem{
		Classification: &library.MediaClassification{OfficialRating: "PG-13"},
	}
	assert.False(t, mediaItemBlockedByParental(pf13, pg13),
		"worstAge == maxAge must be allowed (inclusive cap)")

	tv14 := &library.MediaItem{
		Classification: &library.MediaClassification{OfficialRating: "TV-14"},
	}
	assert.True(t, mediaItemBlockedByParental(pf13, tv14),
		"worstAge one year over the cap must be blocked")

	maxAge0 := 0
	pf0 := parentalFilter{maxAge: maxAge0, region: "US"}
	allAges := &library.MediaItem{
		Classification: &library.MediaClassification{OfficialRating: "G"},
	}
	assert.False(t, mediaItemBlockedByParental(pf0, allAges),
		"an all-ages rating must be allowed even at the strictest cap")
}

// Every exemption must carry a real justification - an empty string would let
// someone silence the guard without thinking.
func TestParentalExemptionsAreJustified(t *testing.T) {
	for m, reason := range parentalExempt {
		require.NotEmptyf(t, strings.TrimSpace(reason), "parentalExempt[%s] needs a reason", m)
	}
	for m, mechanism := range parentalGated {
		require.NotEmptyf(t, strings.TrimSpace(mechanism), "parentalGated[%s] must name the mechanism", m)
	}
	for m, reason := range parentalKnownGap {
		require.NotEmptyf(t, strings.TrimSpace(reason), "parentalKnownGap[%s] needs a reason", m)
	}
}
