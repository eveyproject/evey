package postgres

// SortColumnMap maps proto/API sort field names to SQL column names.
var SortColumnMap = map[string]string{
	"title":            "sort_title",
	"sort_title":       "sort_title",
	"added_at":         "added_at",
	"date_added":       "added_at",
	"year":             "year",
	"premiere_date":    "premiere_date",
	"community_rating": "community_rating",
}
