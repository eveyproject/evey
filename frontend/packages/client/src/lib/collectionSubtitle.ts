export interface CollectionSubtitleParts {
  badge?: string;
  count?: string;
}

// What a collection card says under its name: "Saga" for a TMDB saga, and how
// many titles THIS viewer can see. One copy for the four clients (the web, the
// phone and the television each kept one, the browser television inlined it).
export function collectionSubtitle(
  kind: string | undefined,
  itemCount: number | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
): CollectionSubtitleParts {
  const parts: CollectionSubtitleParts = {};
  if (kind === "saga") parts.badge = t("media.collections.saga");
  if (itemCount !== undefined && itemCount > 0) parts.count = t("media.person.titleCount", { count: itemCount });
  return parts;
}
