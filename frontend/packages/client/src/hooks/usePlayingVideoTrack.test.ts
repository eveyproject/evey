import { describe, it, expect } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { usePlayingVideoTrack, type VideoTrackSource } from "./usePlayingVideoTrack";

type Track = { size: { width: number; height: number }; peakBitrate: number; frameRate: number };

function fakePlayer(tracks: Track[], current: Track) {
  const listeners = new Map<string, () => void>();
  const player = {
    videoTrack: current as Track | null,
    availableVideoTracks: tracks,
    addListener(event: string, cb: () => void) {
      listeners.set(event, cb);
      return { remove: () => listeners.delete(event) };
    },
  };
  return { player: player as unknown as VideoTrackSource & { videoTrack: Track | null }, emit: (e: string) => listeners.get(e)?.(), listeners };
}

describe("usePlayingVideoTrack", () => {
  const top = { size: { width: 1920, height: 1080 }, peakBitrate: 8_000_000, frameRate: 24 };
  const low = { size: { width: 1280, height: 720 }, peakBitrate: 3_000_000, frameRate: 24 };

  it("follows the native player's ABR switches", () => {
    const { player, emit } = fakePlayer([top, low], top);
    const { result } = renderHook(() => usePlayingVideoTrack(player));
    expect(result.current.playing?.height).toBe(1080);
    expect(result.current.ladderBitrates).toEqual([8_000_000, 3_000_000]);

    act(() => {
      player.videoTrack = low;
      emit("videoTrackChange");
    });
    expect(result.current.playing).toEqual({ width: 1280, height: 720, bitrate: 3_000_000, frameRate: 24 });
  });

  it("stops listening when the player goes", () => {
    const { player, listeners } = fakePlayer([top], top);
    const { unmount } = renderHook(() => usePlayingVideoTrack(player));
    expect(listeners.size).toBe(2);
    unmount();
    expect(listeners.size).toBe(0);
  });
});
