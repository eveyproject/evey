package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/plugin"
)

// What a plugin gave is shown through the administrator's field choice (the
// default leaves the title out), never from a disabled plugin, and a new
// answer replaces the old one locale by locale.
func TestPluginTextRepo_DisplayedThroughTheFieldChoice(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	lib := newTestLibrary("ptlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	items := postgres.NewMediaItemRepository(db)
	for _, id := range []string{"pt1", "pt2"} {
		require.NoError(t, items.Create(ctx, newTestMediaItem(id, lib.ID)))
	}
	plugins := postgres.NewPluginRepository(db)
	for _, id := range []string{"pa", "pb", "pc"} {
		require.NoError(t, plugins.Create(ctx, newTestPlugin(id)))
	}
	off, err := plugins.GetByID(ctx, "pc")
	require.NoError(t, err)
	off.Enabled = false
	require.NoError(t, plugins.Update(ctx, off))

	repo := postgres.NewPluginTextRepository(db)
	settings := postgres.NewPluginMetadataSettingsRepository(db)
	full := func(item, pluginID, locale string) library.PluginText {
		return library.PluginText{MediaItemID: item, PluginID: pluginID, Locale: locale, Title: pluginID + " title", Overview: pluginID + " overview", Tagline: pluginID + " tagline"}
	}
	require.NoError(t, repo.ReplaceForItem(ctx, "pt1", "pa", []library.PluginText{full("pt1", "pa", "fr"), full("pt1", "pa", "de")}))
	require.NoError(t, repo.ReplaceForItem(ctx, "pt1", "pb", []library.PluginText{full("pt1", "pb", "fr")}))
	require.NoError(t, repo.ReplaceForItem(ctx, "pt1", "pc", []library.PluginText{full("pt1", "pc", "fr")}))
	require.NoError(t, settings.SetMetadataFields(ctx, "pb", []string{plugin.MetadataFieldTitle}))

	got, err := repo.DisplayedForLocale(ctx, []string{"pt1", "pt2"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, map[string][]library.PluginText{"pt1": {
		{MediaItemID: "pt1", PluginID: "pa", Locale: "fr", Overview: "pa overview", Tagline: "pa tagline"},
		{MediaItemID: "pt1", PluginID: "pb", Locale: "fr", Title: "pb title"},
	}}, got)

	// A new answer drops the locales the plugin no longer gives.
	require.NoError(t, repo.ReplaceForItem(ctx, "pt1", "pa", []library.PluginText{{MediaItemID: "pt1", PluginID: "pa", Locale: "fr", Overview: "new"}}))
	de, err := repo.DisplayedForLocale(ctx, []string{"pt1"}, "de")
	require.NoError(t, err)
	assert.Empty(t, de)
	fr, err := repo.DisplayedForLocale(ctx, []string{"pt1"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, "new", fr["pt1"][0].Overview)

	// No field allowed: nothing shows.
	require.NoError(t, settings.SetMetadataFields(ctx, "pa", []string{}))
	fr, err = repo.DisplayedForLocale(ctx, []string{"pt1"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, []library.PluginText{{MediaItemID: "pt1", PluginID: "pb", Locale: "fr", Title: "pb title"}}, fr["pt1"])

	// Uninstalling a plugin takes its texts and its choice away.
	require.NoError(t, plugins.Delete(ctx, "pb"))
	fr, err = repo.DisplayedForLocale(ctx, []string{"pt1"}, "fr")
	require.NoError(t, err)
	assert.Empty(t, fr)
	var left int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM plugin_metadata_settings WHERE plugin_id = 'pb'`).Scan(&left))
	assert.Zero(t, left)

	// One locale replaced or removed, the others kept.
	require.NoError(t, repo.ReplaceForItem(ctx, "pt2", "pa", []library.PluginText{full("pt2", "pa", "fr"), full("pt2", "pa", "de")}))
	require.NoError(t, settings.SetMetadataFields(ctx, "pa", []string{plugin.MetadataFieldOverview}))
	require.NoError(t, repo.ReplaceLocale(ctx, library.PluginText{MediaItemID: "pt2", PluginID: "pa", Locale: "fr", Overview: "corrigé"}))
	require.NoError(t, repo.ReplaceLocale(ctx, library.PluginText{MediaItemID: "pt2", PluginID: "pa", Locale: "de"}))
	fr, err = repo.DisplayedForLocale(ctx, []string{"pt2"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, "corrigé", fr["pt2"][0].Overview)
	de, err = repo.DisplayedForLocale(ctx, []string{"pt2"}, "de")
	require.NoError(t, err)
	assert.Empty(t, de)
}

func TestPluginMetadataSettingsRepo_DefaultsThenChoice(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, postgres.NewPluginRepository(db).Create(ctx, newTestPlugin("ps")))
	settings := postgres.NewPluginMetadataSettingsRepository(db)

	fields, err := settings.MetadataFields(ctx, "ps")
	require.NoError(t, err)
	assert.Equal(t, plugin.DefaultMetadataFields, fields)

	require.NoError(t, settings.SetMetadataFields(ctx, "ps", nil))
	fields, err = settings.MetadataFields(ctx, "ps")
	require.NoError(t, err)
	assert.Equal(t, []string{}, fields)

	require.NoError(t, settings.SetMetadataFields(ctx, "ps", []string{"title", "overview"}))
	fields, err = settings.MetadataFields(ctx, "ps")
	require.NoError(t, err)
	assert.Equal(t, []string{"title", "overview"}, fields)
}
