import { describe, it, expect } from "vitest";
import type { V1Library } from "@evey/api";
import { buildHomeSections, resolveMaxItems, FEED_TYPES } from "./homeSections";
import { MAX_HOME_LIBRARY_ROWS } from "./homeLibraryRows";

const movies: V1Library = { id: "lib-mov", name: "Movies", type: "LIBRARY_TYPE_MOVIES" };
const shows: V1Library = { id: "lib-tv", name: "Shows", type: "LIBRARY_TYPE_SHOWS" };
const music: V1Library = { id: "lib-mus", name: "Music", type: "LIBRARY_TYPE_MUSIC" };
const photos: V1Library = { id: "lib-pho", name: "Photos", type: "LIBRARY_TYPE_PHOTOS" };
const unrouted: V1Library = { id: "lib-unk", name: "Unknown", type: "LIBRARY_TYPE_UNSPECIFIED" };

describe("buildHomeSections", () => {
  // A fresh install has no stored config; the TV must still show the same
  // shelves in the same order as web and mobile rather than its own idea.
  it("falls back to the shared default order when nothing is configured", () => {
    const sections = buildHomeSections([], []);
    expect(sections.map((s) => s.key)).toEqual([...FEED_TYPES]);
  });

  // This is the whole point of the change: an admin who reorders the home used
  // to change nothing on the television.
  it("honours the configured order", () => {
    const sections = buildHomeSections(
      [
        { feedType: "most_popular", enabled: true, displayOrder: 2 },
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "recommended", enabled: true, displayOrder: 1 },
      ],
      [],
    );
    expect(sections.map((s) => s.key)).toEqual(["resume_playback", "recommended", "most_popular"]);
  });

  it("drops disabled sections", () => {
    const sections = buildHomeSections(
      [
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "recommended", enabled: false, displayOrder: 1 },
      ],
      [],
    );
    expect(sections.map((s) => s.key)).toEqual(["resume_playback"]);
  });

  it("resolves a library: entry against the real library, carrying its type", () => {
    const sections = buildHomeSections(
      [{ feedType: `library:${music.id}`, enabled: true, displayOrder: 0, maxItems: 7 }],
      [music],
    );
    expect(sections).toEqual([
      { kind: "library", key: "library:lib-mus", libraryId: "lib-mus", name: "Music", libraryType: "LIBRARY_TYPE_MUSIC", maxItems: 7 },
    ]);
  });

  // A config row pointing at a deleted library is stale, not an error - a row
  // that can never fill would just sit there empty.
  it("drops a library: entry whose library no longer exists", () => {
    const sections = buildHomeSections(
      [{ feedType: "library:gone", enabled: true, displayOrder: 0 }],
      [movies],
    );
    expect(sections.filter((s) => s.kind === "library").map((s) => s.key)).toEqual(["library:lib-mov"]);
  });

  it("appends libraries the config says nothing about, so a new one shows up by itself", () => {
    const sections = buildHomeSections(
      [{ feedType: "resume_playback", enabled: true, displayOrder: 0 }],
      [movies, shows],
    );
    expect(sections.map((s) => s.key)).toEqual(["resume_playback", "library:lib-mov", "library:lib-tv"]);
  });

  it("does not append a library the config already places", () => {
    const sections = buildHomeSections(
      [{ feedType: `library:${movies.id}`, enabled: true, displayOrder: 0 }],
      [movies, shows],
    );
    expect(sections.map((s) => s.key)).toEqual(["library:lib-mov", "library:lib-tv"]);
  });

  // Music and photo libraries earned a row once the TV gained screens to send
  // the viewer to. A type with no screen gets none, and a card that lands
  // nowhere is worse than an absent row.
  it("gives music and photo libraries a row, but not an unrouted type", () => {
    const sections = buildHomeSections([], [music, photos, unrouted]);
    const libs = sections.filter((s) => s.kind === "library").map((s) => s.key);
    expect(libs).toEqual(["library:lib-mus", "library:lib-pho"]);
  });

  // Each unconfigured row is one more request on a NAS-class box, which is why
  // that list is capped - while rows the operator placed on purpose are not.
  it("caps the unconfigured library rows but never the configured ones", () => {
    const many = Array.from({ length: MAX_HOME_LIBRARY_ROWS + 4 }, (_, i) => ({
      id: `lib-${i}`,
      name: `Library ${i}`,
      type: "LIBRARY_TYPE_MOVIES",
    })) as V1Library[];

    const auto = buildHomeSections([], many);
    expect(auto.filter((s) => s.kind === "library")).toHaveLength(MAX_HOME_LIBRARY_ROWS);

    const configured = buildHomeSections(
      many.map((l, i) => ({ feedType: `library:${l.id}`, enabled: true, displayOrder: i })),
      many,
    );
    expect(configured.filter((s) => s.kind === "library")).toHaveLength(many.length);
  });

  it("keeps plugin sections at their configured position", () => {
    const sections = buildHomeSections(
      [
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "plugin:plg-abc", enabled: true, displayOrder: 1 },
      ],
      [],
    );
    expect(sections[1]).toEqual({ kind: "plugin", key: "plugin:plg-abc", pluginId: "plg-abc" });
  });

  it("ignores an entry with no feed type at all", () => {
    expect(buildHomeSections([{ enabled: true, displayOrder: 0 }], [])).toEqual([]);
  });
});

describe("resolveMaxItems", () => {
  it("prefers the configured value", () => {
    expect(resolveMaxItems(7, 20)).toBe(7);
  });

  // Zero must never win: it would silently fetch nothing and leave an empty
  // shelf that looks like a broken server.
  it("falls back for zero, undefined and negatives", () => {
    expect(resolveMaxItems(0, 20)).toBe(20);
    expect(resolveMaxItems(undefined, 20)).toBe(20);
    expect(resolveMaxItems(-3, 20)).toBe(20);
  });
});
