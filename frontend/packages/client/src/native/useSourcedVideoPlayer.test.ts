import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useSourcedVideoPlayer, type PlayableVideoSource } from "./useSourcedVideoPlayer";

// Each created player is a fresh object with a release spy, so a test can tell
// which instance was released and when.
type FakePlayer = { id: number; release: ReturnType<typeof vi.fn> };
const created: FakePlayer[] = [];
const createVideoPlayer = vi.fn(() => {
  const p: FakePlayer = { id: created.length + 1, release: vi.fn() };
  created.push(p);
  return p;
});
vi.mock("expo-video", () => ({ createVideoPlayer: (...args: unknown[]) => createVideoPlayer(...(args as [])) }));

const SRC: PlayableVideoSource = { uri: "http://srv.test/master.m3u8", contentType: "hls" };

describe("useSourcedVideoPlayer", () => {
  beforeEach(() => {
    created.length = 0;
    createVideoPlayer.mockClear();
  });

  // The whole point: before the stream URL is known there is no native player,
  // so no listener can ever hear an empty ExoPlayer report its "end".
  it("creates no player while there is no source", () => {
    const { result, unmount } = renderHook(() => useSourcedVideoPlayer(null));
    expect(result.current).toBeNull();
    expect(createVideoPlayer).not.toHaveBeenCalled();
    unmount();
    expect(created).toHaveLength(0);
  });

  it("creates the player once the source arrives and runs setup on it", () => {
    const setup = vi.fn();
    const { result, rerender } = renderHook(
      ({ source }: { source: PlayableVideoSource | null }) => useSourcedVideoPlayer(source, setup),
      { initialProps: { source: null } },
    );
    expect(result.current).toBeNull();
    rerender({ source: SRC });
    expect(result.current).toBe(created[0]);
    expect(setup).toHaveBeenCalledTimes(1);
    expect(setup).toHaveBeenCalledWith(created[0]);
  });

  it("keeps the same player when the source is rebuilt with the same value", () => {
    const { result, rerender } = renderHook(
      ({ source }: { source: PlayableVideoSource | null }) => useSourcedVideoPlayer(source),
      { initialProps: { source: { ...SRC } } },
    );
    rerender({ source: { ...SRC } });
    rerender({ source: { ...SRC } });
    expect(created).toHaveLength(1);
    expect(result.current).toBe(created[0]);
    expect(created[0].release).not.toHaveBeenCalled();
  });

  it("replaces the player when the source changes and releases the previous one after the commit", () => {
    const { result, rerender } = renderHook(
      ({ source }: { source: PlayableVideoSource | null }) => useSourcedVideoPlayer(source),
      { initialProps: { source: SRC } },
    );
    rerender({ source: { uri: "http://srv.test/master.m3u8?audioTrack=2", contentType: "hls" } });
    expect(created).toHaveLength(2);
    expect(result.current).toBe(created[1]);
    expect(created[0].release).toHaveBeenCalledTimes(1);
    expect(created[1].release).not.toHaveBeenCalled();
  });

  it("releases the player when the source goes away, and hands back null", () => {
    const { result, rerender } = renderHook(
      ({ source }: { source: PlayableVideoSource | null }) => useSourcedVideoPlayer(source),
      { initialProps: { source: SRC } },
    );
    rerender({ source: null });
    expect(result.current).toBeNull();
    expect(created[0].release).toHaveBeenCalledTimes(1);
  });

  it("releases the player exactly once on unmount", () => {
    const { unmount } = renderHook(() => useSourcedVideoPlayer(SRC));
    unmount();
    expect(created).toHaveLength(1);
    expect(created[0].release).toHaveBeenCalledTimes(1);
  });
});
