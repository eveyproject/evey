import { describe, it, expect } from "vitest";
import { isCurrentQueueTrack } from "./currentTrack";

const queue = [{ id: "t1" }, { id: "t2" }, { id: "t3" }];

describe("isCurrentQueueTrack", () => {
  it("reconnait la piste en cours", () => {
    expect(isCurrentQueueTrack(queue, 1, "t2")).toBe(true);
    expect(isCurrentQueueTrack(queue, 1, "t1")).toBe(false);
  });

  // The actual bug: the visible list is filtered, the queue is not. Position 0
  // of the list is "t3" here, while the queue sits on "t1" - comparing indices
  // lit up the wrong row.
  it("ne se laisse pas berner par une liste filtree qui a une autre position", () => {
    const filtered = [{ id: "t3" }, { id: "t2" }];
    expect(isCurrentQueueTrack(queue, 0, filtered[0].id)).toBe(false);
    expect(isCurrentQueueTrack(queue, 1, filtered[1].id)).toBe(true);
  });

  it("ne surligne rien quand la file est vide ou le curseur hors bornes", () => {
    expect(isCurrentQueueTrack([], 0, "t1")).toBe(false);
    expect(isCurrentQueueTrack(queue, -1, "t1")).toBe(false);
    expect(isCurrentQueueTrack(queue, 9, "t1")).toBe(false);
    expect(isCurrentQueueTrack(undefined, 0, "t1")).toBe(false);
  });

  // Two rows with no id are two unknown rows, not the same one.
  it("ne fait jamais correspondre deux pistes sans identifiant", () => {
    expect(isCurrentQueueTrack([{ id: undefined }], 0, undefined)).toBe(false);
  });
});
