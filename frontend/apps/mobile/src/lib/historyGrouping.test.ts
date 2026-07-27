import { describe, it, expect, afterEach } from "vitest";
import type { TFunction } from "i18next";
import { getHistoryGroupKind, getHistoryGroupLabel, groupHistoryByDate } from "./historyGrouping";

// Minimal stand-in for i18next's t(): returns the fallback (2nd arg) when
// given one, otherwise the key itself. Good enough for pure-logic tests -
// we're not exercising translation, just the bucketing.
const t = ((key: string, fallback?: string) => fallback ?? key) as unknown as TFunction;

const originalTz = process.env.TZ;
afterEach(() => {
  process.env.TZ = originalTz;
});

describe("getHistoryGroupKind", () => {
  const now = new Date(2026, 0, 16, 12, 0, 0); // Jan 16 2026, noon local

  it("buckets a timestamp from earlier today as today", () => {
    const iso = new Date(2026, 0, 16, 0, 0, 1).toISOString();
    expect(getHistoryGroupKind(iso, now)).toEqual({ kind: "today" });
  });

  it("buckets exactly one day back as yesterday", () => {
    const iso = new Date(2026, 0, 15, 23, 59, 59).toISOString();
    expect(getHistoryGroupKind(iso, now)).toEqual({ kind: "yesterday" });
  });

  it("buckets 6 days back (upper edge of thisWeek) as thisWeek", () => {
    const iso = new Date(2026, 0, 10, 0, 0, 0).toISOString();
    expect(getHistoryGroupKind(iso, now).kind).toBe("thisWeek");
  });

  it("buckets exactly 7 days back as thisMonth, not thisWeek", () => {
    const iso = new Date(2026, 0, 9, 0, 0, 0).toISOString();
    expect(getHistoryGroupKind(iso, now).kind).toBe("thisMonth");
  });

  it("buckets 29 days back (upper edge of thisMonth) as thisMonth", () => {
    const iso = new Date(2025, 11, 18, 0, 0, 0).toISOString();
    expect(getHistoryGroupKind(iso, now).kind).toBe("thisMonth");
  });

  it("buckets exactly 30 days back as older, with the raw date attached", () => {
    const iso = new Date(2025, 11, 17, 0, 0, 0).toISOString();
    const result = getHistoryGroupKind(iso, now);
    expect(result.kind).toBe("older");
    expect(result.date?.getFullYear()).toBe(2025);
  });

  it("treats a future timestamp as older rather than today (documents current, shared-with-web behavior)", () => {
    const iso = new Date(2026, 1, 1, 0, 0, 0).toISOString(); // 2 weeks in the future
    const result = getHistoryGroupKind(iso, now);
    expect(result.kind).toBe("older");
  });

  it("returns unknown for a missing timestamp", () => {
    expect(getHistoryGroupKind(undefined, now)).toEqual({ kind: "unknown" });
  });

  it("returns unknown for an unparseable timestamp", () => {
    expect(getHistoryGroupKind("not-a-date", now)).toEqual({ kind: "unknown" });
  });

  it("is sensitive to the runtime timezone since day boundaries are computed from local calendar dates", () => {
    // Same absolute instant, interpreted in two different timezones, can land
    // on either side of a local midnight and therefore change bucket.
    const instant = "2026-01-16T02:00:00.000Z";

    process.env.TZ = "America/New_York"; // instant is Jan 15, 21:00 local
    const nyNow = new Date(2026, 0, 16, 12, 0, 0);
    const nyResult = getHistoryGroupKind(instant, nyNow);

    process.env.TZ = "Asia/Tokyo"; // instant is Jan 16, 11:00 local
    const tokyoNow = new Date(2026, 0, 16, 12, 0, 0);
    const tokyoResult = getHistoryGroupKind(instant, tokyoNow);

    expect(nyResult.kind).toBe("yesterday");
    expect(tokyoResult.kind).toBe("today");
  });
});

describe("getHistoryGroupLabel", () => {
  const now = new Date(2026, 0, 16, 12, 0, 0);

  it("maps each kind to its expected fallback label", () => {
    expect(getHistoryGroupLabel(new Date(2026, 0, 16).toISOString(), t, now)).toBe("Today");
    expect(getHistoryGroupLabel(new Date(2026, 0, 15).toISOString(), t, now)).toBe("Yesterday");
    expect(getHistoryGroupLabel(undefined, t, now)).toBe("Unknown");
  });

  it("falls back to a month/year label for older entries", () => {
    const label = getHistoryGroupLabel(new Date(2025, 5, 1).toISOString(), t, now);
    expect(label).toContain("2025");
  });
});

describe("groupHistoryByDate", () => {
  const now = new Date(2026, 0, 16, 12, 0, 0);
  type Item = { id: string; lastPlayedAt?: string };
  const getIso = (item: Item) => item.lastPlayedAt;

  it("returns an empty array for an empty list", () => {
    expect(groupHistoryByDate<Item>([], getIso, t, now)).toEqual([]);
  });

  it("groups items into sections by bucket, preserving per-bucket order", () => {
    const items: Item[] = [
      { id: "a", lastPlayedAt: new Date(2026, 0, 16, 9, 0).toISOString() }, // today
      { id: "b", lastPlayedAt: new Date(2026, 0, 16, 8, 0).toISOString() }, // today
      { id: "c", lastPlayedAt: new Date(2026, 0, 15, 9, 0).toISOString() }, // yesterday
    ];

    const sections = groupHistoryByDate(items, getIso, t, now);

    expect(sections).toEqual([
      { title: "Today", data: [items[0], items[1]] },
      { title: "Yesterday", data: [items[2]] },
    ]);
  });
});
