import type { TFunction } from "i18next";

const DAY_MS = 24 * 60 * 60 * 1000;

export type HistoryGroupKind =
  | "unknown"
  | "today"
  | "yesterday"
  | "thisWeek"
  | "thisMonth"
  | "older";

export interface HistoryGroupResult {
  kind: HistoryGroupKind;
  /** Only set for "older" - the raw date used to render the month/year fallback label. */
  date?: Date;
}

/**
 * Mirrors the web WatchHistory bucketing (day-boundary diff against a normalized
 * "today") so both clients group the same timestamp into the same bucket.
 * Timestamps that fail to parse, or are missing, fall into "unknown" rather
 * than throwing - the caller decides how to label that.
 */
export function getHistoryGroupKind(iso: string | undefined, now: Date = new Date()): HistoryGroupResult {
  if (!iso) return { kind: "unknown" };
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return { kind: "unknown" };

  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const itemDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffDays = Math.floor((today.getTime() - itemDate.getTime()) / DAY_MS);

  if (diffDays === 0) return { kind: "today" };
  if (diffDays === 1) return { kind: "yesterday" };
  if (diffDays < 7 && diffDays > 0) return { kind: "thisWeek" };
  if (diffDays < 30 && diffDays > 0) return { kind: "thisMonth" };
  return { kind: "older", date };
}

/** Translates a bucket kind into its display label, matching the web copy. */
export function getHistoryGroupLabel(iso: string | undefined, t: TFunction, now: Date = new Date()): string {
  const group = getHistoryGroupKind(iso, now);
  switch (group.kind) {
    case "unknown":
      return t("common.unknown", "Unknown");
    case "today":
      return t("auth.watchHistory.today", "Today");
    case "yesterday":
      return t("auth.watchHistory.yesterday", "Yesterday");
    case "thisWeek":
      return t("auth.watchHistory.thisWeek", "This Week");
    case "thisMonth":
      return t("auth.watchHistory.thisMonth", "This Month");
    case "older":
      return group.date!.toLocaleDateString(undefined, { year: "numeric", month: "long" });
  }
}

export interface HistorySection<T> {
  title: string;
  data: T[];
}

/**
 * Buckets a flat history list into date sections, preserving each item's
 * relative order within its bucket. Bucket order follows first-seen order in
 * the input (the caller is expected to pass items already sorted newest-first).
 */
export function groupHistoryByDate<T>(
  items: T[],
  getIso: (item: T) => string | undefined,
  t: TFunction,
  now: Date = new Date(),
): HistorySection<T>[] {
  const grouped = items.reduce<Record<string, T[]>>((acc, item) => {
    const label = getHistoryGroupLabel(getIso(item), t, now);
    (acc[label] ??= []).push(item);
    return acc;
  }, {});
  return Object.entries(grouped).map(([title, data]) => ({ title, data }));
}
