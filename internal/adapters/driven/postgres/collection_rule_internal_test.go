package postgres

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

// TestBuildCollectionRuleCondition covers the defensive validation of
// smart-collection rules: the web client (SmartCollectionDialog) only emits
// valid operators, numeric year/rating values and strips empty-value rules,
// but a malformed API client could send garbage. The builder must skip such
// rules (return "", nil) rather than produce a 500 cast error, a
// match-nothing predicate, or silently coerce an unknown operator to "=".
func TestBuildCollectionRuleCondition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		rule     library.CollectionRule
		wantCond string // "" means the rule must be skipped
		wantArg  any    // nil when skipped
	}{
		// --- valid rules from the web client ---
		{"genre", library.CollectionRule{Field: "genre", Operator: "eq", Value: "Action"}, "mc.genres @> $1::jsonb", `["Action"]`},
		{"type", library.CollectionRule{Field: "type", Operator: "eq", Value: "movie"}, "mi.type = $1", "movie"},
		{"library_id", library.CollectionRule{Field: "library_id", Operator: "eq", Value: "lib-1"}, "mi.library_id = $1", "lib-1"},
		{"year eq", library.CollectionRule{Field: "year", Operator: "eq", Value: "2020"}, "mc.year = $1", "2020"},
		{"year empty-op defaults to eq", library.CollectionRule{Field: "year", Operator: "", Value: "2020"}, "mc.year = $1", "2020"},
		{"year gt", library.CollectionRule{Field: "year", Operator: "gt", Value: "1999"}, "mc.year > $1", "1999"},
		{"year lt", library.CollectionRule{Field: "year", Operator: "lt", Value: "1999"}, "mc.year < $1", "1999"},
		{"year gte", library.CollectionRule{Field: "year", Operator: "gte", Value: "2000"}, "mc.year >= $1", "2000"},
		{"year lte", library.CollectionRule{Field: "year", Operator: "lte", Value: "2000"}, "mc.year <= $1", "2000"},
		{"rating", library.CollectionRule{Field: "rating_gte", Operator: "gte", Value: "7.5"}, "mcl.community_rating >= $1::double precision", "7.5"},

		// --- malformed rules: must be skipped ---
		{"empty value", library.CollectionRule{Field: "type", Operator: "eq", Value: ""}, "", nil},
		{"empty genre value", library.CollectionRule{Field: "genre", Operator: "eq", Value: ""}, "", nil},
		{"unknown year operator", library.CollectionRule{Field: "year", Operator: "between", Value: "2020"}, "", nil},
		{"non-numeric year", library.CollectionRule{Field: "year", Operator: "gte", Value: "abc"}, "", nil},
		{"non-numeric rating", library.CollectionRule{Field: "rating_gte", Operator: "gte", Value: "good"}, "", nil},
		{"unknown field", library.CollectionRule{Field: "director", Operator: "eq", Value: "x"}, "", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			argIdx := 1
			cond, args := buildCollectionRuleCondition(tc.rule, &argIdx)
			if cond != tc.wantCond {
				t.Fatalf("cond = %q, want %q", cond, tc.wantCond)
			}
			if tc.wantCond == "" {
				if args != nil {
					t.Fatalf("skipped rule returned args %v, want nil", args)
				}
				if argIdx != 1 {
					t.Fatalf("skipped rule advanced argIdx to %d, want 1 (no placeholder consumed)", argIdx)
				}
				return
			}
			if len(args) != 1 {
				t.Fatalf("got %d args, want 1", len(args))
			}
			if got := args[0]; got != tc.wantArg {
				t.Fatalf("arg = %v, want %v", got, tc.wantArg)
			}
			if argIdx != 2 {
				t.Fatalf("argIdx = %d, want 2 (one placeholder consumed)", argIdx)
			}
		})
	}
}
