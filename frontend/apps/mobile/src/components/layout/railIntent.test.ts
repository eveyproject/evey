import { describe, it, expect } from "vitest";
import { createRailIntentTracker } from "./railIntent";

describe("createRailIntentTracker", () => {
  it("emits hide after sustained upward finger travel (scrolling down)", () => {
    const t = createRailIntentTracker(24);
    t.start(300);
    expect(t.move(290)).toBeNull();
    expect(t.move(280)).toBeNull();
    expect(t.move(270)).toBe("hide");
  });

  it("emits show after sustained downward finger travel (scrolling up)", () => {
    const t = createRailIntentTracker(24);
    t.start(100);
    expect(t.move(115)).toBeNull();
    expect(t.move(130)).toBe("show");
  });

  it("resets its accumulator after emitting, so jitter does not re-fire", () => {
    const t = createRailIntentTracker(24);
    t.start(300);
    t.move(270);
    expect(t.move(268)).toBeNull();
  });

  it("ignores mostly-horizontal moves so carousels do not toggle the rail", () => {
    const t = createRailIntentTracker(24);
    t.start(300, 0);
    expect(t.move(290, 50)).toBeNull();
    expect(t.move(280, 100)).toBeNull();
    expect(t.move(270, 150)).toBeNull();
  });

  it("a new touch clears leftover travel from the previous one", () => {
    const t = createRailIntentTracker(24);
    t.start(300);
    t.move(285);
    t.start(300);
    expect(t.move(290)).toBeNull();
  });
});
