package metadata

import "context"

// PluginTextRequest is what a metadata plugin is told about an item: enough
// to find it in its own catalogue (TMDB and IMDb ids first, the title and year
// otherwise) and the locales the server shows.
type PluginTextRequest struct {
	MediaItemID   string
	Type          string // "movie" or "series"
	Title         string
	OriginalTitle string
	Year          int // 0 when unknown
	ProviderIDs   map[string]string
	// Language is the locale of the TMDB text, the one a plugin's answer is
	// taken to be in when it names none.
	Language string
	Locales  []string
}

// PluginLocaleText is a plugin's text for one locale; an empty field is one
// it does not cover.
type PluginLocaleText struct {
	Locale   string
	Title    string
	Overview string
	Tagline  string
}

// PluginTextAnswer is one plugin's answer. Texts is empty when the plugin
// knows nothing of the item, which clears what it gave before.
type PluginTextAnswer struct {
	PluginID string
	Texts    []PluginLocaleText
}

// PluginMetadataSource asks the metadata plugins about an item. Only the
// plugins that answered are in the result: one that failed or timed out keeps
// what it gave before.
type PluginMetadataSource interface {
	LookupTexts(ctx context.Context, req PluginTextRequest) []PluginTextAnswer
}
