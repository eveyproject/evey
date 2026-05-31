import { renderHook } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useMediaSession, type MediaSessionParams } from "./useMediaSession";

type Handlers = Partial<Record<string, ((d: { seekTime?: number; seekOffset?: number }) => void) | null>>;

function installFakeMediaSession() {
  const handlers: Handlers = {};
  const fake = {
    metadata: null as unknown,
    playbackState: "none" as string,
    setActionHandler: (action: string, handler: Handlers[string]) => {
      handlers[action] = handler;
    },
    setPositionState: vi.fn(),
  };
  Object.defineProperty(navigator, "mediaSession", { value: fake, configurable: true });
  // MediaMetadata is not in jsdom — provide a capturing stub.
  vi.stubGlobal(
    "MediaMetadata",
    class {
      title?: string;
      artwork?: Array<{ src: string }>;
      constructor(init: { title?: string; artwork?: Array<{ src: string }> }) {
        this.title = init.title;
        this.artwork = init.artwork;
      }
    },
  );
  return { fake, handlers };
}

function createVideo(currentTime = 0) {
  const el = document.createElement("video");
  Object.defineProperty(el, "currentTime", { value: currentTime, writable: true });
  el.play = vi.fn().mockResolvedValue(undefined);
  el.pause = vi.fn();
  return el;
}

function params(over: Partial<MediaSessionParams> = {}): MediaSessionParams {
  return {
    videoRef: { current: createVideo() },
    title: "Episode 1",
    artworkUrl: "/api/v1/media/mi-1/artwork/poster",
    playing: true,
    position: 50,
    duration: 1200,
    playbackRate: 1,
    transcodeOffset: 0,
    skipForwardSeconds: 30,
    skipBackwardSeconds: 10,
    ...over,
  };
}

describe("useMediaSession", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    // @ts-expect-error cleanup the injected property between tests
    delete navigator.mediaSession;
    vi.unstubAllGlobals();
  });

  it("publishes title + artwork metadata", () => {
    const { fake } = installFakeMediaSession();
    renderHook(() => useMediaSession(params()));
    expect((fake.metadata as { title: string }).title).toBe("Episode 1");
    expect((fake.metadata as { artwork: Array<{ src: string }> }).artwork[0].src).toBe(
      "/api/v1/media/mi-1/artwork/poster",
    );
  });

  it("binds play/pause/seek handlers and reflects playback state", () => {
    const { fake, handlers } = installFakeMediaSession();
    const video = createVideo(100);
    renderHook(() => useMediaSession(params({ videoRef: { current: video } })));

    handlers.play?.({});
    expect(video.play).toHaveBeenCalled();
    handlers.pause?.({});
    expect(video.pause).toHaveBeenCalled();
    expect(fake.playbackState).toBe("playing");
  });

  it("seekforward/seekbackward use the skip sizes", () => {
    const { handlers } = installFakeMediaSession();
    const video = createVideo(100);
    renderHook(() => useMediaSession(params({ videoRef: { current: video } })));

    handlers.seekforward?.({});
    expect(video.currentTime).toBe(130); // +30
    handlers.seekbackward?.({});
    expect(video.currentTime).toBe(120); // -10
  });

  it("seekto subtracts the transcodeOffset (absolute → element clock)", () => {
    const { handlers } = installFakeMediaSession();
    const video = createVideo(0);
    renderHook(() =>
      useMediaSession(params({ videoRef: { current: video }, transcodeOffset: 300 })),
    );
    handlers.seekto?.({ seekTime: 500 });
    expect(video.currentTime).toBe(200); // 500 - 300
  });

  it("wires nexttrack/previoustrack to neighbouring episodes", () => {
    const { handlers } = installFakeMediaSession();
    const onPlayEpisode = vi.fn();
    const episodes = [{ id: "ep1" }, { id: "ep2" }, { id: "ep3" }];
    renderHook(() =>
      useMediaSession(
        params({ episodes, currentEpisodeId: "ep2", onPlayEpisode }),
      ),
    );
    handlers.nexttrack?.({});
    expect(onPlayEpisode).toHaveBeenCalledWith("ep3");
    handlers.previoustrack?.({});
    expect(onPlayEpisode).toHaveBeenCalledWith("ep1");
  });

  it("leaves prev/next unbound at the edges of the episode list", () => {
    const { handlers } = installFakeMediaSession();
    const onPlayEpisode = vi.fn();
    const episodes = [{ id: "ep1" }, { id: "ep2" }];
    renderHook(() =>
      useMediaSession(
        params({ episodes, currentEpisodeId: "ep1", onPlayEpisode }),
      ),
    );
    // ep1 is first → no previoustrack handler bound.
    expect(handlers.previoustrack ?? null).toBeNull();
    handlers.nexttrack?.({});
    expect(onPlayEpisode).toHaveBeenCalledWith("ep2");
  });

  it("publishes a clamped position state", () => {
    const { fake } = installFakeMediaSession();
    renderHook(() => useMediaSession(params({ position: 2000, duration: 1200 })));
    expect(fake.setPositionState).toHaveBeenCalledWith({
      duration: 1200,
      position: 1200, // clamped to duration
      playbackRate: 1,
    });
  });

  it("is a no-op when Media Session is unsupported", () => {
    // navigator.mediaSession not installed → must not throw.
    expect(() => renderHook(() => useMediaSession(params()))).not.toThrow();
  });
});
