package metadata

import (
	"context"
	"log/slog"
	"testing"

	domain "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prod 2026-06-03: "/media/films/The Megeladon - 1080p.mkv" (misspelled
// "Megalodon") got TMDB 1439930 "The Punisher: One Last Kill". TMDB returns
// nothing for the misspelled query, the cascade retry trims down to "The",
// and every popular "The ..." result shares the stopword "the" with the
// query, so the coherence filter in pickBestMatch lets the top result
// through. Nothing downstream compares the two titles.
func TestSearchAndFetchBest_RejectsUnrelatedTitle(t *testing.T) {
	p := &cascadeTestProvider{
		wantQuery: "The",
		result: []domain.MetadataResult{
			{Title: "The Punisher : One Last Kill", OriginalTitle: "The Punisher: One Last Kill", ProviderIDs: map[string]string{"tmdb": "1439930"}},
			{Title: "The Batman", OriginalTitle: "The Batman", ProviderIDs: map[string]string{"tmdb": "414906"}},
		},
		detailResult: &domain.MetadataResult{
			Title:       "The Punisher : One Last Kill",
			Overview:    "Frank Castle returns.",
			ProviderIDs: map[string]string{"tmdb": "1439930"},
		},
	}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)

	got := e.searchAndFetchBest(context.Background(), p, "The Megeladon", nil, "movie", 0, slog.Default())
	assert.Nil(t, got, "an unrelated provider title must not be accepted")
}

// The guard must not reject legitimate matches. Every pair below comes from
// the owner's production catalogue (1329 matched movies) and is correct.
func TestTitlePlausible_KeepsRealProductionMatches(t *testing.T) {
	for _, tc := range []struct {
		query      string
		candidates []string
	}{
		// French display title, original title carries the filename.
		{"The day after tomorrow", []string{"Le Jour d'après", "The Day After Tomorrow"}},
		{"Hacksaw ridge", []string{"Tu ne tueras point", "Hacksaw Ridge"}},
		{"House of wax", []string{"La Maison de cire", "House of Wax"}},
		{"In the grey", []string{"Les Spécialistes", "In the Grey"}},
		{"Jupiter ascending", []string{"Jupiter : Le Destin de l'univers", "Jupiter Ascending"}},
		// Typo in the filename, same work.
		{"Edge of Tomorow", []string{"Edge of Tomorrow", "Edge of Tomorrow"}},
		{"Motherandroid", []string{"Mother/Android", "Mother/Android"}},
		{"De battre mon coeur sest arrete", []string{"De battre mon cœur s'est arrêté", "De battre mon cœur s'est arrêté"}},
		{"Bienvenue chez les chtis", []string{"Bienvenue chez les Ch'tis", "Bienvenue chez les Ch'tis"}},
		{"Us marshals", []string{"U.S. Marshals", "U.S. Marshals"}},
		// Filename carries a marketing subtitle the provider title drops.
		{"Train to Busan Presents Peninsula", []string{"Peninsula", "반도"}},
		{"Dracula a love tale", []string{"Dracula", "Dracula"}},
		{"Wanted Choisis Ton Destin", []string{"Wanted", ""}},
		// Numbering and cut variants.
		{"Scary movie 1", []string{"Scary Movie", "Scary Movie"}},
		{"Iron Man 1", []string{"Iron Man", "Iron Man"}},
		{"Pirates des caraibes 3", []string{"Pirates des Caraïbes : Jusqu'au bout du monde", "Pirates of the Caribbean: At World's End"}},
		{"Rebel moon chapter two curse of forgiveness directors cut", []string{"Rebel Moon - Partie 2 : L'Entailleuse", "Rebel Moon - Part Two: The Scargiver"}},
		{"L'effet papillon - director's cut", []string{"L'Effet papillon", "The Butterfly Effect"}},
		{"Saw the final chapter", []string{"Saw 3D : Chapitre final", "Saw 3D"}},
		{"Now you see me now you dont", []string{"Insaisissables 3", "Now You See Me: Now You Don't"}},
		{"The fantastic four first steps", []string{"Les 4 Fantastiques : Premiers Pas", "The Fantastic 4: First Steps"}},
		{"Lockout", []string{"Lock Out", "Lockout"}},
		{"Youre next", []string{"You're Next", "You're Next"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			assert.True(t, titlePlausible(tc.query, tc.candidates),
				"legitimate production match rejected: %q vs %v", tc.query, tc.candidates)
		})
	}
}

// Confirmed wrong matches found in the owner's production database.
func TestTitlePlausible_RejectsProductionMismatches(t *testing.T) {
	for _, tc := range []struct {
		query      string
		candidates []string
	}{
		{"The Megeladon", []string{"The Punisher : One Last Kill", "The Punisher: One Last Kill"}},
		{"2001 L'odyssée De L'espace", []string{"The Pretender 2001", "老夫子2001"}},
		{"La French", []string{"La Lycéenne découvre l'amour", "La ragazzina"}},
		{"Tous l'argent du monde", []string{"Tous vedettes !", "Tous vedettes !"}},
		{"Hooc - Peter pan", []string{"Turner & Hooch", "Turner & Hooch"}},
		{"Hush big hit", []string{"The Big Flame", "The Big Flame"}},
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
