package metadata

import "testing"

// The exact prod case: a German doc whose TMDB `title` stays German for
// language=fr, but whose French title lives in alternative_titles (FR).
func TestApplyRegionalTitle_GermanDocGetsFrenchRegionalTitle(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               1636842,
		Title:            "Wasserhöhlen - Labyrinthe in der Tiefe", // fr request still returns German
		OriginalTitle:    "Wasserhöhlen - Labyrinthe in der Tiefe",
		OriginalLanguage: "de",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "US", Title: "Underwater Caves"},
			{Region: "FR", Title: "Les grottes immergées - Labyrinthes des profondeurs"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Les grottes immergées - Labyrinthes des profondeurs" {
		t.Fatalf("want FR regional title, got %q", r.Title)
	}
}

// A genuine localized title (TMDB returned something != original) must never be
// overridden by an alternative title.
func TestApplyRegionalTitle_KeepsGenuineLocalizedTitle(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               42,
		Title:            "Un Certain Film", // localized, differs from original
		OriginalTitle:    "Some Movie",
		OriginalLanguage: "en",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "FR", Title: "WRONG should not win"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Un Certain Film" {
		t.Fatalf("genuine localized title clobbered: got %q", r.Title)
	}
}

// Same language as the server → no override even if title == original.
func TestApplyRegionalTitle_SameLanguageNoOverride(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               7,
		Title:            "Amélie",
		OriginalTitle:    "Amélie",
		OriginalLanguage: "fr",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "FR", Title: "should not be used"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Amélie" {
		t.Fatalf("same-language title overridden: got %q", r.Title)
	}
}

// No FR regional title available → leave the original-language title as-is.
func TestApplyRegionalTitle_NoMatchingRegionKeepsOriginal(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               9,
		Title:            "Originaltitel",
		OriginalTitle:    "Originaltitel",
		OriginalLanguage: "de",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "US", Title: "English Title"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Originaltitel" {
		t.Fatalf("want original kept when no FR alt, got %q", r.Title)
	}
}

// TV uses the "results" array; en maps to US/GB regions.
func TestApplyRegionalTitle_TVUsesResultsAndEnRegions(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               100,
		Name:             "Originaltitel",
		OriginalName:     "Originaltitel",
		OriginalLanguage: "de",
		AlternativeTitles: &tmdbAltTitles{Results: []tmdbAltTitle{
			{Region: "GB", Title: "British Title"},
		}},
	}
	r := mapDetailResult(d, "tv", "en")
	if r.Title != "British Title" {
		t.Fatalf("want GB title for en, got %q", r.Title)
	}
}

// The exact Avatar case: FR title == original ("Avatar"), and the only FR
// alternative title is the polluted "Avatar (1)" - it must NOT be applied.
func TestApplyRegionalTitle_SkipsPollutedRegionalTitle(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               19995,
		Title:            "Avatar",
		OriginalTitle:    "Avatar",
		OriginalLanguage: "en",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "FR", Title: "Avatar (1)"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Avatar" {
		t.Fatalf("polluted regional title applied: got %q, want %q", r.Title, "Avatar")
	}
}

// A polluted FR alt must be skipped so a genuinely-different one later in the
// list still wins.
func TestApplyRegionalTitle_SkipsPollutedButKeepsGenuineLater(t *testing.T) {
	d := tmdbDetailResponse{
		ID:               123,
		Title:            "Some Movie",
		OriginalTitle:    "Some Movie",
		OriginalLanguage: "en",
		AlternativeTitles: &tmdbAltTitles{Titles: []tmdbAltTitle{
			{Region: "FR", Title: "Some Movie (1)"},
			{Region: "FR", Title: "Un Autre Titre"},
		}},
	}
	r := mapDetailResult(d, "movie", "fr")
	if r.Title != "Un Autre Titre" {
		t.Fatalf("want genuine FR title after skipping polluted one, got %q", r.Title)
	}
}

func TestStripTrailingParens(t *testing.T) {
	cases := map[string]string{
		"Avatar (1)":         "Avatar",
		"Avatar":             "Avatar",
		"X (1) (uncredited)": "X",
		"  Spaced (2) ":      "Spaced",
		"(Untitled)":         "(Untitled)",
		"Le (Bon) Film":      "Le (Bon) Film", // paren not trailing
	}
	for in, want := range cases {
		if got := stripTrailingParens(in); got != want {
			t.Errorf("stripTrailingParens(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegionsForLanguage(t *testing.T) {
	cases := map[string][]string{
		"fr": {"FR"},
		"de": {"DE"},
		"en": {"US", "GB", "CA", "AU"},
		"pt": {"BR", "PT"},
		"ja": {"JP"},
	}
	for lang, want := range cases {
		got := regionsForLanguage(lang)
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("regionsForLanguage(%q) = %v, want starting %v", lang, got, want)
		}
	}
}
