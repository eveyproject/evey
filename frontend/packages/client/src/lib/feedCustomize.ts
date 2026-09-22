import { FEED_TYPES, type FeedConfigEntry } from "./homeSections";

// The rules behind the "customize the home" panel, kept out of the component.
//
// They are shared with mobile (apps/mobile/app/(auth)/(tabs)/index.tsx) and web
// and every one of them exists because of a real defect:
//   - orphan rows: a feed type that no longer resolves rendered as a raw UUID
//     with no way to dismiss it;
//   - newly installed plugins: absent from a stored config, they were invisible
//     until something else forced a rewrite.
// Pure functions so both can be tested without a screen.

export interface FeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
}

/**
 * The rows to show, in order: stored config minus what no longer exists, plus
 * home.section plugins the config has never heard of.
 *
 * New plugins go FIRST so an install is visible immediately; everything keeps
 * its relative order otherwise.
 */
export function buildFeedRows(
  configs: readonly FeedConfigEntry[],
  libraryNames: Record<string, string>,
  pluginNames: Record<string, string>,
): FeedRow[] {
  const known = new Set<string>(FEED_TYPES);
  const kept = configs.filter((c) => {
    const ft = c.feedType ?? "";
    if (known.has(ft)) return true;
    if (ft.startsWith("library:")) return !!libraryNames[ft.replace("library:", "")];
    if (ft.startsWith("plugin:")) return !!pluginNames[ft.replace("plugin:", "")];
    return false;
  });

  const present = new Set(kept.map((c) => c.feedType ?? ""));
  const fresh = Object.keys(pluginNames)
    .map((id) => `plugin:${id}`)
    .filter((ft) => !present.has(ft))
    .map((feedType) => ({ feedType, enabled: true, displayOrder: 0 }));

  return [...fresh, ...kept.map((c) => ({
    feedType: c.feedType ?? "",
    enabled: c.enabled ?? true,
    displayOrder: c.displayOrder ?? 0,
  }))]
    // Renumber from zero so displayOrder is always the position, never a stale
    // value inherited from a config written before a row was removed.
    .map((row, i) => ({ ...row, displayOrder: i }));
}

/** Swap a row with its neighbour, renumbering. Out-of-range moves are no-ops. */
export function moveRow(rows: readonly FeedRow[], index: number, direction: -1 | 1): FeedRow[] {
  const target = index + direction;
  if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) {
    return [...rows];
  }
  const next = [...rows];
  [next[index], next[target]] = [next[target], next[index]];
  return next.map((row, i) => ({ ...row, displayOrder: i }));
}

/** Flip one row's enabled flag. */
export function toggleRow(rows: readonly FeedRow[], index: number): FeedRow[] {
  return rows.map((row, i) => (i === index ? { ...row, enabled: !row.enabled } : row));
}
