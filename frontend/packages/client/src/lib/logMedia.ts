import { useQueries } from "@tanstack/react-query";
import { episodeLabel, libraryServiceGetMediaItem } from "@evey/api";
import type { V1MediaItem } from "@evey/api";

// What a media id in a log line IS: the operator reads "Some Series · S01E01 ·
// Pilot", not an uuid and not a bare episode title. An episode names its
// series and season by walking parentId twice (episode -> season -> series);
// the walk is cached per id by react-query, so a screen full of lines about
// the same episode costs three requests, not three per line.
export interface MediaLabel {
  kind: "movie" | "episode" | "season" | "series" | "track" | "album" | "other";
  // label is the human line without the kind word ("Some Series · S01E01 · Pilot").
  label: string;
}

export type MediaLabelIndex = ReadonlyMap<string, MediaLabel>;

export const EMPTY_MEDIA_LABELS: MediaLabelIndex = new Map();

function kindOf(type: string | undefined): MediaLabel["kind"] {
  switch (type) {
    case "MEDIA_ITEM_TYPE_MOVIE":
      return "movie";
    case "MEDIA_ITEM_TYPE_EPISODE":
      return "episode";
    case "MEDIA_ITEM_TYPE_SEASON":
      return "season";
    case "MEDIA_ITEM_TYPE_SERIES":
      return "series";
    case "MEDIA_ITEM_TYPE_TRACK":
      return "track";
    case "MEDIA_ITEM_TYPE_ALBUM":
      return "album";
    default:
      return "other";
  }
}

// resolveMediaLabel fetches the item and, for an episode or a season, its
// parents. A parent that cannot be read (deleted mid-way) degrades to what is
// known rather than failing the whole label.
export async function resolveMediaLabel(
  id: string,
  fetchItem: (id: string) => Promise<V1MediaItem> = (i) => libraryServiceGetMediaItem(i),
): Promise<MediaLabel> {
  const item = await fetchItem(id);
  const kind = kindOf(item.type);
  const title = item.title ?? id;
  const safe = async (pid: string | undefined): Promise<V1MediaItem | null> => {
    if (!pid) return null;
    try {
      return await fetchItem(pid);
    } catch {
      return null;
    }
  };
  switch (kind) {
    case "movie":
      return { kind, label: item.year ? `${title} (${item.year})` : title };
    case "episode": {
      const season = await safe(item.parentId);
      const series = item.seriesTitle ? null : await safe(season?.parentId);
      const seriesTitle = item.seriesTitle || series?.title || "";
      return {
        kind,
        label: episodeLabel({
          seriesTitle,
          seasonNumber: season?.indexNumber,
          episodeNumber: item.indexNumber,
          title,
        }),
      };
    }
    case "season": {
      const series = await safe(item.parentId);
      return { kind, label: episodeLabel({ seriesTitle: series?.title ?? title, seasonNumber: item.indexNumber }) };
    }
    default:
      return { kind, label: title };
  }
}

// useMediaLabels resolves the ids currently on screen. Pass only the visible
// ones: a live tail holds up to a thousand lines and would otherwise fetch
// them all. Unknown ids simply stay bare in the row.
export function useMediaLabels(ids: readonly string[]): MediaLabelIndex {
  const unique = Array.from(new Set(ids.filter(Boolean)));
  const results = useQueries({
    queries: unique.map((id) => ({
      queryKey: ["/api/v1/media", id, "log-label"],
      queryFn: () => resolveMediaLabel(id),
      staleTime: 5 * 60_000,
      retry: false,
    })),
  });
  const idx = new Map<string, MediaLabel>();
  results.forEach((r, i) => {
    if (r.data) idx.set(unique[i], r.data);
  });
  return idx;
}
