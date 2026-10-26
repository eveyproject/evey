package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type fakePluginSource struct {
	requests []domain.PluginTextRequest
	answers  []domain.PluginTextAnswer
}

func (f *fakePluginSource) LookupTexts(_ context.Context, req domain.PluginTextRequest) []domain.PluginTextAnswer {
	f.requests = append(f.requests, req)
	return f.answers
}

type replaceCall struct {
	item, plugin string
	texts        []domainlib.PluginText
}

type recordingPluginTexts struct {
	calls []replaceCall
	err   error
}

func (r *recordingPluginTexts) ReplaceForItem(_ context.Context, item, plugin string, texts []domainlib.PluginText) error {
	r.calls = append(r.calls, replaceCall{item, plugin, texts})
	return r.err
}

func (r *recordingPluginTexts) DisplayedForLocale(context.Context, []string, string) (map[string][]domainlib.PluginText, error) {
	return nil, nil
}

// langProvider is a primary provider whose text is in a set language.
type langProvider struct {
	fakeProvider
	lang string
}

func (p *langProvider) Language() string { return p.lang }

// Enriching a movie asks the plugins about it with what TMDB just gave and
// stores each answer under its plugin, an empty answer included (the plugin
// knows nothing of it now).
func TestEnrichItem_AsksThePluginsAndStoresTheirTexts(t *testing.T) {
	ctx := context.Background()
	// Wrapped as in production: the language is found through Unwrap.
	primary := NewCachedProvider(&langProvider{fakeProvider: fakeProvider{name: "tmdb"}, lang: "fr-FR"}, time.Minute)
	e, _, _, _, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	e.chain = NewProviderChain(primary)
	source := &fakePluginSource{answers: []domain.PluginTextAnswer{
		{PluginID: "allocine", Texts: []domain.PluginLocaleText{{Locale: "fr", Overview: "Un résumé."}}},
		{PluginID: "other"},
	}}
	store := &recordingPluginTexts{}
	EnricherWithPluginTexts(source, store)(e)
	e.translationLocales = []string{"en", "fr"}

	year := 1995
	item := &domainlib.MediaItem{ID: "m1", Type: domainlib.MovieItem}
	require.NoError(t, miRepo.Create(ctx, item))
	require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{
		Title: "Heat", OriginalTitle: "Heat", Year: &year, ProviderIDs: map[string]string{"tmdb": "949"},
	}))

	require.Len(t, source.requests, 1)
	assert.Equal(t, domain.PluginTextRequest{
		MediaItemID: "m1", Type: "movie", Title: "Heat", OriginalTitle: "Heat", Year: 1995,
		ProviderIDs: item.Catalog.ProviderIDs, Language: "fr", Locales: []string{"en", "fr"},
	}, source.requests[0])
	assert.Equal(t, []replaceCall{
		{"m1", "allocine", []domainlib.PluginText{{MediaItemID: "m1", PluginID: "allocine", Locale: "fr", Overview: "Un résumé."}}},
		{"m1", "other", []domainlib.PluginText{}},
	}, store.calls)
}

// Seasons and episodes are left to TMDB, and a store that fails does not fail
// the enrichment that already committed.
func TestEnrichItem_PluginTextsOnlyForMoviesAndSeries(t *testing.T) {
	ctx := context.Background()
	e, _, _, _, miRepo, _ := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	source := &fakePluginSource{answers: []domain.PluginTextAnswer{{PluginID: "p", Texts: []domain.PluginLocaleText{{Locale: "en", Overview: "x"}}}}}
	store := &recordingPluginTexts{err: errors.New("db down")}
	EnricherWithPluginTexts(source, store)(e)

	for _, typ := range []domainlib.MediaItemType{domainlib.Season, domainlib.Episode, domainlib.Series} {
		item := &domainlib.MediaItem{ID: shared.MediaItemID("i-" + string(typ)), Type: typ}
		require.NoError(t, miRepo.Create(ctx, item))
		require.NoError(t, e.EnrichItem(ctx, item, &domain.MetadataResult{Title: "T"}))
	}
	require.Len(t, source.requests, 1)
	assert.Equal(t, "series", source.requests[0].Type)
	assert.Equal(t, "en", source.requests[0].Language)
}
