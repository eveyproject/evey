package library

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// PluginText is the text a metadata plugin gave for one item in one locale.
// It sits beside the TMDB text instead of replacing it: TMDB stays the item's
// identity, and a refresh from TMDB never touches what a plugin gave. An empty
// field is one the plugin does not cover.
type PluginText struct {
	MediaItemID string
	PluginID    string
	Locale      string
	Title       string
	Overview    string
	Tagline     string
}

// Empty reports whether the text covers no field at all.
func (t PluginText) Empty() bool {
	return t.Title == "" && t.Overview == "" && t.Tagline == ""
}

// PluginTextRepository keeps the plugins' texts.
type PluginTextRepository interface {
	// ReplaceForItem stores what one plugin gave for one item: its texts for
	// every locale, and none of the locales it gave before and no longer does.
	ReplaceForItem(ctx context.Context, mediaItemID, pluginID string, texts []PluginText) error
	// DisplayedForLocale returns, for each item, the texts of one locale as
	// they are to be shown: from enabled plugins only, each field kept only
	// when the administrator lets that plugin replace it, in plugin id order.
	DisplayedForLocale(ctx context.Context, mediaItemIDs []string, locale string) (map[string][]PluginText, error)
}

// OverlayPluginTexts lays plugin texts over a translation, in order: a later
// plugin's field wins over an earlier one's, a plugin's over TMDB's. It
// returns the translation it was given when no text covers a field, a new one
// when the plugins add to it (t may be nil).
func OverlayPluginTexts(t *MediaItemTranslation, mediaItemID, locale string, texts []PluginText) *MediaItemTranslation {
	if len(texts) == 0 {
		return t
	}
	out := MediaItemTranslation{MediaItemID: shared.MediaItemID(mediaItemID), Locale: locale}
	if t != nil {
		out = *t
	}
	for _, pt := range texts {
		if pt.Title != "" {
			out.Title = pt.Title
		}
		if pt.Overview != "" {
			out.Overview = pt.Overview
		}
		if pt.Tagline != "" {
			out.Tagline = pt.Tagline
		}
	}
	return &out
}
