import { describe, it, expect } from "vitest";
import { SUN_MODELS, sunModelForDay, dayIndex } from "../src/domain/services/sunModel";

const DAY = 86_400_000;

describe("sunModel", () => {
  it("has at least 2 distinct models with unique ids", () => {
    expect(SUN_MODELS.length).toBeGreaterThanOrEqual(2);
    const ids = new Set(SUN_MODELS.map((m) => m.id));
    expect(ids.size).toBe(SUN_MODELS.length);
  });

  it("every model has non-empty id/channel/label", () => {
    for (const m of SUN_MODELS) {
      expect(m.id.length).toBeGreaterThan(0);
      expect(m.channel.length).toBeGreaterThan(0);
      expect(m.label.length).toBeGreaterThan(0);
    }
  });

  it("rotates one model per UTC day, cycling over the list", () => {
    const base = Date.UTC(2026, 0, 1);
    const seq = [];
    for (let d = 0; d < SUN_MODELS.length * 2; d++) {
      seq.push(sunModelForDay(base + d * DAY).id);
    }
    // First cycle equals the SUN_MODELS order (base day maps to some offset;
    // check the pattern repeats with the list length as period).
    for (let d = 0; d < SUN_MODELS.length; d++) {
      expect(seq[d]).toBe(seq[d + SUN_MODELS.length]);
    }
  });

  it("is stable within the same UTC day", () => {
    const morning = Date.UTC(2026, 5, 15, 1, 0, 0);
    const evening = Date.UTC(2026, 5, 15, 23, 30, 0);
    expect(sunModelForDay(morning).id).toBe(sunModelForDay(evening).id);
  });

  it("changes at the UTC-day boundary", () => {
    const lateDay1 = Date.UTC(2026, 5, 15, 23, 59, 0);
    const earlyDay2 = Date.UTC(2026, 5, 16, 0, 1, 0);
    // Adjacent days are different models as long as >1 model exists.
    expect(sunModelForDay(lateDay1).id).not.toBe(sunModelForDay(earlyDay2).id);
  });

  it("dayIndex is monotonic and increments by 1 per day", () => {
    const t = Date.UTC(2026, 2, 10, 12, 0, 0);
    expect(dayIndex(t + DAY) - dayIndex(t)).toBe(1);
  });

  it("handles pre-epoch timestamps without negative indexing", () => {
    const m = sunModelForDay(-5 * DAY);
    expect(SUN_MODELS.map((x) => x.id)).toContain(m.id);
  });
});
