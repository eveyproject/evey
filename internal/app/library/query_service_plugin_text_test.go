package library_test

import (
	"context"
	"errors"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePluginTexts answers DisplayedForLocale from a fixed map, as the
// repository would after its field filter.
type fakePluginTexts struct {
	texts map[string][]library.PluginText
	err   error
}

func (f *fakePluginTexts) ReplaceForItem(context.Context, string, string, []library.PluginText) error {
	return nil
}

func (f *fakePluginTexts) ReplaceLocale(context.Context, library.PluginText) error { return nil }

func (f *fakePluginTexts) DisplayedForLocale(_ context.Context, ids []string, locale string) (map[string][]library.PluginText, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string][]library.PluginText{}
	for _, id := range ids {
		for _, t := range f.texts[id] {
			if t.Locale == locale {
				out[id] = append(out[id], t)
			}
		}
	}
	return out, nil
}

func newPluginTextQuerySvc(tr *fakeMediaItemTranslationRepo, texts *fakePluginTexts) *applib.QueryService {
	return applib.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applib.WithTranslations(tr, newFakePersonTranslationRepo()),
		applib.WithPluginTexts(texts),
	)
}

// A plugin's field wins over TMDB's translation, a later plugin's over an
// earlier one's, and a field no plugin covers keeps TMDB's text - on the one
// item read and on the batch read alike.
func TestQueryService_PluginTextsOverlayTheTranslation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newFakeMediaItemTranslationRepo()
	require.NoError(t, tr.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: "m1", Locale: "fr", Title: "Heat", Overview: "TMDB", Tagline: "TMDB tag"}))
	texts := &fakePluginTexts{texts: map[string][]library.PluginText{
		"m1": {
			{MediaItemID: "m1", PluginID: "a", Locale: "fr", Overview: "Plugin A", Tagline: "A tag"},
			{MediaItemID: "m1", PluginID: "b", Locale: "fr", Overview: "Plugin B"},
			{MediaItemID: "m1", PluginID: "b", Locale: "de", Overview: "Plugin B de"},
		},
		// No TMDB translation at all: the plugin's text alone.
		"m2": {{MediaItemID: "m2", PluginID: "a", Locale: "fr", Overview: "Seul"}},
	}}
	svc := newPluginTextQuerySvc(tr, texts)

	one, err := svc.GetMediaItemTranslation(ctx, "m1", "fr")
	require.NoError(t, err)
	assert.Equal(t, library.MediaItemTranslation{MediaItemID: "m1", Locale: "fr", Title: "Heat", Overview: "Plugin B", Tagline: "A tag"}, *one)

	only, err := svc.GetMediaItemTranslation(ctx, "m2", "fr")
	require.NoError(t, err)
	assert.Equal(t, library.MediaItemTranslation{MediaItemID: "m2", Locale: "fr", Overview: "Seul"}, *only)

	batch, err := svc.GetMediaItemTranslations(ctx, []string{"m1", "m2", "m3"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, "Plugin B", batch["m1"].Overview)
	assert.Equal(t, "Seul", batch["m2"].Overview)
	assert.NotContains(t, batch, "m3")

	// The stored TMDB row is not touched by the overlay.
	stored, err := tr.GetByMediaItemAndLocale(ctx, "m1", "fr")
	require.NoError(t, err)
	assert.Equal(t, "TMDB", stored.Overview)
}

// An unreadable overlay leaves the TMDB text rather than failing the read.
func TestQueryService_PluginTextsUnreadableKeepTMDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tr := newFakeMediaItemTranslationRepo()
	require.NoError(t, tr.Upsert(ctx, &library.MediaItemTranslation{MediaItemID: "m1", Locale: "fr", Overview: "TMDB"}))
	svc := newPluginTextQuerySvc(tr, &fakePluginTexts{err: errors.New("db down")})

	one, err := svc.GetMediaItemTranslation(ctx, "m1", "fr")
	require.NoError(t, err)
	assert.Equal(t, "TMDB", one.Overview)
	batch, err := svc.GetMediaItemTranslations(ctx, []string{"m1"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, "TMDB", batch["m1"].Overview)
}
