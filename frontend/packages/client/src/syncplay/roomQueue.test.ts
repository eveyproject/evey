import { describe, it, expect } from "vitest";
import { addToRoomQueue, moveInRoomQueue, removeFromRoomQueue, MAX_ROOM_QUEUE, type RoomQueueItem } from "./roomQueue";

const item = (id: string): RoomQueueItem => ({ id, title: `Some Track ${id}`, artist: "Some Artist" });

describe("room queue", () => {
  it("adds without duplicates or empty ids", () => {
    const q = addToRoomQueue([item("a")], [item("a"), item("b"), { id: "", title: "", artist: "" }]);
    expect(q.map((x) => x.id)).toEqual(["a", "b"]);
  });

  it("stops at the queue cap", () => {
    const many = Array.from({ length: MAX_ROOM_QUEUE + 20 }, (_, i) => item(String(i)));
    expect(addToRoomQueue([], many)).toHaveLength(MAX_ROOM_QUEUE);
  });

  it("moves and removes", () => {
    const q = [item("a"), item("b"), item("c")];
    expect(moveInRoomQueue(q, 0, 1).map((x) => x.id)).toEqual(["b", "a", "c"]);
    expect(moveInRoomQueue(q, 0, -1)).toBe(q);
    expect(removeFromRoomQueue(q, "b").map((x) => x.id)).toEqual(["a", "c"]);
  });
});
