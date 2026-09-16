import { render, screen, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useToastStore } from "@evey/store";

// HLS mock - captures event handlers so tests can fire them
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
    SUBTITLE_TRACKS_UPDATED: "hlsSubtitleTracksUpdated",
    CUES_PARSED: "hlsCuesParsed",
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
  subtitleTrack = -1;
  subtitleDisplay = false;
  subtitleTracks: unknown[] = [];
  currentLevel = -1;
  startLevel = -1;
  levels: { height: number }[] = [];
  on = vi.fn((event: string, handler: HlsEventHandler) => {
    if (!hlsEventListeners[event]) hlsEventListeners[event] = [];
    hlsEventListeners[event].push(handler);
  });
  off = vi.fn((event: string, handler: HlsEventHandler) => {
    hlsEventListeners[event] = (hlsEventListeners[event] ?? []).filter((h) => h !== handler);
  });
  constructor(config: MockHlsConfig = {}) {
    this.config = config;
    // Tests need a handle on the constructed mock instance to fire captured
    // event listeners later.
    // oxlint-disable-next-line no-this-alias
    lastHlsInstance = this;
  }
}

vi.mock("hls.js", () => ({ default: MockHls }));

// The transcode branch routes on prefersNativeHLS() (4ced99e1b), not on
// Hls.isSupported() - jsdom's canPlayType returns "" so the real helper always
// says false. Overridable so the native-HLS (Safari) tests can enter the branch.
const { prefersNativeHLSMock } = vi.hoisted(() => ({ prefersNativeHLSMock: vi.fn(() => false) }));
vi.mock("./hooks/hlsPlayerUtils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./hooks/hlsPlayerUtils")>()),
  prefersNativeHLS: () => prefersNativeHLSMock(),
}));

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
  getVolumeBoost: vi.fn(() => 1),
  setVolumeBoost: vi.fn(),
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
  canPlayHevc: () => false,
  canWasmDecodeAC3: () => false,
  isWasmDecodableAudioCodec: () => false,
  getBrowserCapabilities: () => ({
    videoCodecs: ["h264"],
    audioCodecs: ["aac", "mp3", "opus"],
    containers: ["mp4"],
  }),
}));

vi.mock("./PlayerControls", () => ({
  PlayerControls: () => <div data-testid="player-controls" />,
}));

// Captured so a test can drive the admin-message path the reporter feeds.
let lastProgressReporterProps: { onRemoteMessage?: (message: string) => void } | null = null;
vi.mock("./ProgressReporter", () => ({
  ProgressReporter: (props: { onRemoteMessage?: (message: string) => void }) => {
    lastProgressReporterProps = props;
    return null;
  },
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
  // Half of these tests drive hls.js into a fatal error on purpose; the
  // player's DEV-only warning for it is the expected outcome, not noise worth
  // a stderr block per test. Everything else console.warn says still shows.
  const realWarn = console.warn.bind(console);
  vi.spyOn(console, "warn").mockImplementation((...args: unknown[]) => {
    if (args[0] === "[player] HLS fatal error:") return;
    realWarn(...args);
  });
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

  it("draws an admin message inside the player container (visible in fullscreen) for 12 s", () => {
    vi.useFakeTimers();
    try {
      const { container } = renderPlayer();
      act(() => { lastProgressReporterProps?.onRemoteMessage?.("Dinner in 5"); });
      const banner = screen.getByTestId("admin-message");
      expect(banner).toHaveTextContent("Dinner in 5");
      // Outside the fullscreen element the browser never paints it.
      expect(container.querySelector("[data-player-container]")).toContainElement(banner);
      // No global toast: it would land outside the container.
      expect(useToastStore.getState().toasts).toHaveLength(0);

      act(() => { vi.advanceTimersByTime(11_000); });
      expect(screen.getByTestId("admin-message")).toBeInTheDocument();
      act(() => { vi.advanceTimersByTime(1_000); });
      expect(screen.queryByTestId("admin-message")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
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
  // progressively - the original container is exactly what the client can't
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
    prefersNativeHLSMock.mockReturnValue(true);
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
      prefersNativeHLSMock.mockReturnValue(false);
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

    // Fire the 401 - the refresh handler is async, so wait a microtask cycle.
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
    // setFatalError state update - findByText retries with a microtask cycle.
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

// Server-side load shed (HTTP 503): the server refuses to start a NEW stream
// for two unrelated reasons that must read as unrelated to a non-technical
// user - "too many streams" (other playbacks saturate the box) vs "this file
// isn't ready yet" (its own seek index is still building). Neither is the
// generic "something is broken" fatalError, and a single 503 must not get
// chewed up by the 20-retry generic backoff.
describe("HLSPlayer: server load-shed (503) overlays", () => {
  function fireFatal503(body?: unknown) {
    const handlers = hlsEventListeners[MockHls.Events.ERROR] ?? [];
    for (const h of handlers) {
      h(MockHls.Events.ERROR, {
        fatal: true,
        type: MockHls.ErrorTypes.NETWORK_ERROR,
        details: "manifestLoadError",
        response: {
          code: 503,
          // hls.js's default loader doesn't actually forward the body under
          // `text` on error (see the extractShedReason comment) - this test
          // covers the "custom/future loader forwards it here" path directly,
          // independently of the extraction fallback covered below.
          ...(body !== undefined ? { text: JSON.stringify(body) } : {}),
        },
      });
    }
  }

  it("shows the 'too many streams' overlay for a 503 carrying that code, and a single fatal event doesn't trip the generic fatalError path", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    act(() => { fireFatal503({ code: "too_many_streams", retryAfterSec: 5, message: "shed: too many concurrent streams" }); });

    expect(await screen.findByText("Too many streams right now")).toBeInTheDocument();
    expect(screen.getByText(/can't prepare another video/i)).toBeInTheDocument();
    // The technical `message` field must never reach the UI.
    expect(screen.queryByText(/shed: too many concurrent streams/i)).not.toBeInTheDocument();
    expect(screen.queryByText("Playback failed")).not.toBeInTheDocument();
  });

  it("shows the 'file is getting ready' overlay for a 503 carrying that code", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    act(() => { fireFatal503({ code: "file_not_ready", retryAfterSec: 10, message: "shed: seek index building" }); });

    expect(await screen.findByText("This video is getting ready")).toBeInTheDocument();
    expect(screen.getByText(/still being prepared/i)).toBeInTheDocument();
  });

  it("falls back to the 'too many streams' overlay when the 503 body can't be parsed", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    // No body at all (the common real-world case with hls.js's default loader).
    act(() => { fireFatal503(); });

    expect(await screen.findByText("Too many streams right now")).toBeInTheDocument();
  });

  it("retrying calls hls.startLoad() to resume the live instance instead of reloading the page", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    act(() => { fireFatal503({ code: "too_many_streams" }); });
    await screen.findByText("Too many streams right now");

    act(() => { screen.getByText("Retry").click(); });

    expect(lastHlsInstance!.startLoad).toHaveBeenCalled();
    expect(screen.queryByText("Too many streams right now")).not.toBeInTheDocument();
  });

  it("a transient 503 on a FRAGMENT retries instead of showing a shed overlay or failing", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    // A deep seek's audio rebase encoder is still spinning up: the segment
    // handler returns a plain 503 ("not yet ready, retry") with NO shed body and
    // NO X-Evey-Error header. hls.js reports it as a fatal fragLoadError - it
    // must NOT be treated as a capacity shed, it must retry.
    const handlers = hlsEventListeners[MockHls.Events.ERROR] ?? [];
    act(() => {
      for (const h of handlers) {
        h(MockHls.Events.ERROR, {
          fatal: true,
          type: MockHls.ErrorTypes.NETWORK_ERROR,
          details: "fragLoadError",
          response: { code: 503 },
        });
      }
    });
    expect(screen.queryByText("Too many streams right now")).not.toBeInTheDocument();
    expect(screen.queryByText("This video is getting ready")).not.toBeInTheDocument();
    expect(screen.queryByText("Playback failed")).not.toBeInTheDocument();
  });

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

  it("a plain 404 still follows the existing fatalError path, not the shed overlay", async () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" }, { streamToken: "test-token" });
    act(() => { fireFatal404(); });
    act(() => { fireFatal404(); });

    expect(await screen.findByTestId("download-btn")).toBeInTheDocument();
    expect(screen.queryByText("Too many streams right now")).not.toBeInTheDocument();
    expect(screen.queryByText("This video is getting ready")).not.toBeInTheDocument();
  });
});

// Regression guard for the "buffering on start": the rung MUST be pinned
// during MANIFEST_PARSED, not on the next re-render. The pin lived in a
// useEffect depending on state set by that same handler, so hls.js had time to
// start on free ABR and fetch level 0 - a SECOND ffmpeg on the same hash, on a
// host that only sustains one.
describe("HLSPlayer: transcode rung pinning", () => {
  it("pins the top rung DURING MANIFEST_PARSED, before any fragment can load", () => {
    renderPlayer({ ...DEFAULT_STREAM_INFO, playMethod: "PLAY_METHOD_TRANSCODE" });
    const hls = lastHlsInstance!;
    hls.levels = [{ height: 480 }, { height: 720 }, { height: 1080 }];

    // Deliberately OUTSIDE act(): we want the state left by the handler itself,
    // before React has flushed anything. React's "not wrapped in act" warning
    // is the expected side effect of that choice, so it is muted here only.
    const muted = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      for (const h of hlsEventListeners["hlsManifestParsed"] ?? []) h();
    } finally {
      muted.mockRestore();
    }

    expect(hls.currentLevel).toBe(2);
    // startLoad picks the FIRST playlist from startLevel, not currentLevel: left
    // at -1 it fetched level 0 and the server opened a 480p rafale nobody read.
    expect(hls.startLevel).toBe(2);
  });
});
