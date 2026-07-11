package playlist

import (
	"errors"
	"testing"
)

func intPtr(v int) *int           { return &v }
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
		got := spec.IsSatisfiedBy(Candidate{Year: intPtr(c.year)})
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
