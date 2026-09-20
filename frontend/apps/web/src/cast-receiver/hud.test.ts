import { describe, it, expect } from "vitest";
import { endCardModel, formatClock, osdModel } from "./hud";

describe("formatClock", () => {
  it("renders m:ss under an hour and h:mm:ss above", () => {
    expect(formatClock(0)).toBe("0:00");
    expect(formatClock(65)).toBe("1:05");
    expect(formatClock(3661)).toBe("1:01:01");
  });

  it("clamps degenerate input", () => {
    expect(formatClock(-5)).toBe("0:00");
    expect(formatClock(NaN)).toBe("0:00");
    expect(formatClock(Infinity)).toBe("0:00");
  });
});

describe("osdModel", () => {
  it("renders elapsed/total, the remaining line and the fill", () => {
    expect(osdModel("Some Episode", 600, 2400, "fr-FR", false)).toEqual({
      title: "Some Episode",
      clock: "10:00 / 40:00",
      remaining: "30:00 restantes",
      progress: 0.25,
      status: "",
      paused: false,
    });
    expect(osdModel("Some Episode", 600, 2400, "en-US", false).remaining).toBe("30:00 left");
  });

  it("names the paused state, and only that state", () => {
    expect(osdModel("Some Episode", 600, 2400, "fr", true).status).toBe("En pause");
    expect(osdModel("Some Episode", 600, 2400, "en", true).status).toBe("Paused");
    expect(osdModel("Some Episode", 600, 2400, "en", false).status).toBe("");
  });

  it("degrades without a duration instead of showing garbage", () => {
    const hud = osdModel("Some Movie", 600, 0, "en", false);
    expect(hud.clock).toBe("10:00");
    expect(hud.remaining).toBe("");
    expect(hud.progress).toBe(0);
  });

  it("clamps a position past the duration (a seek to the very end)", () => {
    const hud = osdModel("Some Movie", 2500, 2400, "fr", false);
    expect(hud.progress).toBe(1);
    expect(hud.remaining).toBe("");
  });
});

describe("endCardModel", () => {
  it("localizes the heading and carries the finished title", () => {
    expect(endCardModel("Some Movie", "fr")).toEqual({ heading: "Lecture terminée", title: "Some Movie" });
    expect(endCardModel("Some Movie", "en").heading).toBe("Playback finished");
  });
});
