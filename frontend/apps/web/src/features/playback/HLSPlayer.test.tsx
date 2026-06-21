import { render, screen, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useToastStore } from "@evey/store";

// HLS mock — captures event handlers so tests can fire them
type HlsEventHandler = (...args: unknown[]) => void;

const hlsEventListeners: Record<string, HlsEventHandler[]> = {};
let lastHlsInstance: MockHls | null = null;

interface MockHlsConfig {
  xhrSetup?: (xhr: XMLHttpRequest, url: string) => void;
  startPosition?: number;
  maxBufferLength?: number;
  maxMaxBufferLength?: number;
}

class MockHls {
  static isSupported = vi.fn(() => true);
  static Events = {
    MANIFEST_PARSED: "hlsManifestParsed",
    AUDIO_TRACKS_UPDATED: "hlsAudioTracksUpdated",
    ERROR: "hlsError",
    FRAG_LOADED: "hlsFragLoaded",
  };
  static ErrorTypes = {
    NETWORK_ERROR: "networkError",
    MEDIA_ERROR: "mediaError",
    OTHER_ERROR: "otherError",
  };
  config: MockHlsConfig;
  loadSource = vi.fn();
  attachMedia = vi.fn();
  destroy = vi.fn();
  startLoad = vi.fn();
  audioTrack = 0;
  currentLevel = -1;
  levels: { height: number }[] = [];
  on = vi.fn((event: string, handler: HlsEventHandler) => {
    if (!hlsEventListeners[event]) hlsEventListeners[event] = [];
    hlsEventListeners[event].push(handler);
  });
  constructor(config: MockHlsConfig = {}) {
    this.config = config;
    lastHlsInstance = this;
  }
}

vi.mock("hls.js", () => ({ default: MockHls }));

vi.mock("@evey/store", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@evey/store")>();
  return {
    ...actual,
    useAuthStore: vi.fn((selector: (s: { accessToken: string | null }) => unknown) =>
      selector({ accessToken: "test-token" }),
    ),
  };
});

vi.mock("@/hooks/usePlaybackSettings", () => ({
  usePlaybackSettings: vi.fn(() => ({ data: { skipForwardSeconds: 10, skipBackwardSeconds: 10 } })),
}));

vi.mock("@/hooks/usePlaybackPrefs", () => ({
  getPreferredAudioLang: vi.fn(() => null),
  setPreferredAudioLang: vi.fn(),
  getPreferredSubtitleLang: vi.fn(() => null),
  setPreferredSubtitleLang: vi.fn(),
  getSeriesTrackPrefs: vi.fn(() => null),
  setSeriesTrackPrefs: vi.fn(),
  getSubtitleStyle: vi.fn(() => ({
    fontSize: 100,
    color: "#ffffff",
    fontFamily: "",
    bgColor: "#000000",
    bgOpacity: 50,
    shadow: true,
    position: "bottom",
  })),
  setSubtitleStyle: vi.fn(),
}));

vi.mock("@/utils/codec", () => ({
  canPlayHevc: false,
  getBrowserCapabilities: () => ({
    videoCodecs: ["h264"],
    audioCodecs: ["aac", "mp3", "opus"],
    containers: ["mp4"],
  }),
}));

vi.mock("./PlayerControls", () => ({
  PlayerControls: () => <div data-testid="player-controls" />,
}));

vi.mock("./ProgressReporter", () => ({
  ProgressReporter: () => null,
}));

vi.mock("./AutoMarkWatched", () => ({
  AutoMarkWatched: () => null,
}));

vi.mock("./NextEpisodeOverlay", () => ({
  NextEpisodeOverlay: () => null,
}));

vi.mock("./SkipIntroButton", () => ({
  SkipIntroButton: () => null,
}));

vi.mock("./KeyboardShortcutsOverlay", () => ({
  KeyboardShortcutsOverlay: () => null,
}));

const DEFAULT_STREAM_INFO = {
  transcodeUrl: "/hls/stream.m3u8",
  directPlayUrl: null,
  mediaStreams: [],
};

function renderPlayer(streamInfo = DEFAULT_STREAM_INFO, opts: { onStreamTokenRefresh?: () => Promise<string>; streamToken?: string; startPositionMs?: number } = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <HLSPlayer
        streamInfo={streamInfo as never}
        sessionId="session-1"
        mediaId="media-1"
        streamToken={opts.streamToken ?? "test-token"}
        onStreamTokenRefresh={opts.onStreamTokenRefresh ?? (() => Promise.resolve("new-token"))}
        startPositionMs={opts.startPositionMs ?? 0}
      />
    </QueryClientProvider>,
  );
}

// Lazy import after mocks are set up
let HLSPlayer: typeof import("./HLSPlayer").HLSPlayer;

beforeEach(async () => {
  for (const key of Object.keys(hlsEventListeners)) delete hlsEventListeners[key];
  lastHlsInstance = null;
  vi.clearAllMocks();
  useToastStore.setState({ toasts: [] });
  Object.defineProperty(HTMLMediaElement.prototype, "play", {
    configurable: true,
    writable: true,
    value: vi.fn().mockResolvedValue(undefined),
  });
  const mod = await import("./HLSPlayer");
  HLSPlayer = mod.HLSPlayer;
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("HLSPlayer", () => {
  it("renders player controls", () => {
    renderPlayer();
    expect(screen.getByTestId("player-controls")).toBeInTheDocument();
  });

  it("does not show fatal error overlay initially", () => {
    renderPlayer();
    expect(screen.queryByText("Playback failed")).not.toBeInTheDocument();
  });

  it("starts in transcode mode with hls.js when playMethod is TRANSCODE", () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" });
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(lastHlsInstance).toBeTruthy();
    expect(lastHlsInstance!.loadSource).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/media/media-1/hls/master.m3u8"),
    );
    expect(lastHlsInstance!.attachMedia).toHaveBeenCalled();
  });

  it("starts in transcode mode with hls.js when playMethod is undefined", () => {
    renderPlayer();
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(lastHlsInstance).toBeTruthy();
    expect(lastHlsInstance!.loadSource).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/media/media-1/hls/master.m3u8"),
    );
  });

  it("starts in direct play mode when playMethod is DIRECT_PLAY", () => {
    renderPlayer({
      ...DEFAULT_STREAM_INFO,
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
      directPlayUrl: "/api/v1/media/media-1/stream?token=test",
    });
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(video!.src).toContain("/api/v1/media/media-1/stream");
  });

  // direct_stream is a container-only remux: the server's HLS copy rung
  // repackages the original streams with `-c:v copy`. It must run through the
  // HLS pipeline (hls.js loading the master), NOT play the original file
  // progressively — the original container is exactly what the client can't
  // demux. Regression guard for the formerly no-op direct_stream branch.
  it("starts in transcode/HLS mode when playMethod is DIRECT_STREAM", () => {
    renderPlayer({
      ...DEFAULT_STREAM_INFO,
      playMethod: "PLAY_METHOD_DIRECT_STREAM",
      transcodeUrl: "/api/v1/media/media-1/hls/master.m3u8",
      directPlayUrl: null,
    });
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(lastHlsInstance).toBeTruthy();
    expect(lastHlsInstance!.loadSource).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/media/media-1/hls/master.m3u8"),
    );
    expect(lastHlsInstance!.attachMedia).toHaveBeenCalled();
  });
});

// Regression: on the native-HLS path (Safari/iOS, where Hls.isSupported() is
// false) the transcode branch used to hard-seek to 0, so resume-from-watch-
// progress and audio/subtitle restarts jumped back to the start. The resume
// target must be applied once metadata is loaded.
describe("HLSPlayer: native HLS (Safari) resume", () => {
  it("seeks to the resume position on loadedmetadata instead of restarting at 0", () => {
    MockHls.isSupported.mockReturnValue(false);
    try {
      renderPlayer(
        { ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" },
        { startPositionMs: 600_000 },
      );
      const video = document.querySelector("video")!;
      let ct = 0;
      Object.defineProperty(video, "currentTime", {
        configurable: true,
        get: () => ct,
        set: (v: number) => { ct = v; },
      });
      // Native path attaches a loadedmetadata listener; firing it must apply
      // the 600s resume target, not leave currentTime at 0.
      video.dispatchEvent(new Event("loadedmetadata"));
      expect(video.currentTime).toBe(600);
    } finally {
      MockHls.isSupported.mockReturnValue(true);
    }
  });
});

// Regression: a stream token has a 5-min sliding TTL on the server. After a
// long pause (5 min, 2 h, 6 h…), the token baked into the m3u8 segment URIs
// expires; resume returns 401. The 401 retry path must refresh the token and
// every subsequent fragment fetch must use the fresh value, not the stale one
// still embedded in cached URLs.
describe("HLSPlayer: 401 token refresh after long pause", () => {
  function fireFatal401() {
    const handlers = hlsEventListeners[MockHls.Events.ERROR] ?? [];
    for (const h of handlers) {
      h(MockHls.Events.ERROR, {
        fatal: true,
        type: MockHls.ErrorTypes.NETWORK_ERROR,
        details: "fragLoadError",
        response: { code: 401 },
      });
    }
  }

  function callXhrSetup(url: string): { method: string; finalUrl: string } | null {
    const setup = lastHlsInstance?.config.xhrSetup;
    if (!setup) return null;
    let captured: { method: string; finalUrl: string } | null = null;
    const xhr = {
      open: (method: string, finalUrl: string) => { captured = { method, finalUrl }; },
    } as unknown as XMLHttpRequest;
    setup(xhr, url);
    return captured;
  }

  it("xhrSetup overrides a stale token already present in the URL with the current ref value", () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "fresh-token" });
    expect(lastHlsInstance).toBeTruthy();
    const out = callXhrSetup("/api/v1/media/media-1/hls/seg_0.ts?token=stale-token");
    expect(out).not.toBeNull();
    expect(out!.finalUrl).toContain("token=fresh-token");
    expect(out!.finalUrl).not.toContain("token=stale-token");
  });

  it("xhrSetup injects token when URL has no query string", () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "fresh-token" });
    const out = callXhrSetup("/api/v1/media/media-1/hls/seg_0.ts");
    expect(out!.finalUrl).toContain("token=fresh-token");
  });

  it("xhrSetup preserves other query params when overriding token", () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "fresh-token" });
    const out = callXhrSetup("/api/v1/media/media-1/hls/seg_0.ts?token=stale&foo=bar");
    expect(out!.finalUrl).toContain("token=fresh-token");
    expect(out!.finalUrl).toContain("foo=bar");
  });

  it("on 401 fragment error, refreshes the token then resumes loading with the new token", async () => {
    const refresh = vi.fn().mockResolvedValue("refreshed-token");
    renderPlayer(
      { ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" },
      { streamToken: "stale-token", onStreamTokenRefresh: refresh },
    );

    // Fire the 401 — the refresh handler is async, so wait a microtask cycle.
    fireFatal401();
    await Promise.resolve();
    await Promise.resolve();

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(lastHlsInstance!.startLoad).toHaveBeenCalled();

    // The CRITICAL assertion: subsequent fragment xhr must use the NEW token,
    // even when the URL handed to xhrSetup still carries the stale baked-in
    // token from the cached m3u8.
    const out = callXhrSetup("/api/v1/media/media-1/hls/seg_5.ts?token=stale-token");
    expect(out!.finalUrl).toContain("token=refreshed-token");
    expect(out!.finalUrl).not.toContain("token=stale-token");
  });

  it("on a chain of 401 then refresh failure, falls into fatal-error overlay", async () => {
    const refresh = vi.fn().mockRejectedValue(new Error("refresh failed"));
    renderPlayer(
      { ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" },
      { streamToken: "stale-token", onStreamTokenRefresh: refresh },
    );

    fireFatal401();
    // Wait for the rejected refresh promise to settle and React to flush the
    // setFatalError state update — findByText retries with a microtask cycle.
    expect(await screen.findByText("Unable to play at any quality level")).toBeInTheDocument();
    expect(refresh).toHaveBeenCalledTimes(1);
  });
});

// Playability fallback: when transcode playback fails irrecoverably (two
// consecutive 404s exhaust the recovery path → fatalError), the error overlay
// must offer a download of the original file so the user isn't dead-ended.
describe("HLSPlayer: download fallback on fatal playback error", () => {
  function fireFatal404() {
    const handlers = hlsEventListeners[MockHls.Events.ERROR] ?? [];
    for (const h of handlers) {
      h(MockHls.Events.ERROR, {
        fatal: true,
        type: MockHls.ErrorTypes.NETWORK_ERROR,
        details: "fragLoadError",
        response: { code: 404 },
      });
    }
  }

  it("shows a download link pointing at the original file once playback fatals", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    expect(lastHlsInstance).toBeTruthy();

    // Two consecutive 404s trip the fatalError branch.
    act(() => { fireFatal404(); });
    act(() => { fireFatal404(); });

    const link = await screen.findByTestId("download-btn");
    expect(link).toHaveAttribute("href", "/api/v1/media/media-1/download?token=test-token");
  });
});
