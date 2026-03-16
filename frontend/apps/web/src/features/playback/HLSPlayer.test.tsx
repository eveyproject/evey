import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useToastStore } from "@evey/store";

// HLS mock — captures event handlers so tests can fire them
type HlsEventHandler = (...args: unknown[]) => void;

const hlsEventListeners: Record<string, HlsEventHandler[]> = {};

class MockHls {
  static isSupported = vi.fn(() => true);
  static Events = {
    MANIFEST_PARSED: "hlsManifestParsed",
    AUDIO_TRACKS_UPDATED: "hlsAudioTracksUpdated",
    ERROR: "hlsError",
  };
  loadSource = vi.fn();
  attachMedia = vi.fn();
  destroy = vi.fn();
  audioTrack = 0;
  on = vi.fn((event: string, handler: HlsEventHandler) => {
    if (!hlsEventListeners[event]) hlsEventListeners[event] = [];
    hlsEventListeners[event].push(handler);
  });
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
      startPositionMs={0}
    />,
  );
}

// Lazy import after mocks are set up
let HLSPlayer: typeof import("./HLSPlayer").HLSPlayer;

beforeEach(async () => {
  for (const key of Object.keys(hlsEventListeners)) delete hlsEventListeners[key];
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

  it("starts in transcode mode (HLS path is not used)", () => {
    renderPlayer();
    // transcodeMode starts true, so HLS is never initialized
    // The video element should have a transcode src set
    const video = document.querySelector("video");
    expect(video).toBeTruthy();
    expect(video!.src).toContain("/api/v1/media/media-1/transcode");
  });
});
