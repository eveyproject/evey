package grpcapi

import (
	"context"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

// mediaItemTranslationSource is the slice of the library query service the
// locale overlay needs.
type mediaItemTranslationSource interface {
	GetMediaItemTranslations(ctx context.Context, mediaItemIDs []string, locale string) (map[string]*library.MediaItemTranslation, error)
}

// mediaItemOverrideSource is the slice of the metadata query service the
// admin-override overlay needs.
type mediaItemOverrideSource interface {
	BatchListOverrides(ctx context.Context, mediaItemIDs []string) (map[string][]*domainmetadata.MetadataOverride, error)
}

// overlayItemsLocale overlays the viewer's locale translations
// (title/overview/tagline) onto items in place. Best-effort: a failed lookup
// keeps the provider text.
func overlayItemsLocale(ctx context.Context, src mediaItemTranslationSource, ids []string, items []*library.MediaItem) {
	locale := eveyi18n.LocaleFromCtx(ctx)
	if src == nil || locale == "" || len(ids) == 0 {
		return
	}
	trans, tErr := src.GetMediaItemTranslations(ctx, ids, locale)
	if tErr != nil {
		slog.WarnContext(ctx, "list media items: localize titles", slog.Any("err", tErr))
		return
	}
	for _, item := range items {
		if t := trans[string(item.ID)]; t != nil {
			if t.Title != "" {
				item.Title = t.Title
			}
			if t.Overview != "" {
				item.Overview = t.Overview
			}
			if t.Tagline != "" {
				item.Tagline = t.Tagline
			}
		}
	}
}

// overlayItemsOverrides applies manual metadata overrides in place. These win
// over the auto-translation too (same precedence as the detail view) so a
// corrected title shows everywhere the item is listed.
func overlayItemsOverrides(ctx context.Context, src mediaItemOverrideSource, ids []string, items []*library.MediaItem) {
	if src == nil || len(ids) == 0 {
		return
	}
	ovMap, oErr := src.BatchListOverrides(ctx, ids)
	if oErr != nil {
		// Best-effort, but say so: lists then show provider titles where an
		// admin corrected them, and a silent return leaves no way to tell that
		// from "nothing was ever corrected".
		slog.WarnContext(ctx, "overlay media item overrides", slog.Int("ids", len(ids)), slog.Any("err", oErr))
		return
	}
	for _, item := range items {
		if ovs := ovMap[string(item.ID)]; len(ovs) > 0 {
			applyOverridesToItem(item, ovs)
		}
	}
}

// overlayItems applies the locale translation first, then the admin overrides
// (which must win), to every non-nil item.
func overlayItems(ctx context.Context, tr mediaItemTranslationSource, ov mediaItemOverrideSource, items []*library.MediaItem) {
	ids := make([]string, 0, len(items))
	live := make([]*library.MediaItem, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		ids = append(ids, string(it.ID))
		live = append(live, it)
	}
	overlayItemsLocale(ctx, tr, ids, live)
	overlayItemsOverrides(ctx, ov, ids, live)
}

// overlayItemMaps overlays the items of several id-keyed maps in one pass per
// source (a resume row resolves items, seasons and series separately).
func overlayItemMaps(ctx context.Context, tr mediaItemTranslationSource, ov mediaItemOverrideSource, maps ...map[string]*library.MediaItem) {
	var all []*library.MediaItem
	for _, m := range maps {
		for _, it := range m {
			all = append(all, it)
		}
	}
	overlayItems(ctx, tr, ov, all)
}

// playbackOverrideSource returns the override source behind the handler's
// optional metadata service, nil when it does not offer one.
func (h *playbackHandler) playbackOverrideSource() mediaItemOverrideSource {
	src, _ := h.metadataSvc.(mediaItemOverrideSource)
	return src
}

// overlayMaps localizes and applies admin overrides to resolved item maps.
func (h *playbackHandler) overlayMaps(ctx context.Context, maps ...map[string]*library.MediaItem) {
	if h.librarySvc == nil {
		return
	}
	overlayItemMaps(ctx, h.librarySvc, h.playbackOverrideSource(), maps...)
}
