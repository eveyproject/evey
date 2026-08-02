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
