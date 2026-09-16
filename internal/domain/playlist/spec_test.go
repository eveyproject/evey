package playlist

import (
	"errors"
	"strings"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestCompileRules_UnknownField(t *testing.T) {
	_, err := CompileRules([]Rule{{Field: "bogus", Value: "x"}})
	if !errors.Is(err, ErrUnknownRuleField) {
		t.Fatalf("expected ErrUnknownRuleField, got %v", err)
	}
}

func TestCompileRules_Empty(t *testing.T) {
	spec, err := CompileRules(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{}) {
		t.Fatal("empty spec must accept any candidate")
	}
}

func TestGenreSpec_CaseInsensitive(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "genre", Value: "Action"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{Genres: []string{"action"}}) {
		t.Fatal("expected case-insensitive genre match")
	}
	if spec.IsSatisfiedBy(Candidate{Genres: []string{"Drama"}}) {
		t.Fatal("non-matching genre should be rejected")
	}
}

func TestYearSpec_Operators(t *testing.T) {
	cases := []struct {
		op    string
		value string
		year  int
		want  bool
	}{
		{"eq", "2020", 2020, true},
		{"eq", "2020", 2019, false},
		{"gt", "2000", 2001, true},
		{"gt", "2000", 2000, false},
		{"lte", "1999", 1999, true},
		{"lte", "1999", 2000, false},
	}
	for _, c := range cases {
		spec, err := CompileRules([]Rule{{Field: "year", Operator: c.op, Value: c.value}})
		if err != nil {
			t.Fatalf("op=%s: %v", c.op, err)
		}
		got := spec.IsSatisfiedBy(Candidate{Year: new(c.year)})
		if got != c.want {
			t.Fatalf("op=%s value=%s year=%d: got %v want %v", c.op, c.value, c.year, got, c.want)
		}
	}
}

func TestYearSpec_NilYearAlwaysFails(t *testing.T) {
	spec, _ := CompileRules([]Rule{{Field: "year", Operator: "eq", Value: "2020"}})
	if spec.IsSatisfiedBy(Candidate{Year: nil}) {
		t.Fatal("nil year must not match")
	}
}

func TestRatingGteSpec(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "rating_gte", Value: "7.5"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{CommunityRating: floatPtr(8.0)}) {
		t.Fatal("rating 8.0 should match >= 7.5")
	}
	if spec.IsSatisfiedBy(Candidate{CommunityRating: floatPtr(7.0)}) {
		t.Fatal("rating 7.0 should not match >= 7.5")
	}
	if spec.IsSatisfiedBy(Candidate{CommunityRating: nil}) {
		t.Fatal("nil rating should not match")
	}
}

func TestAndSpec_AllMustPass(t *testing.T) {
	spec, err := CompileRules([]Rule{
		{Field: "type", Value: "movie"},
		{Field: "genre", Value: "Action"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{Type: "movie", Genres: []string{"Action"}}) {
		t.Fatal("expected match on both rules")
	}
	if spec.IsSatisfiedBy(Candidate{Type: "movie", Genres: []string{"Drama"}}) {
		t.Fatal("mismatched genre should fail the AND")
	}
}

func TestAndSpec_ToSQL_Joins(t *testing.T) {
	spec, err := CompileRules([]Rule{
		{Field: "type", Value: "movie"},
		{Field: "year", Operator: "gte", Value: "2020"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sq, ok := spec.(SQLSpec)
	if !ok {
		t.Fatal("And must implement SQLSpec")
	}
	argIdx := 1
	clause, args := sq.ToSQL(&argIdx)
	if clause != "mi.type = $1 AND mc.year >= $2" {
		t.Fatalf("clause = %q", clause)
	}
	if len(args) != 2 || args[0] != "movie" || args[1] != 2020 {
		t.Fatalf("args = %v", args)
	}
	if argIdx != 3 {
		t.Fatalf("argIdx = %d, want 3", argIdx)
	}
}

func TestGenreSpec_ToSQL_EscapesQuotes(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "genre", Value: `Sci"Fi`}})
	if err != nil {
		t.Fatal(err)
	}
	sq := spec.(SQLSpec)
	argIdx := 1
	_, args := sq.ToSQL(&argIdx)
	if len(args) != 1 {
		t.Fatalf("args len = %d", len(args))
	}
	got, ok := args[0].(string)
	if !ok {
		t.Fatalf("arg not string: %T", args[0])
	}
	if got != `["Sci\"Fi"]` {
		t.Fatalf("escaped genre = %q", got)
	}
}

// Every ToSQL advances the shared placeholder index by EXACTLY one. Mutation
// testing turned `*argIdx++` into a decrement and nothing failed - yet a wrong
// index means the generated `$N` no longer lines up with the args slice, i.e. a
// smart playlist silently filters on the wrong value (or the query errors out).
func TestGenreSpec_ToSQL_AdvancesThePlaceholderIndexByOne(t *testing.T) {
	idx := 1
	clause, args := genreSpec{genre: "Drame"}.ToSQL(&idx)

	if idx != 2 {
		t.Errorf("argIdx = %d after one clause, want 2", idx)
	}
	if !strings.Contains(clause, "$1") {
		t.Errorf("clause %q must reference the placeholder it consumed", clause)
	}
	if len(args) != 1 {
		t.Fatalf("got %d args, want 1", len(args))
	}

	// A second clause must take the NEXT placeholder, which is the whole point
	// of threading the index through.
	clause2, _ := genreSpec{genre: "Action"}.ToSQL(&idx)
	if idx != 3 {
		t.Errorf("argIdx = %d after two clauses, want 3", idx)
	}
	if !strings.Contains(clause2, "$2") {
		t.Errorf("second clause %q must reference $2", clause2)
	}
}

// The rating filter is "at least", so a title sitting exactly on the threshold
// is included. `>=` mutated to `>` survived, which would silently drop every
// title whose rating equals the filter - the most common case for round numbers.
func TestRatingGteSpec_IncludesTheExactThreshold(t *testing.T) {
	spec := ratingGteSpec{min: 7}
	atThreshold, above, below := 7.0, 7.1, 6.9

	if !spec.IsSatisfiedBy(Candidate{CommunityRating: &atThreshold}) {
		t.Error("a rating exactly equal to the minimum must match")
	}
	if !spec.IsSatisfiedBy(Candidate{CommunityRating: &above}) {
		t.Error("a rating above the minimum must match")
	}
	if spec.IsSatisfiedBy(Candidate{CommunityRating: &below}) {
		t.Error("a rating below the minimum must not match")
	}
	if spec.IsSatisfiedBy(Candidate{CommunityRating: nil}) {
		t.Error("an unrated title must not match a minimum-rating filter")
	}
}

// library_id is the rule that scopes a smart playlist to one library: in
// memory it must match the exact id, in SQL it must bind the id as a
// placeholder (never inline it) and advance the index like every other spec.
func TestLibraryIDSpec_MatchesExactlyAndBindsInSQL(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "library_id", Value: "lib-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{LibraryID: "lib-1"}) {
		t.Fatal("same library must satisfy")
	}
	if spec.IsSatisfiedBy(Candidate{LibraryID: "lib-10"}) || spec.IsSatisfiedBy(Candidate{}) {
		t.Fatal("a different or empty library must not satisfy (exact match, not prefix)")
	}
	argIdx := 4
	clause, args := spec.(SQLSpec).ToSQL(&argIdx)
	if clause != "mi.library_id = $4" {
		t.Fatalf("clause = %q", clause)
	}
	if len(args) != 1 || args[0] != "lib-1" {
		t.Fatalf("args = %v", args)
	}
	if argIdx != 5 {
		t.Fatalf("argIdx = %d, want 5", argIdx)
	}
	// An empty value is "no rule", not "match nothing".
	spec, err = CompileRules([]Rule{{Field: "library_id"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{LibraryID: "anything"}) {
		t.Fatal("an empty library_id rule must be dropped, not turned into an impossible filter")
	}
}

func TestRatingGteSpec_ToSQL_CastsToDoublePrecision(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "rating_gte", Value: "7.5"}})
	if err != nil {
		t.Fatal(err)
	}
	argIdx := 1
	clause, args := spec.(SQLSpec).ToSQL(&argIdx)
	if clause != "mcl.community_rating >= $1::double precision" {
		t.Fatalf("clause = %q", clause)
	}
	if len(args) != 1 || args[0] != 7.5 {
		t.Fatalf("args = %v", args)
	}
	if argIdx != 2 {
		t.Fatalf("argIdx = %d, want 2", argIdx)
	}
	if _, err := CompileRules([]Rule{{Field: "rating_gte", Value: "high"}}); err == nil {
		t.Fatal("a non-numeric rating threshold must be refused, not silently dropped")
	}
}
