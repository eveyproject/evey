// Pure helpers for the genre browsing flow (Genres list -> genre grid).
//
// ListGenres is scoped to one library (see proto/evey/v1/library.proto), but a
// remote-first "flâner par genre" entry point should not make the viewer pick
// a library first - that facet already exists inside the library screen. This
// screen instead merges every browsable library's genres into one picker.

// Same slug rule as web's search.lazy.tsx genre <option> label
// (frontend/apps/web/src/routes/_auth/search.lazy.tsx): genres are free-text
// values from media metadata, not enum keys, so only the common ones found in
// i18n/*/media.yaml's `genres:` block translate - anything else falls back to
// the raw string as-is.
export function genreTranslationKey(genre: string): string {
  return `media.genres.${genre.toLowerCase().replace(/[^a-z]/g, "")}`;
}

// Distinct, case-insensitively deduped, locale-sorted merge of every
// library's genre list. First-seen casing wins so a genre stored as
// "Sci-Fi" in one library and "sci-fi" in another does not show up twice.
export function mergeGenres(lists: Array<string[] | undefined>): string[] {
  const seen = new Map<string, string>();
  for (const list of lists) {
    for (const genre of list ?? []) {
      const trimmed = genre.trim();
      if (!trimmed) continue;
      const key = trimmed.toLowerCase();
      if (!seen.has(key)) seen.set(key, trimmed);
    }
  }
  return Array.from(seen.values()).sort((a, b) => a.localeCompare(b));
}

// Route builder for a genre's grid screen - mirrors musicRoutes.ts's pattern
// of encoding the dynamic segment, since a genre string (e.g. "Sci-Fi",
// "Action & Adventure") is not URL-safe like the opaque ids the rest of the
// app links with.
export function genreHref(genre: string): string {
  return `/(auth)/genres/${encodeURIComponent(genre)}`;
}
