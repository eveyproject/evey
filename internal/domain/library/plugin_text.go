package library

import (
	"context"
	"errors"
	"fmt"

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

// Size is the bytes the text stores.
func (t PluginText) Size() int64 {
	return int64(len(t.Title) + len(t.Overview) + len(t.Tagline))
}

// Bounds of what one metadata plugin stores. UpdateMetadata needs no request
// from the server, so its growth cannot rest on the plugin's restraint, the
// same stance as its key/value store.
const (
	MaxPluginTextLocalesPerItem = 16
	MaxPluginTextRows           = 200_000
	MaxPluginTextBytes          = 128 << 20
)

// ErrPluginTextQuota: the write would take the plugin past a bound above.
var ErrPluginTextQuota = errors.New("plugin metadata text quota reached")

// PluginTextUsage is what a plugin stores apart from the rows a write
// replaces: Rows and Bytes over every item, ItemLocales on the written item.
type PluginTextUsage struct {
	Rows        int64
	Bytes       int64
	ItemLocales int
}

// CheckPluginTextQuota refuses texts for mediaItemID that would take
// pluginID past a bound. locale names the one locale a write replaces, ""
// when it replaces all of the item's locales.
func CheckPluginTextQuota(ctx context.Context, repo PluginTextRepository, pluginID, mediaItemID, locale string, texts []PluginText) error {
	var rows, bytes int64
	for _, t := range texts {
		if t.Empty() {
			continue
		}
		rows++
		bytes += t.Size()
	}
	if rows == 0 {
		return nil // removing never needs room
	}
	u, err := repo.Usage(ctx, pluginID, mediaItemID, locale)
	if err != nil {
		return err
	}
	switch {
	case u.ItemLocales+int(rows) > MaxPluginTextLocalesPerItem:
		return fmt.Errorf("%w: at most %d locales per item", ErrPluginTextQuota, MaxPluginTextLocalesPerItem)
	case u.Rows+rows > MaxPluginTextRows:
		return fmt.Errorf("%w: at most %d texts per plugin", ErrPluginTextQuota, MaxPluginTextRows)
	case u.Bytes+bytes > MaxPluginTextBytes:
		return fmt.Errorf("%w: at most %d MiB of text per plugin", ErrPluginTextQuota, MaxPluginTextBytes>>20)
	}
	return nil
}

// PluginTextRepository keeps the plugins' texts.
type PluginTextRepository interface {
	// ReplaceForItem stores what one plugin gave for one item: its texts for
	// every locale, and none of the locales it gave before and no longer does.
	ReplaceForItem(ctx context.Context, mediaItemID, pluginID string, texts []PluginText) error
	// ReplaceLocale stores what one plugin gives for one item in one locale,
	// its other locales untouched; an empty text removes that locale.
	ReplaceLocale(ctx context.Context, text PluginText) error
	// Usage counts what pluginID stores outside the rows a write on
	// mediaItemID replaces: that one locale, or all of the item's when locale
	// is "".
	Usage(ctx context.Context, pluginID, mediaItemID, locale string) (PluginTextUsage, error)
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
