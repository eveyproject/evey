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

func TestMergeResults_ArtworkAppended(t *testing.T) {
	primary := &domain.MetadataResult{
		Artwork: []domain.ArtworkResult{{Type: "poster", Provider: "tmdb", URL: "http://tmdb.com/1.jpg"}},
	}
	supplement := &domain.MetadataResult{
		Artwork: []domain.ArtworkResult{{Type: "poster", Provider: "omdb", URL: "http://omdb.com/1.jpg"}},
	}

	result := MergeResults(primary, supplement)
	assert.Len(t, result.Artwork, 2)
	assert.Equal(t, "tmdb", result.Artwork[0].Provider)
	assert.Equal(t, "omdb", result.Artwork[1].Provider)
}

func TestMergeResults_NilSupplement(t *testing.T) {
	primary := &domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": "1"},
		Overview:    "test",
	}

	result := MergeResults(primary, nil, nil)
	assert.Equal(t, "test", result.Overview)
}
