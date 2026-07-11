package postgres

// SortColumnMap maps proto/API sort field names to SQL column names.
// All entries are table-qualified so they slot directly into queries
// that JOIN media_items (mi) with the sub-aggregates (mc, mcl) - see
// mediaItemFromJoins.
var SortColumnMap = map[string]string{
	"title":            "mi.sort_title",
	"sort_title":       "mi.sort_title",
	"added_at":         "mi.added_at",
	"date_added":       "mi.added_at",
	"year":             "mc.year",
	"premiere_date":    "mc.premiere_date",
	"community_rating": "mcl.community_rating",
}
