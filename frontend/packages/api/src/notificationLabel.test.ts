import { describe, it, expect } from "vitest";
import { notificationLabel } from "./notificationLabel";

// A translator that renders the shape i18next would, so the assertions read
// like what a viewer sees. Plurals resolve on `count`, as i18next does with
// the _one/_other keys added to common.yaml.
const t = (key: string, opts?: Record<string, unknown>): string => {
  const count = opts?.count as number | undefined;
  switch (key) {
    case "common.notifications.newSeries":
      return `Nouvelle série : ${opts?.name}`;
    case "common.notifications.newEpisode":
      return `Nouvel épisode : ${opts?.name}`;
    case "common.notifications.seasonCount":
      return count === 1 ? `${count} saison` : `${count} saisons`;
    case "common.notifications.episodeCount":
      return count === 1 ? `${count} épisode` : `${count} épisodes`;
    default:
      return key;
  }
};

describe("notificationLabel", () => {
  // Seen on the TV on 2026-08-02, in a French UI:
  // "Nouvelle série : Some Series (1 seasons, 1 episodes)".
  it("words the whole sentence in the viewer's language", () => {
    expect(
      notificationLabel(t, {
        title: "new_series:Some Series (1 season, 1 episode)",
        subject: "Some Series",
        seasonCount: 1,
        episodeCount: 1,
      }),
    ).toBe("Nouvelle série : Some Series (1 saison, 1 épisode)");
  });

  it("pluralises each count on its own", () => {
    expect(
      notificationLabel(t, { title: "new_series:X", subject: "Some Series", seasonCount: 2, episodeCount: 1 }),
    ).toBe("Nouvelle série : Some Series (2 saisons, 1 épisode)");
  });

  it("omits a count that is absent or zero", () => {
    expect(notificationLabel(t, { title: "new_series:X", subject: "Some Series", episodeCount: 3 })).toBe(
      "Nouvelle série : Some Series (3 épisodes)",
    );
    expect(notificationLabel(t, { title: "new_series:X", subject: "Some Series", seasonCount: 0, episodeCount: 0 })).toBe(
      "Nouvelle série : Some Series",
    );
  });

  // Rows written before the change are deliberately not backfilled, so the old
  // parsing has to keep working exactly as it did.
  it("falls back to the stored title when there is no subject", () => {
    expect(notificationLabel(t, { title: "new_series:Some Series (1 season, 1 episode)" })).toBe(
      "Nouvelle série : Some Series (1 season, 1 episode)",
    );
    expect(notificationLabel(t, { title: "new_episode:S01E07" })).toBe("Nouvel épisode : S01E07");
  });

  it("returns the raw string when it carries no known type", () => {
    expect(notificationLabel(t, { title: "something custom" })).toBe("something custom");
    expect(notificationLabel(t, { title: "unknown_type:X" })).toBe("unknown_type:X");
    expect(notificationLabel(t, {})).toBe("");
  });

  it("ignores a blank subject rather than rendering an empty name", () => {
    expect(notificationLabel(t, { title: "new_series:Some Series", subject: "   " })).toBe("Nouvelle série : Some Series");
  });
});

// The rows that pre-date the structured payload.
//
// Notifications stored before the subject/counts became DATA carry only a
// title, and the label falls back to parsing it. Two shapes were never
// exercised: a title with no ":" at all, and one whose prefix is not a known
// type. Both must come out as the stored text - readable, if untranslated -
// and never as a bare i18n key, which is the beta.38 regression this whole
// module exists to prevent.
describe("notificationLabel: rows written before the structured payload", () => {
  it("renders a title with no type prefix as-is, never as a key", () => {
    const out = notificationLabel(t, { title: "Some Series is now available" });
    expect(out).toBe("Some Series is now available");
    expect(out.startsWith("common.")).toBe(false);
  });

  it("renders an UNKNOWN type prefix as-is rather than inventing a key", () => {
    const out = notificationLabel(t, { title: "new_documentary:Some Movie" });
    expect(out).toBe("new_documentary:Some Movie");
    expect(out.startsWith("common.")).toBe(false);
  });

  it("still translates a KNOWN prefix from the stored title alone", () => {
    expect(notificationLabel(t, { title: "new_series:Some Series" })).toBe("Nouvelle série : Some Series");
  });

  it("survives a missing title without rendering undefined", () => {
    expect(notificationLabel(t, {})).toBe("");
  });

  // A subject with a type the map does not know must NOT silently drop the
  // subject: it falls back to the stored title, which is the only readable
  // thing left.
  it("falls back to the stored title when the type is unknown but a subject exists", () => {
    expect(notificationLabel(t, { title: "new_documentary:Some Movie", subject: "Some Movie", type: "new_documentary" }))
      .toBe("new_documentary:Some Movie");
  });
});
