// Merging the genre lists of several libraries into one filter list.
//
// Genre filters match the stored genre string EXACTLY, and metadata providers
// return genres in the server's language. A hardcoded English list therefore
// produces chips that can never match anything on a French library - the web
// had the same list merged into the real one, which showed every genre twice
// ("Adventure" next to "Aventure"). The only correct source is what the
// libraries actually contain.

// mergeGenres flattens per-library genre lists into one sorted, de-duplicated
// list. Blank entries are dropped: a chip with no label is unusable, and the
// backend has been seen to carry empty strings from partial metadata.
export function mergeGenres(perLibrary: (string[] | undefined)[]): string[] {
  const seen = new Set<string>();
  for (const list of perLibrary) {
    for (const g of list ?? []) {
      const trimmed = g.trim();
      if (trimmed !== "") seen.add(trimmed);
    }
  }
  // localeCompare so accented genres sort where a reader expects them
  // ("Épouvante" among the E's, not after Z).
  return Array.from(seen).sort((a, b) => a.localeCompare(b));
}
