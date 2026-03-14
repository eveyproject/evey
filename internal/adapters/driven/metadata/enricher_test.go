package metadata

import (
	"testing"

	domain "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
)

func TestMergeResults_ProviderIDs(t *testing.T) {
	primary := &domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": "550"},
	}
	supplement := &domain.MetadataResult{
		ProviderIDs: map[string]string{"imdb": "tt0137523", "tmdb": "999"},
	}

	result := MergeResults(primary, supplement)
	assert.Equal(t, "550", result.ProviderIDs["tmdb"])
	assert.Equal(t, "tt0137523", result.ProviderIDs["imdb"])
}

func TestMergeResults_Ratings(t *testing.T) {
	primary := &domain.MetadataResult{
		Ratings: map[string]float64{"tmdb": 8.4},
	}
	supplement := &domain.MetadataResult{
		Ratings: map[string]float64{"imdb": 8.8, "rt": 87, "metacritic": 74, "tmdb": 5.0},
	}

	result := MergeResults(primary, supplement)
	assert.InDelta(t, 8.4, result.Ratings["tmdb"], 0.01)
	assert.InDelta(t, 8.8, result.Ratings["imdb"], 0.01)
	assert.InDelta(t, 87.0, result.Ratings["rt"], 0.01)
	assert.InDelta(t, 74.0, result.Ratings["metacritic"], 0.01)
}

func TestMergeResults_FallbackOverview(t *testing.T) {
	primary := &domain.MetadataResult{Overview: ""}
	supplement := &domain.MetadataResult{Overview: "From supplement"}

	result := MergeResults(primary, supplement)
	assert.Equal(t, "From supplement", result.Overview)
}

func TestMergeResults_PrimaryOverviewKept(t *testing.T) {
	primary := &domain.MetadataResult{Overview: "Primary overview"}
	supplement := &domain.MetadataResult{Overview: "Supplement overview"}

	result := MergeResults(primary, supplement)
	assert.Equal(t, "Primary overview", result.Overview)
}

func TestMergeResults_FallbackPeople(t *testing.T) {
	primary := &domain.MetadataResult{}
	supplement := &domain.MetadataResult{
		People: []domain.PersonResult{{Name: "Actor", Role: "Actor"}},
	}

	result := MergeResults(primary, supplement)
	assert.Len(t, result.People, 1)
}

func TestMergeResults_PrimaryPeopleKept(t *testing.T) {
	primary := &domain.MetadataResult{
		People: []domain.PersonResult{{Name: "Primary Actor", Role: "Actor"}},
	}
	supplement := &domain.MetadataResult{
		People: []domain.PersonResult{{Name: "Supp Actor", Role: "Actor"}},
	}

	result := MergeResults(primary, supplement)
	assert.Len(t, result.People, 1)
	assert.Equal(t, "Primary Actor", result.People[0].Name)
}

func TestMergeResults_ArtworkDeduplicatedByType(t *testing.T) {
	primary := &domain.MetadataResult{
		Artwork: []domain.ArtworkResult{{Type: "poster", Provider: "tmdb", URL: "http://tmdb.com/1.jpg"}},
	}
	// same type as primary — should be skipped
	dupSupplement := &domain.MetadataResult{
		Artwork: []domain.ArtworkResult{{Type: "poster", Provider: "omdb", URL: "http://omdb.com/1.jpg"}},
	}
	// different type — should be added
	newSupplement := &domain.MetadataResult{
		Artwork: []domain.ArtworkResult{{Type: "backdrop", Provider: "tvmaze", URL: "http://tvmaze.com/1.jpg"}},
	}

	result := MergeResults(primary, dupSupplement, newSupplement)
	assert.Len(t, result.Artwork, 2)
	assert.Equal(t, "tmdb", result.Artwork[0].Provider)
	assert.Equal(t, "tvmaze", result.Artwork[1].Provider)
}

func TestPickBestMatch_TitleSimilarity(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "The Rookie : Le Flic de Los Angeles", OriginalTitle: "The Rookie"},
		{Title: "Le flic de Shanghai", OriginalTitle: "Martial Law"},
	}
	best := pickBestMatch(results, "Le flic de shanghai", nil)
	assert.Equal(t, "Le flic de Shanghai", best.Title)
}

func TestPickBestMatch_ExactTitle(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "Breaking Bad"},
		{Title: "Bad Breaking"},
	}
	best := pickBestMatch(results, "Breaking Bad", nil)
	assert.Equal(t, "Breaking Bad", best.Title)
}

func TestPickBestMatch_YearBonus(t *testing.T) {
	y2020 := 2020
	y2005 := 2005
	results := []domain.MetadataResult{
		{Title: "The Movie", Year: &y2020},
		{Title: "The Movie", Year: &y2005},
	}
	best := pickBestMatch(results, "The Movie", &y2005)
	assert.Equal(t, &y2005, best.Year)
}

func TestPickBestMatch_RebootPenalty(t *testing.T) {
	y1990 := 1990
	y2017 := 2017
	y2025 := 2025
	// "Robin Hood" reboot: same title, different year → year-close match wins
	results := []domain.MetadataResult{
		{Title: "Robin Hood", Year: &y1990},
		{Title: "Robin Hood", Year: &y2017},
	}
	best := pickBestMatch(results, "Robin Hood", &y2017)
	assert.Equal(t, &y2017, best.Year)

	// Old version gets penalty when query year is far away
	best = pickBestMatch(results, "Robin Hood", &y2025)
	assert.Equal(t, &y2017, best.Year) // 2017 closer to 2025 than 1990

	// Without year hint, no penalty — first high-score wins
	best = pickBestMatch(results, "Robin Hood", nil)
	assert.Equal(t, "Robin Hood", best.Title)
}

func TestPickBestMatch_CloseYearBonus(t *testing.T) {
	y2024 := 2024
	y2025 := 2025
	y2010 := 2010
	results := []domain.MetadataResult{
		{Title: "Show", Year: &y2010},
		{Title: "Show", Year: &y2024},
	}
	// ±2 year should get small bonus, not full bonus
	best := pickBestMatch(results, "Show", &y2025)
	assert.Equal(t, &y2024, best.Year) // 2024 within ±2 of 2025
}

func TestPickBestMatch_OriginalTitleUsed(t *testing.T) {
	results := []domain.MetadataResult{
		{Title: "La Loi de la Force", OriginalTitle: "Martial Law"},
		{Title: "La Force de la Loi", OriginalTitle: "Other Show"},
	}
	best := pickBestMatch(results, "Martial Law", nil)
	assert.Equal(t, "Martial Law", best.OriginalTitle)
}

func TestTitleSimilarity(t *testing.T) {
	assert.InDelta(t, 1.0, titleSimilarity("breaking bad", "Breaking Bad"), 0.01)
	assert.Greater(t,
		titleSimilarity("le flic de shanghai", "Le flic de Shanghai"),
		titleSimilarity("le flic de shanghai", "The Rookie : Le Flic de Los Angeles"),
	)
	assert.InDelta(t, 0.0, titleSimilarity("test", ""), 0.001)
}

func TestMergeResults_NilSupplement(t *testing.T) {
	primary := &domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": "1"},
		Overview:    "test",
	}

	result := MergeResults(primary, nil, nil)
	assert.Equal(t, "test", result.Overview)
}
