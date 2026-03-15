package metadata

import (
	"testing"

	domain "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
)

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

