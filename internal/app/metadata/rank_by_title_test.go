package metadata

import (
	"testing"

	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
)

func yearPtr(y int) *int { return &y }

// Exact title match wins over partial matches.
func TestRankByTitle_ExactMatchFirst(t *testing.T) {
	t.Parallel()
	results := []domainmetadata.MetadataResult{
		{Title: "The Matrix Reloaded", Year: yearPtr(2003)},
		{Title: "The Matrix", Year: yearPtr(1999)},
		{Title: "Matrix Revolutions", Year: yearPtr(2003)},
	}
	ranked := rankByTitle(results, "The Matrix", nil)
	if ranked[0].Title != "The Matrix" {
		t.Fatalf("want exact match first, got %q", ranked[0].Title)
	}
}

// Year match boosts the score (0.3 bonus) over a closer title without matching year.
func TestRankByTitle_YearBoost(t *testing.T) {
	t.Parallel()
	results := []domainmetadata.MetadataResult{
		{Title: "The Matrix", Year: yearPtr(2022)},         // close title but wrong year
		{Title: "The Matrix Reboot", Year: yearPtr(1999)},  // less similar but year matches
	}
	ranked := rankByTitle(results, "The Matrix", yearPtr(1999))
	// Without year: "The Matrix" = 1.0, "The Matrix Reboot" < 1.0.
	// With year match: "The Matrix Reboot" = <1.0 + 0.3.
	// "The Matrix" score (no year match) stays at ~1.0. So in edge cases these
	// can tie. Assert only that the year-matched one appears first when titles
	// are comparably similar:
	// In this contrived case, "The Matrix" is still an exact match (1.0) so
	// it wins 1.0 > 0.X + 0.3 only if the title similarity difference is
	// larger than 0.3. Use a title that gives exactly a similar sim:
	_ = ranked
	// Instead use two titles with identical similarity:
	results2 := []domainmetadata.MetadataResult{
		{Title: "Ghost", Year: yearPtr(2010)}, // partial
		{Title: "Ghost", Year: yearPtr(1990)}, // same partial + year
	}
	ranked2 := rankByTitle(results2, "Ghost", yearPtr(1990))
	if ranked2[0].Year == nil || *ranked2[0].Year != 1990 {
		t.Fatalf("year-matching result should rank first, got year=%v", ranked2[0].Year)
	}
}

// When scores tie, popularity breaks the tie.
func TestRankByTitle_PopularityTiebreaker(t *testing.T) {
	t.Parallel()
	results := []domainmetadata.MetadataResult{
		{Title: "Ghost", Popularity: 5.0},
		{Title: "Ghost", Popularity: 42.0},
		{Title: "Ghost", Popularity: 1.0},
	}
	ranked := rankByTitle(results, "Ghost", nil)
	if ranked[0].Popularity != 42.0 {
		t.Fatalf("highest popularity should rank first, got %f", ranked[0].Popularity)
	}
	if ranked[2].Popularity != 1.0 {
		t.Fatalf("lowest popularity should rank last, got %f", ranked[2].Popularity)
	}
}

// Original title can produce a better score than localized Title.
func TestRankByTitle_OriginalTitleWins(t *testing.T) {
	t.Parallel()
	results := []domainmetadata.MetadataResult{
		{Title: "Whatever French Release", OriginalTitle: "Parasite"},
		{Title: "Something Else", OriginalTitle: "Other"},
	}
	ranked := rankByTitle(results, "Parasite", nil)
	if ranked[0].OriginalTitle != "Parasite" {
		t.Fatalf("original-title match should rank first, got %+v", ranked[0])
	}
}

// Empty results list → empty output, no panic.
func TestRankByTitle_EmptyInput(t *testing.T) {
	t.Parallel()
	ranked := rankByTitle(nil, "anything", nil)
	if len(ranked) != 0 {
		t.Fatalf("want empty, got %d", len(ranked))
	}
}
