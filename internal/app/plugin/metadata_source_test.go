package plugin_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/metadata"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
)

func metadataPlugin(id string, enabled bool, status plugindomain.PluginStatus, caps ...string) *plugindomain.Plugin {
	return &plugindomain.Plugin{ID: id, Name: id, Enabled: enabled, Status: status, Capabilities: caps}
}

// Only enabled, running plugins with the metadata.provider capability are
// asked, in id order, with the request as documented; a plugin that declines
// answers with no text (it clears what it gave), one that fails is left out
// (it keeps what it gave).
func TestMetadataSource_AsksEachMetadataPlugin(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	ctx := context.Background()
	for _, p := range []*plugindomain.Plugin{
		metadataPlugin("b-allocine", true, plugindomain.Running, plugindomain.CapabilityMetadataProvider),
		metadataPlugin("a-declines", true, plugindomain.Running, plugindomain.CapabilityMetadataProvider),
		metadataPlugin("c-fails", true, plugindomain.Running, plugindomain.CapabilityMetadataProvider),
		metadataPlugin("d-disabled", false, plugindomain.Running, plugindomain.CapabilityMetadataProvider),
		metadataPlugin("e-stopped", true, plugindomain.Stopped, plugindomain.CapabilityMetadataProvider),
		metadataPlugin("f-weather", true, plugindomain.Running, "media.read"),
	} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	runner := plgfakes.NewFakePluginRunner()
	var asked []string
	var payload map[string]any
	runner.AskFunc = func(_ context.Context, pluginID, eventType string, p map[string]any, timeout time.Duration) (map[string]any, bool, error) {
		asked = append(asked, pluginID)
		if eventType != appplugin.MetadataRequestEvent || timeout <= 0 {
			t.Errorf("asked %q with timeout %v", eventType, timeout)
		}
		switch pluginID {
		case "a-declines":
			return nil, false, nil
		case "c-fails":
			return nil, false, errors.New("deadline exceeded")
		}
		payload = p
		return map[string]any{"overview": "Un résumé.", "tagline": ""}, true, nil
	}
	src := appplugin.NewMetadataSource(repo, runner, nil)
	got := src.LookupTexts(ctx, metadata.PluginTextRequest{
		MediaItemID: "m1", Type: "movie", Title: "Heat", Year: 1995,
		ProviderIDs: map[string]string{"tmdb": "949", "imdb": ""},
		Language:    "fr", Locales: []string{"en", "fr"},
	})
	if want := []string{"a-declines", "b-allocine", "c-fails"}; !reflect.DeepEqual(asked, want) {
		t.Fatalf("asked %v, want %v", asked, want)
	}
	want := []metadata.PluginTextAnswer{
		{PluginID: "a-declines"},
		{PluginID: "b-allocine", Texts: []metadata.PluginLocaleText{{Locale: "fr", Overview: "Un résumé."}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("answers %+v, want %+v", got, want)
	}
	wantPayload := map[string]any{
		"media_item_id": "m1", "type": "movie", "title": "Heat", "original_title": "",
		"provider_ids": map[string]any{"tmdb": "949"}, "language": "fr",
		"locales": []any{"en", "fr"}, "year": float64(1995),
	}
	if !reflect.DeepEqual(payload, wantPayload) {
		t.Fatalf("payload %v, want %v", payload, wantPayload)
	}
}

// The answer names its locale, adds others under translations, and is read
// leniently: locale tags shortened to their language, fields of the wrong type
// or oversized ignored, a locale with no text dropped.
func TestMetadataSource_ReadsTheAnswer(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	ctx := context.Background()
	if err := repo.Create(ctx, metadataPlugin("p", true, plugindomain.Running, plugindomain.CapabilityMetadataProvider)); err != nil {
		t.Fatal(err)
	}
	runner := plgfakes.NewFakePluginRunner()
	runner.AskFunc = func(context.Context, string, string, map[string]any, time.Duration) (map[string]any, bool, error) {
		return map[string]any{
			"locale":   "fr-FR",
			"title":    "  Heat  ",
			"overview": 42.0,
			"translations": map[string]any{
				"de_DE": map[string]any{"overview": "Eine Zusammenfassung."},
				"es":    map[string]any{"tagline": string(make([]byte, 9000))},
				"x":     map[string]any{"title": "bad locale"},
				"it":    "not an object",
			},
		}, true, nil
	}
	got := appplugin.NewMetadataSource(repo, runner, nil).LookupTexts(ctx, metadata.PluginTextRequest{MediaItemID: "m1", Language: "en"})
	want := []metadata.PluginTextAnswer{{PluginID: "p", Texts: []metadata.PluginLocaleText{
		{Locale: "de", Overview: "Eine Zusammenfassung."},
		{Locale: "fr", Title: "Heat"},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("answers %+v, want %+v", got, want)
	}
}
