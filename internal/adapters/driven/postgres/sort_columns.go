package postgres

// SortColumnMap maps proto/API sort field names to SQL column names.
// All entries are table-qualified so they slot directly into queries
// that JOIN media_items (mi) with the sub-aggregates (mc, mcl) - see
// mediaItemFromJoins.
// Every name a CLIENT can ask for must be a key here. resolveSortKey falls back
// to title on an unknown key, silently - so a missing entry does not fail, it
// sorts by title and looks like the feature was never implemented. That is
// exactly what "rating" did: the gRPC layer converts MEDIA_SORT_FIELD_RATING to
// "rating" (grpcapi/library_converters.go), this map only knew
// "community_rating", and every library sorted by rating came back in title
// order on web, mobile and TV alike. Reported 2026-09-04.
//
// sortFieldContractTest locks the two sides together: it fails if the gRPC
// converter ever emits a name absent from here.
var SortColumnMap = map[string]string{
	"title":            "mi.sort_title",
	"sort_title":       "mi.sort_title",
	"added_at":         effectiveAddedAt,
	"date_added":       effectiveAddedAt,
	"year":             "mc.year",
	"premiere_date":    "mc.premiere_date",
	"rating":           "mcl.community_rating",
	"community_rating": "mcl.community_rating",
}

// effectiveAddedAt is the date a library sorted by "date added" sees: a series
// moves up when it gets a new episode. Every page has to order and seek on the
// same expression; the first page used to use it and the cursor pages plain
// mi.added_at, so series repeated and rows went missing mid-scroll.
const effectiveAddedAt = `(CASE WHEN mi.type = 'series' THEN COALESCE(mi.latest_child_added_at, mi.added_at) ELSE mi.added_at END)`
