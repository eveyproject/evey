import { episodeLabel } from "@evey/api";

// What the player's top bar should call what is playing.
//
// An episode's own title names nothing across a room: "Some Episode"
// could be any show (seen on device, 2026-08-03). It reads as the shared
// episode label, "Series · S01E03 · Title"; a film is its title alone.
//
// Kept pure and separate from the fetching so the shape of the answer can be
// tested without a network.
type Titled = { title?: string; type?: string; indexNumber?: number } | undefined;

export function playerTitle(media: Titled, ancestorTitle?: string, seasonNumber?: number): string {
  const own = media?.title ?? "";
  if (media?.type !== "MEDIA_ITEM_TYPE_EPISODE") return own;
  const show = ancestorTitle?.trim() ?? "";
  if (show && show === own) return own;
  return episodeLabel({ seriesTitle: show, seasonNumber, episodeNumber: media?.indexNumber, title: own });
}
