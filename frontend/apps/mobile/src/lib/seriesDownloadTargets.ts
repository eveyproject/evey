import type { V1MediaItem } from "@evey/api";
import { hasMediaFile } from "../components/media/mediaFile";

export interface EpisodeTarget {
  episode: V1MediaItem;
  seasonNumber?: number;
}

/**
 * Every downloadable episode of a series, season by season in index order.
 * The season number travels with each episode: offline there is no way to
 * resolve an episode's season, and the screen only knows the one on show.
 */
export async function seriesDownloadTargets(
  seasons: V1MediaItem[],
  listEpisodes: (seasonId: string) => Promise<V1MediaItem[]>,
): Promise<EpisodeTarget[]> {
  const byIndex = (a: V1MediaItem, b: V1MediaItem) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0);
  const targets: EpisodeTarget[] = [];
  for (const season of [...seasons].sort(byIndex)) {
    if (!season.id) continue;
    const episodes = await listEpisodes(season.id);
    for (const episode of [...episodes].sort(byIndex)) {
      if (episode.id && hasMediaFile(episode)) targets.push({ episode, seasonNumber: season.indexNumber });
    }
  }
  return targets;
}
