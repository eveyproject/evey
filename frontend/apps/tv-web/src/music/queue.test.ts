import { beforeEach, describe, expect, it } from "vitest";
import { useMusicQueueStore, type QueueTrack } from "./queue";

const tracks: QueueTrack[] = ["a", "b", "c"].map((id) => ({ id, title: id, artist: "x", album: "y", duration: 20 }));

// The Android TV's queue rules, kept identical here: a finished album reads
// as stopped, shuffle never repeats the track that just ended.
describe("music queue", () => {
  beforeEach(() => useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false, shuffle: false }));

  it("starts at the picked track and stops after the last one", () => {
    const s = useMusicQueueStore.getState();
    s.setQueue(tracks, 1);
    expect(useMusicQueueStore.getState()).toMatchObject({ currentIndex: 1, isPlaying: true });
    s.next();
    expect(useMusicQueueStore.getState().currentIndex).toBe(2);
    s.next();
    expect(useMusicQueueStore.getState()).toMatchObject({ currentIndex: 2, isPlaying: false });
  });

  it("shuffle picks another track every time", () => {
    const s = useMusicQueueStore.getState();
    s.setQueue(tracks, 0);
    s.toggleShuffle();
    for (let i = 0; i < 20; i++) {
      const before = useMusicQueueStore.getState().currentIndex;
      s.next();
      expect(useMusicQueueStore.getState().currentIndex).not.toBe(before);
    }
  });

  it("clear empties the queue and stops", () => {
    const s = useMusicQueueStore.getState();
    s.setQueue(tracks, 0);
    s.clear();
    expect(useMusicQueueStore.getState()).toMatchObject({ queue: [], currentIndex: -1, isPlaying: false });
  });
});
