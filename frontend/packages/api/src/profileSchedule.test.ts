import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { eveyFetch, setTokenProvider, setUnauthorizedHandler, _resetFetcherState } from "./fetcher";
import {
  hhmmToMinutes,
  isProfileClosedError,
  minutesToHHMM,
  normalizeSchedule,
  onProfileClosed,
  rangeCrossesMidnight,
  scheduleProblems,
  toDailyRules,
  toWeekRules,
  type ProfileSchedule,
} from "./profileSchedule";

const fetchMock = vi.fn();
const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });

beforeEach(() => {
  _resetFetcherState();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { dispatchEvent: vi.fn() });
  setTokenProvider(() => null);
  setUnauthorizedHandler(() => {});
});
afterEach(() => vi.unstubAllGlobals());

// Whichever request finds the profile closed - a gateway call or a raw HTTP
// route - the app hears it once, with the next opening, and the error says why.
describe("the profile-closed signal", () => {
  it("fires from the gateway's ErrorInfo and from the HTTP header", async () => {
    const heard: Array<{ nextOpen?: string }> = [];
    const off = onProfileClosed((i) => heard.push(i));

    fetchMock.mockResolvedValueOnce(json(403, {
      code: 7,
      message: "Ce profil est verrouillé",
      details: [{ "@type": "type.googleapis.com/google.rpc.ErrorInfo", reason: "PROFILE_CLOSED", metadata: { next_open: "2026-10-05T16:00:00Z" } }],
    }));
    const err = await eveyFetch({ url: "/api/v1/feeds/home", method: "GET" }).catch((e) => e);
    expect(isProfileClosedError(err)).toBe(true);
    expect(err.nextOpen).toBe("2026-10-05T16:00:00Z");

    fetchMock.mockResolvedValueOnce(new Response("forbidden", { status: 403, headers: { "X-Evey-Error": "profile_closed" } }));
    await eveyFetch({ url: "/api/v1/media/x/stream-token", method: "POST" }).catch(() => {});

    fetchMock.mockResolvedValueOnce(json(403, { message: "forbidden" }));
    const other = await eveyFetch({ url: "/api/v1/x", method: "GET" }).catch((e) => e);
    expect(isProfileClosedError(other)).toBe(false);

    off();
    expect(heard).toEqual([{ nextOpen: "2026-10-05T16:00:00Z" }, {}]);
  });
});

describe("schedule editor helpers", () => {
  const sched: ProfileSchedule = {
    enabled: true,
    mode: "week",
    rules: [
      { days: [1, 2, 3, 4, 5], ranges: [{ start: 18 * 60, end: 22 * 60 }] },
      { days: [6, 0], ranges: [{ start: 9 * 60, end: 12 * 60 }, { start: 14 * 60, end: 21 * 60 }] },
    ],
  };

  it("reads and writes HH:MM, midnight as 24:00 at the end", () => {
    expect(minutesToHHMM(18 * 60)).toBe("18:00");
    expect(hhmmToMinutes("07:30")).toBe(450);
    expect(hhmmToMinutes("24:00")).toBe(1440);
    expect(hhmmToMinutes("24:30")).toBeNull();
    expect(hhmmToMinutes("7h")).toBeNull();
    expect(rangeCrossesMidnight({ start: 20 * 60, end: 30 })).toBe(true);
  });

  // Week and day-by-day are two views of the same rules: going daily expands
  // the blocks, coming back works only when every day of a block agrees.
  it("switches between the two views without losing a range", () => {
    const daily = toDailyRules(sched);
    expect(daily).toHaveLength(7);
    expect(daily[0]).toEqual({ days: [1], ranges: [{ start: 1080, end: 1320 }] });
    expect(toWeekRules({ ...sched, rules: daily })).toEqual(sched.rules);

    const wednesdayOff = daily.map((r) => (r.days[0] === 3 ? { ...r, ranges: [{ start: 14 * 60, end: 18 * 60 }] } : r));
    expect(toWeekRules({ ...sched, rules: wednesdayOff })).toBeNull();
  });

  it("says what is wrong and sends only what the server accepts", () => {
    expect(scheduleProblems({ enabled: true, mode: "week", rules: [] })).toEqual(["no_rules"]);
    expect(scheduleProblems({ enabled: true, mode: "daily", rules: [{ days: [1], ranges: [{ start: 60, end: 60 }] }] })).toContain("empty_range");
    expect(normalizeSchedule({ enabled: true, mode: "daily", rules: [{ days: [1], ranges: [{ start: 20 * 60, end: 0 }] }, { days: [], ranges: [{ start: 1, end: 2 }] }] }))
      .toEqual({ enabled: true, mode: "daily", rules: [{ days: [1], ranges: [{ start: 1200, end: 1440 }] }] });
  });
});
