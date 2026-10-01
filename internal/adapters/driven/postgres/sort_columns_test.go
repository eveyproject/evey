package postgres

// resolveSortKey is pure, so this needs no database - and it is worth pinning
// because its fallback is silent. An unknown sort field does not error, it
// quietly returns title, which is why "sort by rating" shipped broken on every
// client at once until 2026-09-04.

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
)

func TestResolveSortKey(t *testing.T) {
	cases := []struct {
		name     string
		field    string
		wantKey  string
		wantCol  string
		wantEcho bool // the resolved key must equal the requested field
	}{
		// The regression: the gRPC layer sends "rating", and it must resolve to
		// the rating column. If this ever falls back to title again, two things
		// break at once - the ordering itself, AND cursor pagination, because the
		// cursor is stamped with the REQUESTED field while the query resolves the
		// fallback, and the next page is rejected with ErrCursorMismatch (a 400
		// surfacing as "loading failed" in the apps).
		{"rating resolves to the rating column", "rating", "rating", "mcl.community_rating", true},
		{"community_rating alias still works", "community_rating", "community_rating", "mcl.community_rating", true},
		{"title", "title", "title", "mi.sort_title", true},
		{"year", "year", "year", "mc.year", true},
		{"date_added", "date_added", "date_added", effectiveAddedAt, true},
		{"premiere_date", "premiere_date", "premiere_date", "mc.premiere_date", true},
		// The fallback itself is intended behaviour for input we never emit.
		{"unknown field falls back to title", "no_such_field", "title", "mi.sort_title", false},
		{"empty field falls back to title", "", "title", "mi.sort_title", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, col := resolveSortKey(library.ListMediaItemsFilter{SortField: tc.field})
			if key != tc.wantKey {
				t.Errorf("key = %q, want %q", key, tc.wantKey)
			}
			if col != tc.wantCol {
				t.Errorf("column = %q, want %q", col, tc.wantCol)
			}
			if tc.wantEcho && key != tc.field {
				t.Errorf("resolved key %q differs from the requested field %q: "+
					"the cursor is stamped with the request, so pagination will reject its own cursor", key, tc.field)
			}
		})
	}
}

// Every column in the map must be table-qualified: the query JOINs three tables
// and an unqualified name would either fail or, worse, silently bind to the
// wrong one.
func TestSortColumnMapIsTableQualified(t *testing.T) {
	for key, col := range SortColumnMap {
		if col == effectiveAddedAt {
			continue // an expression over mi.* columns only
		}
		if len(col) < 4 || (col[:3] != "mi." && col[:3] != "mc." && col[:4] != "mcl.") {
			t.Errorf("SortColumnMap[%q] = %q is not qualified with mi./mc./mcl.", key, col)
		}
	}
}
