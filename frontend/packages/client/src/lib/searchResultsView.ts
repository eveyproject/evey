// What a search screen shows right now. Shared by the two televisions, whose
// screens ask the same question in the same order.
//
// The rule that matters: an error on the ACTIVE side wins over everything, so
// a failed music search shows the error rather than posing as "no results".

export type SearchResultsView =
  | "error"
  | "loading"
  | "prompt"
  | "musicEmpty"
  | "musicResults"
  | "mediaEmpty"
  | "mediaResults";

export function selectSearchResultsView(params: {
  searchKind: "media" | "music";
  isError: boolean;
  activeLoading: boolean;
  hasQuery: boolean;
  musicItemsCount: number;
  mediaItemsCount: number;
}): SearchResultsView {
  const { searchKind, isError, activeLoading, hasQuery, musicItemsCount, mediaItemsCount } = params;
  // isError is the ACTIVE side's error - a failed music search must show the
  // error state too, not pose as "no results".
  if (isError && hasQuery) return "error";
  if (activeLoading && hasQuery) return "loading";
  if (!hasQuery) return "prompt";
  if (searchKind === "music") return musicItemsCount === 0 ? "musicEmpty" : "musicResults";
  return mediaItemsCount === 0 ? "mediaEmpty" : "mediaResults";
}
