import { useActivityServiceGetPopularMedia } from "@evey/api";

export interface PopularItem {
  mediaItemId: string;
  title: string;
  type?: string;
  year?: number;
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
            playCount: it.playCount ?? 0,
          })),
        staleTime: 5 * 60_000,
        enabled,
      },
    },
  );
}
