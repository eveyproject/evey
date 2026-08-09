package postgres

import "github.com/evey-project/evey/internal/domain/library"

// SearchWhereForTest exposes the SQL fallback's WHERE builder to the package's
// external test file. The builder is pure (no DB), which is the only way to
// pin the placeholder numbering and the "filters alone are a search" rule
// without standing up Postgres for what is ultimately string assembly.
func SearchWhereForTest(query string, filters map[string]string) (string, []any, bool) {
	return searchWhere(query, filters)
}

// SortClauseForTest exposes the SQL fallback's ORDER BY builder, pure like the
// WHERE builder above.
func SortClauseForTest(sort []string) string {
	return buildSortClause(sort)
}

// SortColumnForTest resolves a sort key exactly as every non-search listing path
// does, so a test can pin the two fallbacks to the same column.
func SortColumnForTest() string {
	_, col := resolveSortKey(library.ListMediaItemsFilter{})
	return col
}
