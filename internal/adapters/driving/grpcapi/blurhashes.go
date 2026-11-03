package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/metadata"
)

// artworkBatchGetter is the slice of the metadata query service the poster
// stamping needs: one batched artwork fetch for a whole page of cards.
type artworkBatchGetter interface {
	BatchGetArtwork(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error)
}

// fillPrimaryBlurhashes attaches the most-preferred poster record carrying a
// blurhash to converted media items that ship without artwork (resume /
// next-up build bare protos via mediaItemToProto). All three clients already
// read MediaItem.artwork[PRIMARY].blurhash for their card placeholder - the
// list endpoints attach artwork batch-side (mediaItemsToProto) and this
// mirrors that for playback rows, one batched fetch per response.
// Best-effort: a failed fetch leaves artwork empty instead of failing the
// response.
func fillPrimaryBlurhashes(ctx context.Context, svc artworkBatchGetter, items []*eveyv1.MediaItem) {
	if svc == nil || len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, it := range items {
		if it == nil || it.Id == "" || len(it.Artwork) > 0 {
			continue
		}
		if _, ok := seen[it.Id]; !ok {
			seen[it.Id] = struct{}{}
			ids = append(ids, it.Id)
		}
	}
	if len(ids) == 0 {
		return
	}
	byItem, err := svc.BatchGetArtwork(ctx, ids)
	if err != nil {
		slog.DebugContext(ctx, "fill blurhashes: batch artwork fetch failed", slog.Any("err", err))
		return
	}
	for _, it := range items {
		if it == nil || len(it.Artwork) > 0 {
			continue
		}
		if a := primaryPosterWithBlurhash(byItem[it.Id]); a != nil {
			it.Artwork = append(it.Artwork, artworkRecordToProto(a))
		}
	}
}

// primaryPosterWithBlurhash is the first poster record carrying a blurhash.
// Records are ordered by sort_order, so it is the most-preferred one - the
// same rule as the feed querier.
func primaryPosterWithBlurhash(records []*metadata.ArtworkRecord) *metadata.ArtworkRecord {
	for _, a := range records {
		if a != nil && a.Type == "poster" && a.Blurhash != nil && *a.Blurhash != "" {
			return a
		}
	}
	return nil
}

// fillPopularBlurhashes stamps the primary poster placeholder on the popular
// rail's items, one batched fetch. Best-effort like fillPrimaryBlurhashes.
func fillPopularBlurhashes(ctx context.Context, svc artworkBatchGetter, items []*eveyv1.PopularMediaItem) {
	if svc == nil || len(items) == 0 {
		return
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.MediaItemId
	}
	byItem, err := svc.BatchGetArtwork(ctx, ids)
	if err != nil {
		slog.DebugContext(ctx, "popular blurhashes: batch artwork fetch failed", slog.Any("err", err))
		return
	}
	for _, it := range items {
		if a := primaryPosterWithBlurhash(byItem[it.MediaItemId]); a != nil {
			it.PrimaryBlurhash = *a.Blurhash
		}
	}
}
