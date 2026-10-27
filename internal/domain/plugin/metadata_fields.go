package plugin

import (
	"context"
	"fmt"
	"slices"
)

// The fields a metadata plugin (capability metadata.provider) can replace on
// an item. TMDB stays the item's identity; these are only shown in place of
// TMDB's text, and only those the administrator allows.
const (
	MetadataFieldTitle    = "title"
	MetadataFieldOverview = "overview"
	MetadataFieldTagline  = "tagline"
)

// MetadataFields lists every field a metadata plugin can replace, in display
// order.
var MetadataFields = []string{MetadataFieldTitle, MetadataFieldOverview, MetadataFieldTagline}

// DefaultMetadataFields is what a plugin may replace before the administrator
// chooses: never the title, which drives sorting, search and the next match.
var DefaultMetadataFields = []string{MetadataFieldOverview, MetadataFieldTagline}

// CapabilityMetadataProvider is the manifest capability of a plugin that
// answers metadata requests.
const CapabilityMetadataProvider = "metadata.provider"

// NormalizeMetadataFields checks a field choice and returns it deduplicated in
// display order.
func NormalizeMetadataFields(fields []string) ([]string, error) {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if !slices.Contains(MetadataFields, f) {
			return nil, fmt.Errorf("unknown metadata field %q", f)
		}
	}
	for _, f := range MetadataFields {
		if slices.Contains(fields, f) {
			out = append(out, f)
		}
	}
	return out, nil
}

// MetadataSettingsRepository keeps the administrator's field choice per
// plugin.
type MetadataSettingsRepository interface {
	// MetadataFields is the plugin's choice, DefaultMetadataFields when none
	// was made.
	MetadataFields(ctx context.Context, pluginID string) ([]string, error)
	SetMetadataFields(ctx context.Context, pluginID string, fields []string) error
}
