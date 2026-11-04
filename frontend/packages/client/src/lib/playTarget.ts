// Which media id the play button should hand to the player.
//
// A series and a season own no file: streaming their id can only 404, and the
// player then blames the stream ("Failed to load stream" seen on a real TV,
// 2026-08-02). Their episode list arrives in a second request, so the honest
// answer while it is still empty is "nothing to play yet", never the parent id.
export function resolvePlayTarget(args: {
  id: string | undefined;
  isSeries: boolean;
  isSeason: boolean;
  episodeIds: (string | undefined)[];
}): string | null {
  const { id, isSeries, isSeason, episodeIds } = args;
  if (!id) return null;
  if (isSeries || isSeason) {
    return episodeIds.find((e): e is string => !!e) ?? null;
  }
  return id;
}

// The order resolvePlayTarget depends on. ListMediaItems has no sort key for
// the episode index - SortColumnMap covers title, added_at, year,
// premiere_date and community_rating - so it answers in sort_title order, and
// every client restores the numbering itself. The TV screen did not, which made
// resolvePlayTarget hand back whichever episode sorted first by TITLE and
// listed episodes alphabetically instead of 1, 2, 3.
//
// Kept next to resolvePlayTarget rather than inline in the screen because the
// two are one contract: "first in this array" only means "episode 1" while the
// array is ordered this way.
export function sortByIndexNumber<T extends { indexNumber?: number }>(items: T[]): T[] {
  return [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
}

// An episode without a number (a file of a season folder whose name states
// none) is listed after the numbered ones, as the server orders it
// (index_number NULLS LAST). index_number is a plain int32 and reads 0 both
// for "no number" and for a real E00; hasIndexNumber tells them apart. Only
// hasIndexNumber false goes last: a real E00 stays first, and a server that
// predates the flag (undefined) keeps the plain numeric order. Seasons keep
// sortByIndexNumber: their 0 is the specials season, a real number.
type EpisodeIndex = { indexNumber?: number; hasIndexNumber?: boolean };

function episodeIndexKey(item: EpisodeIndex): number {
  const n = item.indexNumber ?? 0;
  return item.hasIndexNumber === false && n === 0 ? Number.POSITIVE_INFINITY : n;
}

export function compareEpisodeIndex(a: EpisodeIndex, b: EpisodeIndex): number {
  const ka = episodeIndexKey(a);
  const kb = episodeIndexKey(b);
  if (ka === kb) return 0;
  return ka < kb ? -1 : 1;
}

export function sortEpisodesByIndex<T extends EpisodeIndex>(items: T[]): T[] {
  return [...items].sort(compareEpisodeIndex);
}
