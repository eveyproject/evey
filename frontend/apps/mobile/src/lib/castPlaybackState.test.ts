import { describe, it, expect } from "vitest";
import { reportedPlaybackState } from "./castPlaybackState";

// The local player is parked while casting: paused, volume 0, and (on some
// paths) muted. Every assertion below is about NOT reporting that.
const parkedLocal = { isPaused: true, volume: 0, isMuted: true };
const playingLocal = { isPaused: false, volume: 0.8, isMuted: false };

describe("reportedPlaybackState", () => {
  it("reports the local player when no cast session is connected", () => {
    expect(
      reportedPlaybackState(playingLocal, {
        connected: false,
        isPlaying: false,
        volume: undefined,
        isMuted: undefined,
      }),
    ).toEqual(playingLocal);
  });

  it("ignores the parked local player and reports the receiver while casting", () => {
    expect(
      reportedPlaybackState(parkedLocal, {
        connected: true,
        isPlaying: true,
        volume: 0.6,
        isMuted: false,
      }),
    ).toEqual({ isPaused: false, volume: 0.6, isMuted: false });
  });

  it("reports paused when the receiver is not playing", () => {
    const state = reportedPlaybackState(playingLocal, {
      connected: true,
      isPlaying: false,
      volume: 0.5,
      isMuted: false,
    });
    expect(state.isPaused).toBe(true);
  });

  it("reports the receiver's mute state, not the local one", () => {
    const state = reportedPlaybackState(parkedLocal, {
      connected: true,
      isPlaying: true,
      volume: 0.3,
      isMuted: true,
    });
    expect(state.isMuted).toBe(true);
    expect(state.volume).toBe(0.3);
  });

  it("falls back to full volume and unmuted before the first receiver status", () => {
    // The bug this guards: falling back to the LOCAL values here would report
    // volume 0 / muted for a television playing out loud.
    const state = reportedPlaybackState(parkedLocal, {
      connected: true,
      isPlaying: true,
      volume: undefined,
      isMuted: undefined,
    });
    expect(state).toEqual({ isPaused: false, volume: 1, isMuted: false });
  });

  it("does not report a muted receiver as unmuted just because volume is known", () => {
    const state = reportedPlaybackState(playingLocal, {
      connected: true,
      isPlaying: true,
      volume: 0,
      isMuted: true,
    });
    expect(state).toEqual({ isPaused: false, volume: 0, isMuted: true });
  });
});
