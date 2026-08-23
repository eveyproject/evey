// What a card, a list row or a heading should call an item.
//
// Two rules live here, and only here, so the three apps answer them the same
// way:
//
// 1. A season has no name. Its label is derived from its number, client-side,
//    through media.season - like every other translated string, because nothing
//    translated is ever stored. The title the scan freezes on a season row is an
//    English "Season N" and must never reach a screen.
// 2. The server stamps seriesTitle on episode and season payloads, so naming
//    the show costs no extra request; items without the stamp keep their own
//    title, never a broken prefix.

export const SEASON_TYPE = "MEDIA_ITEM_TYPE_SEASON";

// The caller's i18n translator. Only media.season is ever asked for.
export type Translate = (key: string, options?: Record<string, unknown>) => string;

type SeasonLike = { title?: string; indexNumber?: number };
type TypedItem = SeasonLike & { type?: string };
type StampedItem = TypedItem & { seriesTitle?: string };

// The one rule, for callers that already know they hold a season (a season
// picker, a season list).
//
// Fallback: an index-less season keeps its stored title. A search hit is the
// real case - the Meilisearch document carries no index_number, so a season
// found through search has nothing to derive from. Showing the frozen
// "Season 2" untranslated beats showing a blank row, and it is at least the
// number the viewer is looking for.
export function seasonLabel(season: SeasonLike, t: Translate): string {
  if (season.indexNumber !== undefined && season.indexNumber !== null) {
    return t("media.season", { number: season.indexNumber });
  }
  return season.title ?? "";
}

// What to call an item whose type is not known in advance.
export function itemLabel(item: TypedItem, t: Translate): string {
  if (item.type === SEASON_TYPE) return seasonLabel(item, t);
  return item.title ?? "";
}

export function displayTitle(item: StampedItem, t: Translate): string {
  const own = itemLabel(item, t);
  if (item.seriesTitle && own) return `${item.seriesTitle} - ${own}`;
  return item.seriesTitle || own || "";
}
