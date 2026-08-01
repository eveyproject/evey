import type { V1Library } from "@evey/api";
import { selectHomeLibraryRows } from "./homeLibraryRows";

// The home screen's row plan, shared with web and mobile.
//
// Until 2026-08-02 the TV home hardcoded three rows plus one per library. The
// owner then asked for the home, the media page and the profile menu to match
// web/mobile "wherever it is possible", which makes the server's feed config
// the source of truth here too: same feed types, same order, same per-section
// item counts. The TV keeps its own hero and its own row chrome - that is
// presentation, not content.
//
// This module is the pure part (what to render, in what order) so it can be
// unit-tested without a renderer; the screen owns the queries and the pixels.

// Keep in sync with mobile's FEED_TYPES and web's KNOWN_FEED_TYPES.
// "recently_added" is deliberately absent everywhere: it duplicates the
// per-library rows, which already sort by date added.
export const FEED_TYPES = [
  "resume_playback",
  "next_episode",
  "collections",
  "recommended",
  "most_popular",
] as const;

export interface FeedConfigEntry {
  feedType?: string;
  enabled?: boolean;
  displayOrder?: number;
  maxItems?: number;
}

export type HomeSection =
  | { kind: "feed"; key: string; feedType: string; maxItems?: number }
  | { kind: "library"; key: string; libraryId: string; name: string; libraryType?: string; maxItems?: number }
  | { kind: "plugin"; key: string; pluginId: string };

/** A section's item budget: the admin's number when set, else the caller's
 * default. Zero must never win - it would silently fetch nothing. */
export function resolveMaxItems(configured: number | undefined, fallback: number): number {
  return configured && configured > 0 ? configured : fallback;
}

/**
 * The ordered row plan for the home screen.
 *
 * - Enabled feed-config entries first, in their configured order (this is what
 *   makes the TV honour the same layout as web and mobile).
 * - Then the libraries the config says nothing about, so a newly-added library
 *   shows up without anyone touching the config - same as web and mobile.
 * - Plugin sections the config does not mention are prepended by the caller,
 *   matching mobile: a home.section plugin is a hero widget, not a footer.
 *
 * The trailing library rows stay capped (selectHomeLibraryRows): each one is a
 * request, and the weak-NAS target is the reason that cap exists. Libraries the
 * operator explicitly placed in the config are NOT capped - that is a decision
 * they made on purpose.
 */
export function buildHomeSections(
  configs: FeedConfigEntry[],
  libraries: V1Library[],
): HomeSection[] {
  const ordered = configs.length > 0
    ? [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0))
    : FEED_TYPES.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i } as FeedConfigEntry));

  const byId = new Map<string, V1Library>();
  for (const lib of libraries) {
    if (lib.id) byId.set(lib.id, lib);
  }

  const sections: HomeSection[] = [];
  for (const cfg of ordered) {
    const feedType = cfg.feedType ?? "";
    if (!feedType || cfg.enabled === false) continue;

    if (feedType.startsWith("library:")) {
      const libraryId = feedType.slice("library:".length);
      const lib = byId.get(libraryId);
      // A config row for a library that no longer exists is stale, not an
      // error: drop it rather than render a row that can never fill.
      if (!lib) continue;
      sections.push({
        kind: "library",
        key: feedType,
        libraryId,
        name: lib.name ?? "",
        libraryType: lib.type,
        maxItems: cfg.maxItems,
      });
      continue;
    }
    if (feedType.startsWith("plugin:")) {
      sections.push({ kind: "plugin", key: feedType, pluginId: feedType.slice("plugin:".length) });
      continue;
    }
    sections.push({ kind: "feed", key: feedType, feedType, maxItems: cfg.maxItems });
  }

  const configured = new Set(sections.filter((s) => s.kind === "library").map((s) => s.key));
  for (const row of selectHomeLibraryRows(libraries.filter((l) => !configured.has(`library:${l.id}`)))) {
    sections.push({
      kind: "library",
      key: `library:${row.id}`,
      libraryId: row.id,
      name: row.name,
      libraryType: byId.get(row.id)?.type,
    });
  }
  return sections;
}
