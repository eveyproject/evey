import { eveyFetch } from "./fetcher";

// Opening hours of a kids profile. Times are minutes since midnight in the
// SERVER's time zone; an end at or before the start runs past midnight. The
// server enforces them on every request, so this is only what the editor
// shows and the lock screen reads.

export interface ScheduleRange {
  start: number;
  end: number;
}

export interface ScheduleRule {
  /** 0 = Sunday ... 6 = Saturday, like Date.getDay(). */
  days: number[];
  ranges: ScheduleRange[];
}

export type ScheduleMode = "week" | "daily";

export interface ProfileSchedule {
  enabled: boolean;
  mode: ScheduleMode;
  rules: ScheduleRule[];
}

export interface ProfileScheduleAccess {
  open: boolean;
  overridden: boolean;
  closesAt?: string;
  nextOpen?: string;
}

export interface ProfileScheduleState {
  isKids: boolean;
  schedule: ProfileSchedule | null;
  /** The suggested hours, to prefill an editor with nothing saved yet. */
  proposal: ProfileSchedule;
  access: ProfileScheduleAccess;
  timezone: string;
}

export function profileScheduleKey(profileId: string) {
  return [`/api/v1/profiles/${profileId}/schedule`] as const;
}

export function getProfileSchedule(profileId: string, signal?: AbortSignal) {
  return eveyFetch<ProfileScheduleState>({ url: `/api/v1/profiles/${encodeURIComponent(profileId)}/schedule`, method: "GET", signal });
}

/** null removes the opening hours. */
export function updateProfileSchedule(profileId: string, schedule: ProfileSchedule | null) {
  return eveyFetch<ProfileScheduleState>({
    url: `/api/v1/profiles/${encodeURIComponent(profileId)}/schedule`,
    method: "PUT",
    data: { schedule },
  });
}

/** A parent's PIN opens a closed kids profile until its next opening. */
export function unlockProfileSchedule(profileId: string, pin: string) {
  return eveyFetch<ProfileScheduleState>({
    url: `/api/v1/profiles/${encodeURIComponent(profileId)}/schedule/unlock`,
    method: "POST",
    data: { pin },
  });
}

export { onProfileClosed, notifyProfileClosed, profileClosedFromBody, isProfileClosedError, type ProfileClosedInfo } from "./profileClosed";

// --- editor helpers ---------------------------------------------------------

export const WEEKDAYS = [1, 2, 3, 4, 5] as const;
export const WEEKEND = [6, 0] as const;
/** Monday first, the order an editor lists days in. */
export const DAYS_MONDAY_FIRST = [1, 2, 3, 4, 5, 6, 0] as const;
const MINUTES_PER_DAY = 24 * 60;

export function minutesToHHMM(m: number): string {
  const v = ((m % MINUTES_PER_DAY) + MINUTES_PER_DAY) % MINUTES_PER_DAY;
  return `${String(Math.floor(v / 60)).padStart(2, "0")}:${String(v % 60).padStart(2, "0")}`;
}

/** "HH:MM" to minutes; "24:00" is the end of the day. null when malformed. */
export function hhmmToMinutes(s: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s.trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (min > 59 || h > 24 || (h === 24 && min !== 0)) return null;
  return h * 60 + min;
}

/** True when the range runs past midnight into the next day. */
export function rangeCrossesMidnight(r: ScheduleRange): boolean {
  return r.end <= r.start;
}

/** The ranges that apply on one day, from the rules naming it. */
export function rangesForDay(s: ProfileSchedule, day: number): ScheduleRange[] {
  return s.rules.filter((r) => r.days.includes(day)).flatMap((r) => r.ranges.map((x) => ({ ...x })));
}

function sameRanges(a: ScheduleRange[], b: ScheduleRange[]): boolean {
  if (a.length !== b.length) return false;
  const key = (r: ScheduleRange[]) => r.map((x) => `${x.start}-${x.end}`).sort().join(",");
  return key(a) === key(b);
}

/**
 * Daily view: one rule per day that has ranges, Monday first. A day with no
 * range is closed.
 */
export function toDailyRules(s: ProfileSchedule): ScheduleRule[] {
  return DAYS_MONDAY_FIRST.map((d) => ({ days: [d], ranges: rangesForDay(s, d) })).filter((r) => r.ranges.length > 0);
}

/**
 * Week view: weekdays and weekend as two blocks. Returns null when the days
 * of one block disagree (Wednesday differs from Monday): that schedule can
 * only be shown day by day, and switching would lose it.
 */
export function toWeekRules(s: ProfileSchedule): ScheduleRule[] | null {
  const out: ScheduleRule[] = [];
  for (const block of [WEEKDAYS, WEEKEND]) {
    const first = rangesForDay(s, block[0]);
    if (block.some((d) => !sameRanges(rangesForDay(s, d), first))) return null;
    if (first.length > 0) out.push({ days: [...block], ranges: first });
  }
  return out;
}

/** Client-side mirror of the server's checks, to say what is wrong before saving. */
export function scheduleProblems(s: ProfileSchedule): Array<"empty_range" | "no_rules"> {
  const problems: Array<"empty_range" | "no_rules"> = [];
  if (s.enabled && s.rules.every((r) => r.ranges.length === 0)) problems.push("no_rules");
  if (s.rules.some((r) => r.ranges.some((x) => x.start === x.end))) problems.push("empty_range");
  return problems;
}

/** What the server accepts: rules without ranges or days dropped. */
export function normalizeSchedule(s: ProfileSchedule): ProfileSchedule {
  return {
    enabled: s.enabled,
    mode: s.mode,
    rules: s.rules
      .filter((r) => r.days.length > 0 && r.ranges.length > 0)
      .map((r) => ({ days: [...r.days], ranges: r.ranges.map((x) => ({ start: x.start, end: x.end === 0 && x.start > 0 ? MINUTES_PER_DAY : x.end })) })),
  };
}
