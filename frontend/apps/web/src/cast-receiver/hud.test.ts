import { describe, it, expect } from "vitest";
import { endCardModel, formatClock, pauseHudModel } from "./hud";

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

describe("pauseHudModel", () => {
  it("renders elapsed/total and the remaining line", () => {
    expect(pauseHudModel("Some Episode", 600, 2400, "fr-FR")).toEqual({
      title: "Some Episode",
      clock: "10:00 / 40:00",
      remaining: "30:00 restantes",
    });
    expect(pauseHudModel("Some Episode", 600, 2400, "en-US").remaining).toBe("30:00 left");
  });

  it("degrades without a duration instead of showing garbage", () => {
    const hud = pauseHudModel("Some Movie", 600, 0, "en");
    expect(hud.clock).toBe("10:00");
    expect(hud.remaining).toBe("");
  });
});

describe("endCardModel", () => {
  it("localizes the heading and carries the finished title", () => {
    expect(endCardModel("Some Movie", "fr")).toEqual({ heading: "Lecture terminée", title: "Some Movie" });
    expect(endCardModel("Some Movie", "en").heading).toBe("Playback finished");
  });
});
