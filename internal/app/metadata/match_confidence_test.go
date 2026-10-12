package metadata

import (
	"context"
	"log/slog"
	"testing"

	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shape of a production incident (2026-06-03): a misspelled filename makes the
// provider return nothing, the cascade retry trims down to "The", and every
// popular "The ..." result shares the stopword "the" with the query, so the
// coherence filter in pickBestMatch lets the top result through. Nothing
// downstream compares the two titles.
func TestSearchAndFetchBest_RejectsUnrelatedTitle(t *testing.T) {
	p := &cascadeTestProvider{
		wantQuery: "The",
		result: []domain.MetadataResult{
			{Title: "The Avenger : One Last Night", OriginalTitle: "The Avenger: One Last Night", ProviderIDs: map[string]string{"tmdb": "1000001"}},
			{Title: "The Knight", OriginalTitle: "The Knight", ProviderIDs: map[string]string{"tmdb": "1000002"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "The Avenger : One Last Night",
			Overview:    "Some overview.",
			ProviderIDs: map[string]string{"tmdb": "1000001"},
		},
	}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)

	got := e.searchAndFetchBest(context.Background(), p, "The Sandcastel", nil, "movie", 0, slog.Default())
	assert.Nil(t, got, "an unrelated provider title must not be accepted")
}

// The guard must not reject legitimate matches. Each pair is synthetic but
// keeps the shape of a real pair from the production replay (1329 matched
// movies): same rule deciding it, same word counts, same typo distance.
func TestTitlePlausible_KeepsRealProductionMatches(t *testing.T) {
	for _, tc := range []struct {
		query      string
		candidates []string
	}{
		// French display title, original title carries the filename.
		{"The night before winter", []string{"La Veille de l'hiver", "The Night Before Winter"}},
		{"Copper valley", []string{"Tu ne passeras pas", "Copper Valley"}},
		{"House of glass", []string{"La Maison de verre", "House of Glass"}},
		{"In the blue", []string{"Les Plongeurs", "In the Blue"}},
		{"Saturn descending", []string{"Saturne : La Chute de l'anneau", "Saturn Descending"}},
		// Typo in the filename, same work.
		{"Harbor of Silense", []string{"Harbor of Silence", "Harbor of Silence"}},
		{"Fathermachine", []string{"Father/Machine", "Father/Machine"}},
		{"Mon coeur sest tu en ete", []string{"Mon cœur s'est tu en été", "Mon cœur s'est tu en été"}},
		{"Bienvenue chez les ptiots", []string{"Bienvenue chez les P'tiots", "Bienvenue chez les P'tiots"}},
		{"Us rangers", []string{"U.S. Rangers", "U.S. Rangers"}},
		// Filename carries a marketing subtitle the provider title drops.
		{"Night Train Presents Archipelago", []string{"Archipelago", "섬"}},
		{"Nosfera a love tale", []string{"Nosfera", "Nosfera"}},
		{"Hunted Choisis Ton Chemin", []string{"Hunted", ""}},
		// Numbering and cut variants.
		{"Funny movie 1", []string{"Funny Movie", "Funny Movie"}},
		{"Glass Man 1", []string{"Glass Man", "Glass Man"}},
		{"Corsaires des antilles 3", []string{"Corsaires des Antilles : Jusqu'au bout de la mer", "Corsairs of the Antilles: At Sea's End"}},
		// The tightest legitimate pair: 3 of 8 content words (0.375).
		{"Iron tide chapter two song of reckoning directors cut", []string{"Iron Tide - Partie 2 : La Faucheuse", "Iron Tide - Part Two: The Tidecaller"}},
		{"L'effet colibri - director's cut", []string{"L'Effet colibri", "The Hummingbird Effect"}},
		{"Hex the final chapter", []string{"Hex 3D : Chapitre final", "Hex 3D"}},
		{"Now you hear me now you dont", []string{"Inaudibles 3", "Now You Hear Me: Now You Don't"}},
		{"The marvelous five first steps", []string{"Les 5 Merveilleux : Premiers Pas", "The Marvelous 5: First Steps"}},
		{"Shutout", []string{"Shut Out", "Shutout"}},
		{"Youre last", []string{"You're Last", "You're Last"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			assert.True(t, titlePlausible(tc.query, tc.candidates),
				"legitimate production match rejected: %q vs %v", tc.query, tc.candidates)
		})
	}
}

// Wrong matches with the shape of those confirmed in the production database.
func TestTitlePlausible_RejectsProductionMismatches(t *testing.T) {
	for _, tc := range []struct {
		query      string
		candidates []string
	}{
		{"The Sandcastel", []string{"The Avenger : One Last Night", "The Avenger: One Last Night"}},
		{"3001 Le voyage des étoiles", []string{"The Stranger 3001", "老人3001"}},
		{"La Grande", []string{"La Voisine découvre la mer", "La bambina"}},
		{"Tous les chemins du monde", []string{"Tous champions !", "Tous champions !"}},
		{"Crok - Robin wood", []string{"Walker & Crock", "Walker & Crock"}},
		// The loosest wrong match: 1 of 3 content words (0.333).
		{"Mute red shot", []string{"The Red Spark", "The Red Spark"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			assert.False(t, titlePlausible(tc.query, tc.candidates),
				"wrong production match accepted: %q vs %v", tc.query, tc.candidates)
		})
	}
}

func TestTitlePlausible_EmptyInputsAreNotPlausible(t *testing.T) {
	require.False(t, titlePlausible("", []string{"Anything"}))
	require.False(t, titlePlausible("Anything", nil))
	require.False(t, titlePlausible("Anything", []string{""}))
}
