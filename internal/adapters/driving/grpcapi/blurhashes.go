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
		// Records are ordered by sort_order: the first poster carrying a hash
		// is the most-preferred one, same rule as the feed querier.
		for _, a := range byItem[it.Id] {
			if a != nil && a.Type == "poster" && a.Blurhash != nil && *a.Blurhash != "" {
				it.Artwork = append(it.Artwork, artworkRecordToProto(a))
				break
			}
		}
	}
}
