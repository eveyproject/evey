import { describe, it, expect, beforeEach } from "vitest";
import { useMusicQueueStore } from "./musicQueue";
import type { QueueTrack } from "./musicQueue";

function makeTrack(id: string): QueueTrack {
  return { id, title: `Track ${id}`, artist: "Artist", album: "Album", duration: 180 };
}

const t1 = makeTrack("1");
const t2 = makeTrack("2");
const t3 = makeTrack("3");

beforeEach(() => {
  useMusicQueueStore.getState().clear();
});

describe("useMusicQueueStore", () => {
  describe("setQueue", () => {
    it("sets queue and starts playing from index 0 by default", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3]);
      const { queue, currentIndex, isPlaying } = useMusicQueueStore.getState();
      expect(queue).toEqual([t1, t2, t3]);
      expect(currentIndex).toBe(0);
      expect(isPlaying).toBe(true);
    });

    it("respects custom startIndex", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 2);
      expect(useMusicQueueStore.getState().currentIndex).toBe(2);
    });

    it("replaces existing queue", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3]);
      useMusicQueueStore.getState().setQueue([t3]);
      expect(useMusicQueueStore.getState().queue).toEqual([t3]);
    });
  });

  describe("playTrack", () => {
    it("sets currentIndex and starts playing", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3]);
      useMusicQueueStore.getState().playTrack(1);
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    });

    it("ignores out-of-bounds negative index", () => {
      useMusicQueueStore.getState().setQueue([t1, t2]);
      useMusicQueueStore.getState().playTrack(0);
      useMusicQueueStore.getState().playTrack(-1);
      expect(useMusicQueueStore.getState().currentIndex).toBe(0);
    });

    it("ignores out-of-bounds high index", () => {
      useMusicQueueStore.getState().setQueue([t1, t2]);
      useMusicQueueStore.getState().playTrack(0);
      useMusicQueueStore.getState().playTrack(5);
      expect(useMusicQueueStore.getState().currentIndex).toBe(0);
    });
  });

  describe("next", () => {
    it("advances to next track", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3]);
      useMusicQueueStore.getState().next();
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    });

    it("stays on the last track and marks the player stopped", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 2);
      useMusicQueueStore.getState().next();
      expect(useMusicQueueStore.getState().currentIndex).toBe(2);
      // The album is over: a pause button over silence would need two
      // presses to play again.
      expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    });

    it("does nothing on empty queue", () => {
      useMusicQueueStore.getState().next();
      expect(useMusicQueueStore.getState().currentIndex).toBe(-1);
    });
  });

  describe("previous", () => {
    it("goes back to previous track", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 2);
      useMusicQueueStore.getState().previous();
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    });

    it("does nothing at first track", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 0);
      useMusicQueueStore.getState().previous();
      expect(useMusicQueueStore.getState().currentIndex).toBe(0);
    });

    it("does nothing on empty queue", () => {
      useMusicQueueStore.getState().previous();
      expect(useMusicQueueStore.getState().currentIndex).toBe(-1);
    });
  });

  describe("togglePlay", () => {
    it("toggles isPlaying from true to false", () => {
      useMusicQueueStore.getState().setQueue([t1]);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
      useMusicQueueStore.getState().togglePlay();
      expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    });

    it("toggles isPlaying from false to true", () => {
      useMusicQueueStore.getState().togglePlay();
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    });
  });

  describe("clear", () => {
    it("resets all state", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 1);
      useMusicQueueStore.getState().clear();
      const { queue, currentIndex, isPlaying } = useMusicQueueStore.getState();
      expect(queue).toEqual([]);
      expect(currentIndex).toBe(-1);
      expect(isPlaying).toBe(false);
    });
  });

  describe("removeFromQueue", () => {
    it("removes a track by index", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 0);
      useMusicQueueStore.getState().removeFromQueue(1);
      expect(useMusicQueueStore.getState().queue).toEqual([t1, t3]);
    });

    it("adjusts currentIndex down when removing track before current", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 2);
      useMusicQueueStore.getState().removeFromQueue(0);
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
    });

    it("keeps currentIndex when removing track after current", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 0);
      useMusicQueueStore.getState().removeFromQueue(2);
      expect(useMusicQueueStore.getState().currentIndex).toBe(0);
    });

    it("clamps currentIndex when removing currently playing track (not last)", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 1);
      useMusicQueueStore.getState().removeFromQueue(1);
      // newIndex = min(1, 1) = 1 (t3 is now at index 1)
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
      expect(useMusicQueueStore.getState().queue).toEqual([t1, t3]);
    });

    it("sets currentIndex to 0 when removing last playing track", () => {
      useMusicQueueStore.getState().setQueue([t1, t2, t3], 2);
      useMusicQueueStore.getState().removeFromQueue(2);
      // newIndex = min(2, 1) = 1
      expect(useMusicQueueStore.getState().currentIndex).toBe(1);
    });

    it("sets currentIndex to -1 when queue becomes empty", () => {
      useMusicQueueStore.getState().setQueue([t1], 0);
      useMusicQueueStore.getState().removeFromQueue(0);
      expect(useMusicQueueStore.getState().currentIndex).toBe(-1);
      expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    });

    it("sets isPlaying to false when queue becomes empty", () => {
      useMusicQueueStore.getState().setQueue([t1], 0);
      useMusicQueueStore.getState().removeFromQueue(0);
      expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    });

    it("keeps isPlaying true when queue still has tracks", () => {
      useMusicQueueStore.getState().setQueue([t1, t2], 0);
      useMusicQueueStore.getState().removeFromQueue(1);
      expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    });
  });
});
