package postgres

// SearchWhereForTest exposes the SQL fallback's WHERE builder to the package's
// external test file. The builder is pure (no DB), which is the only way to
// pin the placeholder numbering and the "filters alone are a search" rule
// without standing up Postgres for what is ultimately string assembly.
func SearchWhereForTest(query string, filters map[string]string) (string, []any, bool) {
	return searchWhere(query, filters)
}
