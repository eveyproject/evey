import { describe, expect, it } from "vitest";
import { applyUiScale, dpPx } from "./uiScale";

// 1rem is 10 dp of the Android television: the screen width over 960 dp,
// times the interface size. A 4K panel lays out like a 1080p one.
describe("the interface scale", () => {
  it("makes 1rem ten Android dp at the chosen size", () => {
    Object.defineProperty(window, "innerWidth", { value: 1920, configurable: true });
    applyUiScale(80);
    expect(document.documentElement.style.fontSize).toBe("16px");
    expect(dpPx()).toBeCloseTo(1.6);
    applyUiScale(100);
    expect(document.documentElement.style.fontSize).toBe("20px");
  });

  it("follows the screen, so a 4K panel keeps the same proportions", () => {
    Object.defineProperty(window, "innerWidth", { value: 3840, configurable: true });
    applyUiScale(80);
    expect(document.documentElement.style.fontSize).toBe("32px");
    Object.defineProperty(window, "innerWidth", { value: 1280, configurable: true });
    window.dispatchEvent(new Event("resize"));
    expect(document.documentElement.style.fontSize).toBe(`${(1280 / 960) * 0.8 * 10}px`);
  });
});
