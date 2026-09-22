import { describe, it, expect, beforeEach } from "vitest";
import { useMusicQueueStore, type QueueTrack } from "./musicQueue";

function makeAlbum(count: number): QueueTrack[] {
  return Array.from({ length: count }, (_, i) => ({
    id: `tr-${i + 1}`,
    title: `Track ${i + 1}`,
    artist: "Test Artist",
    album: "Test Album",
    duration: 180,
  }));
}

beforeEach(() => {
  useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false });
});

describe("musicQueue store", () => {
  it("enqueues an album starting at the given track and marks it playing", () => {
    const album = makeAlbum(4);
    useMusicQueueStore.getState().setQueue(album, 1);

    const state = useMusicQueueStore.getState();
    expect(state.queue).toEqual(album);
    expect(state.currentIndex).toBe(1);
    expect(state.isPlaying).toBe(true);
  });

  it("defaults to the first track when no start index is given", () => {
    const album = makeAlbum(3);
    useMusicQueueStore.getState().setQueue(album);

    expect(useMusicQueueStore.getState().currentIndex).toBe(0);
  });

  it("advances to the next track and stops at the end of the album", () => {
    const album = makeAlbum(3);
    useMusicQueueStore.getState().setQueue(album, 0);

    useMusicQueueStore.getState().next();
    expect(useMusicQueueStore.getState().currentIndex).toBe(1);

    useMusicQueueStore.getState().next();
    expect(useMusicQueueStore.getState().currentIndex).toBe(2);

    // Already on the last track - next() never wraps around: the album is
    // over, and the bar must not keep showing a pause button over silence.
    useMusicQueueStore.getState().next();
    expect(useMusicQueueStore.getState().currentIndex).toBe(2);
    expect(useMusicQueueStore.getState().isPlaying).toBe(false);
  });

  it("moves to the previous track and stops at the start", () => {
    const album = makeAlbum(3);
    useMusicQueueStore.getState().setQueue(album, 2);

    useMusicQueueStore.getState().previous();
    expect(useMusicQueueStore.getState().currentIndex).toBe(1);

    useMusicQueueStore.getState().previous();
    expect(useMusicQueueStore.getState().currentIndex).toBe(0);

    useMusicQueueStore.getState().previous();
    expect(useMusicQueueStore.getState().currentIndex).toBe(0);
  });

  it("jumps directly to a track index via playTrack, ignoring out-of-range indices", () => {
    const album = makeAlbum(3);
    useMusicQueueStore.getState().setQueue(album, 0);

    useMusicQueueStore.getState().playTrack(2);
    expect(useMusicQueueStore.getState().currentIndex).toBe(2);

    useMusicQueueStore.getState().playTrack(99);
    expect(useMusicQueueStore.getState().currentIndex).toBe(2);
  });

  it("toggles play/pause without changing the current track", () => {
    const album = makeAlbum(2);
    useMusicQueueStore.getState().setQueue(album, 0);
    expect(useMusicQueueStore.getState().isPlaying).toBe(true);

    useMusicQueueStore.getState().togglePlay();
    expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    expect(useMusicQueueStore.getState().currentIndex).toBe(0);

    useMusicQueueStore.getState().togglePlay();
    expect(useMusicQueueStore.getState().isPlaying).toBe(true);
  });

  it("clears the queue entirely", () => {
    useMusicQueueStore.getState().setQueue(makeAlbum(2), 1);
    useMusicQueueStore.getState().clear();

    const state = useMusicQueueStore.getState();
    expect(state.queue).toEqual([]);
    expect(state.currentIndex).toBe(-1);
    expect(state.isPlaying).toBe(false);
  });
});

// Shuffle, the same rule as the phone and the browser: next and previous pick
// any other track of the album, never the one playing, and an album of one
// track has nowhere to go. The toggle is what a viewer expects to find on the
// bar (owner, 2026-09-22: the mode was on every other client but the TV).
describe("shuffle", () => {
  it("picks another track on next and previous, and stays put alone", () => {
    const tracks = [
      { id: "a", title: "A", artist: "", album: "", duration: 1 },
      { id: "b", title: "B", artist: "", album: "", duration: 1 },
      { id: "c", title: "C", artist: "", album: "", duration: 1 },
    ];
    useMusicQueueStore.setState({ queue: tracks, currentIndex: 2, isPlaying: true, shuffle: false });
    useMusicQueueStore.getState().toggleShuffle();
    expect(useMusicQueueStore.getState().shuffle).toBe(true);
    for (let i = 0; i < 20; i++) {
      const before = useMusicQueueStore.getState().currentIndex;
      useMusicQueueStore.getState().next();
      const after = useMusicQueueStore.getState().currentIndex;
      expect(after).not.toBe(before);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    }
    const before = useMusicQueueStore.getState().currentIndex;
    useMusicQueueStore.getState().previous();
    expect(useMusicQueueStore.getState().currentIndex).not.toBe(before);

    useMusicQueueStore.setState({ queue: [tracks[0]], currentIndex: 0 });
    useMusicQueueStore.getState().next();
    expect(useMusicQueueStore.getState().currentIndex).toBe(0);
    useMusicQueueStore.getState().toggleShuffle();
    expect(useMusicQueueStore.getState().shuffle).toBe(false);
  });
});
