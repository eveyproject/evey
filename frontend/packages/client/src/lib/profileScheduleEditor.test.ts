import { describe, it, expect } from "vitest";
import type { ProfileSchedule } from "@evey/api";
import { blocksFor, formatOpening, nextRange, scheduleFromBlocks, weekdayName } from "./profileScheduleEditor";

const proposal: ProfileSchedule = {
  enabled: true,
  mode: "week",
  rules: [
    { days: [1, 2, 3, 4, 5], ranges: [{ start: 1080, end: 1320 }] },
    { days: [6, 0], ranges: [{ start: 540, end: 720 }, { start: 840, end: 1260 }] },
  ],
};

describe("opening-hours editor", () => {
  // Week view, day view, and back: the same rules every way round.
  it("round-trips the blocks of both views", () => {
    const week = blocksFor(proposal, "week")!;
    expect(week.map((b) => b.key)).toEqual(["weekdays", "weekend"]);
    expect(scheduleFromBlocks(true, "week", week)).toEqual(proposal);

    const daily = blocksFor(proposal, "daily")!;
    expect(daily.map((b) => b.key)).toEqual(["1", "2", "3", "4", "5", "6", "0"]);
    const asDaily = scheduleFromBlocks(true, "daily", daily);
    expect(blocksFor(asDaily, "week")!.map((b) => b.ranges)).toEqual(week.map((b) => b.ranges));
  });

  it("refuses the week view when a weekday differs, so nothing is lost", () => {
    const daily = blocksFor(proposal, "daily")!.map((b) => (b.key === "3" ? { ...b, ranges: [{ start: 840, end: 1080 }] } : b));
    expect(blocksFor(scheduleFromBlocks(true, "daily", daily), "week")).toBeNull();
  });

  it("drops closed days and proposes a following range", () => {
    const blocks = [{ key: "1", days: [1], ranges: [] }, { key: "2", days: [2], ranges: [{ start: 600, end: 660 }] }];
    expect(scheduleFromBlocks(true, "daily", blocks).rules).toEqual([{ days: [2], ranges: [{ start: 600, end: 660 }] }]);
    expect(nextRange([])).toEqual({ start: 1080, end: 1200 });
    expect(nextRange([{ start: 540, end: 720 }])).toEqual({ start: 780, end: 900 });
  });

  it("names days and the next opening in the viewer's language", () => {
    expect(weekdayName(1, "en")).toBe("Monday");
    expect(weekdayName(0, "fr")).toBe("dimanche");
    const now = new Date(2026, 9, 5, 8, 0);
    expect(formatOpening(new Date(2026, 9, 5, 18, 0).toISOString(), "en", now)).toMatch(/^today .*6:00/i);
    expect(formatOpening(new Date(2026, 9, 6, 9, 0).toISOString(), "fr", now)).toMatch(/^demain 09:00$/);
    expect(formatOpening(new Date(2026, 9, 8, 18, 0).toISOString(), "fr", now)).toMatch(/^jeudi 18:00$/);
  });
});
