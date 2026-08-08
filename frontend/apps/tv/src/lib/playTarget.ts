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
