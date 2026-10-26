package playlist

import (
	"errors"
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

// The rating filter is "at least", so a title sitting exactly on the threshold
// is included. `>=` mutated to `>` survived, which would silently drop every
// title whose rating equals the filter - the most common case for round numbers.
func TestRatingGteSpec_IncludesTheExactThreshold(t *testing.T) {
	spec := RatingGteSpec{Min: 7}
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
// memory it must match the exact id.
func TestLibraryIDSpec_MatchesExactly(t *testing.T) {
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
	// An empty value is "no rule", not "match nothing".
	spec, err = CompileRules([]Rule{{Field: "library_id"}})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsSatisfiedBy(Candidate{LibraryID: "anything"}) {
		t.Fatal("an empty library_id rule must be dropped, not turned into an impossible filter")
	}
}

func TestRatingGteSpec_ParsesTheThreshold(t *testing.T) {
	spec, err := CompileRules([]Rule{{Field: "rating_gte", Value: "7.5"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.(AndSpec).Specs[0] != (RatingGteSpec{Min: 7.5}) {
		t.Fatalf("spec = %#v", spec)
	}
	if _, err := CompileRules([]Rule{{Field: "rating_gte", Value: "high"}}); err == nil {
		t.Fatal("a non-numeric rating threshold must be refused, not silently dropped")
	}
}
