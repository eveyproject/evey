import { useActivityServiceGetPopularMedia } from "@evey/api";

export interface PopularItem {
  mediaItemId: string;
  title: string;
  type?: string;
  playCount: number;
}

export function useMostPopular(limit = 20) {
  return useActivityServiceGetPopularMedia(
    { limit },
    {
      query: {
        select: (d) =>
          (d.items ?? []).map<PopularItem>((it) => ({
            mediaItemId: it.mediaItemId ?? "",
            title: it.title ?? "",
            type: it.type,
            playCount: it.playCount ?? 0,
          })),
        staleTime: 5 * 60_000,
      },
    },
  );
}
