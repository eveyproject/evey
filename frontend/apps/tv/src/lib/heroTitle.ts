import { itemLabel, type Translate } from "@evey/api";

// What the home hero should call an item.
//
// "Recently added" spotlights seasons and episodes alongside films, and their
// own title is just "Season 3" or "Some Episode" - which names
// nothing across a room (seen on device, 2026-08-03). The ancestors are usually
// in the very same response, so naming the show costs no extra request; when
// they are not, the bare title stays, never a broken prefix.
//
// An episode hangs off its SEASON, not off the series, so the chain is walked
// to the top: "Some Series - Some Episode" beats "Season 3 - The Spider
// and the Fly", which still leaves the viewer guessing which show it is.
//
// Season labels come from itemLabel, never from the stored title: a season has
// no name, only a number.
type TitledItem = { id?: string; title?: string; type?: string; parentId?: string; indexNumber?: number };

const NEEDS_PARENT = new Set(["MEDIA_ITEM_TYPE_SEASON", "MEDIA_ITEM_TYPE_EPISODE"]);

// Bounded: a cycle in the data must not hang the home screen.
const MAX_HOPS = 4;

export function heroTitle(item: TitledItem, siblings: readonly TitledItem[], t: Translate): string {
  const own = itemLabel(item, t);
  if (!item.type || !NEEDS_PARENT.has(item.type)) return own;

  let cursor = item;
  let topTitle = "";
  const seen = new Set<string>([item.id ?? ""]);
  for (let hop = 0; hop < MAX_HOPS && cursor.parentId; hop++) {
    if (seen.has(cursor.parentId)) break;
    seen.add(cursor.parentId);
    const parent = siblings.find((s) => s.id === cursor.parentId);
    if (!parent) break;
    const parentLabel = itemLabel(parent, t);
    if (parentLabel) topTitle = parentLabel;
    cursor = parent;
  }

  if (!topTitle || !own || topTitle === own) return own || topTitle;
  return `${topTitle} - ${own}`;
}
