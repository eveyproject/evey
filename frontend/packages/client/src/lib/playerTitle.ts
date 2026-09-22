// What the player's top bar should call what is playing.
//
// An episode's own title names nothing across a room: "Some Episode"
// could be any show (seen on device, 2026-08-03). The show's name is what the
// viewer recognises, so it leads; the episode follows.
//
// Kept pure and separate from the fetching so the shape of the answer can be
// tested without a network.
type Titled = { title?: string; type?: string } | undefined;

export function playerTitle(media: Titled, ancestorTitle?: string): string {
  const own = media?.title ?? "";
  if (media?.type !== "MEDIA_ITEM_TYPE_EPISODE") return own;
  const show = ancestorTitle?.trim() ?? "";
  if (!show || !own || show === own) return own || show;
  return `${show} - ${own}`;
}
