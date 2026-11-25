import { useActivityServiceGetPopularMedia, type V1MediaStream } from "@evey/api";

export interface PopularItem {
  mediaItemId: string;
  title: string;
  type?: string;
  year?: number;
  primaryBlurhash?: string;
  communityRating?: number;
  /** The card's resolution and HDR badges (see feedItemToCard). */
  videoStream?: V1MediaStream;
  playCount: number;
}

/** `enabled` false holds the request, e.g. until the home knows its row sizes. */
export function useMostPopular(limit = 20, enabled = true) {
  return useActivityServiceGetPopularMedia(
    { limit },
    {
      query: {
        select: (d) =>
          (d.items ?? []).map<PopularItem>((it) => ({
            mediaItemId: it.mediaItemId ?? "",
            title: it.title ?? "",
            type: it.type,
            year: it.year || undefined,
            primaryBlurhash: it.primaryBlurhash || undefined,
            communityRating: it.communityRating || undefined,
            // Dropped here, the popular row was the one home rail without
            // badges while the server sent them.
            videoStream: it.videoStream,
            playCount: it.playCount ?? 0,
          })),
        staleTime: 5 * 60_000,
        enabled,
      },
    },
  );
}
