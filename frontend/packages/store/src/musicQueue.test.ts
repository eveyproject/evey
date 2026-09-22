import { describe, expect, it } from "vitest";
import { nextQueueIndex, previousQueueIndex, randomOtherIndex, removeFromQueueAt } from "./musicQueue";

// One set of rules for the four players. Each case here was a behaviour one
// client had and another did not.
describe("queue rules", () => {
  it("stops after the last track instead of looping", () => {
    expect(nextQueueIndex(3, 1, false)).toBe(2);
    expect(nextQueueIndex(3, 2, false)).toBe("stop");
    expect(nextQueueIndex(0, -1, false)).toBe("stop");
  });

  it("shuffle never picks the track that is playing, and a queue of one stays", () => {
    for (let i = 0; i < 50; i++) expect(nextQueueIndex(4, 2, true)).not.toBe(2);
    expect(nextQueueIndex(1, 0, true)).toBe(0);
    expect(previousQueueIndex(1, 0, true)).toBe(0);
  });

  it("previous stays on the first track rather than falling off the head", () => {
    expect(previousQueueIndex(3, 1, false)).toBe(0);
    expect(previousQueueIndex(3, 0, false)).toBe(0);
  });

  it("removing an entry keeps the cursor on the same track", () => {
    expect(removeFromQueueAt(["a", "b", "c"], 2, 0)).toEqual({ queue: ["b", "c"], currentIndex: 1 });
    expect(removeFromQueueAt(["a", "b", "c"], 1, 1)).toEqual({ queue: ["a", "c"], currentIndex: 1 });
    expect(removeFromQueueAt(["a"], 0, 0)).toEqual({ queue: [], currentIndex: -1 });
  });

  it("randomOtherIndex is bounded and never the current one", () => {
    const seq = [0, 0.99, 0.5];
    let i = 0;
    const idx = randomOtherIndex(3, 1, () => seq[i++ % seq.length]);
    expect(idx).toBeGreaterThanOrEqual(0);
    expect(idx).toBeLessThan(3);
    expect(idx).not.toBe(1);
  });
});
