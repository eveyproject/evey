import {
  DAYS_MONDAY_FIRST,
  WEEKDAYS,
  WEEKEND,
  normalizeSchedule,
  rangesForDay,
  toWeekRules,
  type ProfileSchedule,
  type ScheduleMode,
  type ScheduleRange,
} from "@evey/api";

// The opening-hours editor shared by the web and the phone: the rules shown
// as blocks (weekdays and weekend, or one per day), edited, and turned back
// into rules. Switching view keeps every range, or says it cannot.

export interface ScheduleBlock {
  /** Stable key: "weekdays", "weekend", or the day number. */
  key: string;
  days: number[];
  ranges: ScheduleRange[];
}

export function blocksFor(schedule: ProfileSchedule, mode: ScheduleMode): ScheduleBlock[] | null {
  if (mode === "daily") {
    return DAYS_MONDAY_FIRST.map((d) => ({ key: String(d), days: [d], ranges: rangesForDay(schedule, d) }));
  }
  const week = toWeekRules(schedule);
  if (!week) return null;
  return [
    { key: "weekdays", days: [...WEEKDAYS], ranges: rangesForDay(schedule, WEEKDAYS[0]) },
    { key: "weekend", days: [...WEEKEND], ranges: rangesForDay(schedule, WEEKEND[0]) },
  ];
}

export function scheduleFromBlocks(enabled: boolean, mode: ScheduleMode, blocks: ScheduleBlock[]): ProfileSchedule {
  return normalizeSchedule({
    enabled,
    mode,
    rules: blocks.filter((b) => b.ranges.length > 0).map((b) => ({ days: [...b.days], ranges: b.ranges.map((r) => ({ ...r })) })),
  });
}

/** A new range after the last one of a block: an hour later, or 18:00-20:00. */
export function nextRange(ranges: ScheduleRange[]): ScheduleRange {
  const last = ranges[ranges.length - 1];
  if (!last) return { start: 18 * 60, end: 20 * 60 };
  const start = Math.min(last.end % 1440 + 60, 22 * 60);
  return { start, end: Math.min(start + 120, 1440) };
}

/** "Monday"... in the viewer's language, from a 0-6 day (0 = Sunday). */
export function weekdayName(day: number, locale: string, style: "long" | "short" = "long"): string {
  // 2026-10-04 is a Sunday: day d is that date plus d days.
  const date = new Date(Date.UTC(2026, 9, 4 + day, 12));
  return new Intl.DateTimeFormat(locale, { weekday: style, timeZone: "UTC" }).format(date);
}

/** The time of an instant in the viewer's language and the device's zone. */
export function formatTime(iso: string, locale: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  return new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit" }).format(at);
}

/**
 * When the profile opens again, for "It opens again {{when}}": "today at
 * 18:00"-like wording from the viewer's language - today, tomorrow, or the
 * weekday - with the time in the device's own zone (the instant is absolute).
 */
export function formatOpening(iso: string, locale: string, now: Date = new Date()): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  const time = formatTime(iso, locale);
  const startOf = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOf(at) - startOf(now)) / 86_400_000);
  let day: string;
  if (days === 0 || days === 1) {
    day = new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(days, "day");
  } else {
    day = new Intl.DateTimeFormat(locale, { weekday: "long" }).format(at);
  }
  return `${day} ${time}`;
}

// Zones offered when the runtime cannot list its own (older Hermes builds).
const COMMON_TIME_ZONES = [
  "Europe/Paris", "Europe/Brussels", "Europe/Zurich", "Europe/London", "Europe/Berlin", "Europe/Madrid",
  "Europe/Rome", "Europe/Lisbon", "Africa/Casablanca", "Africa/Dakar", "Africa/Abidjan", "America/Montreal",
  "America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles", "America/Sao_Paulo",
  "Asia/Kolkata", "Asia/Shanghai", "Asia/Tokyo", "Australia/Sydney", "Pacific/Auckland", "UTC",
];

/** Every IANA zone the runtime knows, sorted, with the current one kept. */
export function timeZoneOptions(current?: string): string[] {
  const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
  let zones: string[] = [];
  try {
    zones = typeof sv === "function" ? sv("timeZone") : [];
  } catch {
    zones = [];
  }
  if (zones.length === 0) zones = [...COMMON_TIME_ZONES];
  if (current && !zones.includes(current)) zones = [current, ...zones];
  return [...new Set(zones)].sort();
}
