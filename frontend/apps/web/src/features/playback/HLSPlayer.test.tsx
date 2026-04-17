import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useToastStore } from "@evey/store";

// HLS mock — captures event handlers so tests can fire them
type HlsEventHandler = (...args: unknown[]) => void;

const hlsEventListeners: Record<string, HlsEventHandler[]> = {};
let lastHlsInstance: MockHls | null = null;

class MockHls {
  static isSupported = vi.fn(() => true);
  static Events = {
    MANIFEST_PARSED: "hlsManifestParsed",
    AUDIO_TRACKS_UPDATED: "hlsAudioTracksUpdated",
    ERROR: "hlsError",
  };
  static ErrorTypes = {
    NETWORK_ERROR: "networkError",
    MEDIA_ERROR: "mediaError",
    OTHER_ERROR: "otherError",
  };
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
  constructor() {
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

function renderPlayer(streamInfo = DEFAULT_STREAM_INFO) {
  return render(
    <HLSPlayer
      streamInfo={streamInfo as never}
      sessionId="session-1"
      mediaId="media-1"
      streamToken="test-token"
      onStreamTokenRefresh={() => Promise.resolve("new-token")}
      startPositionMs={0}
    />,
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

  it("starts in direct play mode when playMethod is DIRECT_STREAM", () => {
    renderPlayer({
      ...DEFAULT_STREAM_INFO,
      playMethod: "PLAY_METHOD_DIRECT_STREAM",
      directPlayUrl: "/api/v1/media/media-1/transcode?token=test&remux=1",
    });
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(video!.src).toContain("remux=1");
  });
});
