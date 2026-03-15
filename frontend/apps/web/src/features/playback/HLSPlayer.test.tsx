import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useToastStore } from "@evey/store";

// HLS mock — captures event handlers so tests can fire them
type HlsEventHandler = (...args: unknown[]) => void;

const hlsEventListeners: Record<string, HlsEventHandler[]> = {};

function fireHlsEvent(event: string, ...args: unknown[]) {
  (hlsEventListeners[event] ?? []).forEach((h) => h(...args));
}

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
  // Clear event listeners
  for (const key of Object.keys(hlsEventListeners)) delete hlsEventListeners[key];
  vi.clearAllMocks();
  useToastStore.setState({ toasts: [] });
  // jsdom doesn't implement HTMLMediaElement.play — mock it globally
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

describe("HLSPlayer — quality fallback on fatal error", () => {
  it("switches to transcode mode and fires toast on HLS fatal error", async () => {
    renderPlayer();

    act(() => {
      fireHlsEvent("hlsError", {}, { fatal: true, type: "networkError", details: "fragLoadError" });
    });

    const toasts = useToastStore.getState().toasts;
    expect(toasts.length).toBeGreaterThan(0);
    expect(toasts[0].type).toBe("warning");
    expect(toasts[0].message).toMatch(/Quality reduced/i);
  });

  it("does not fire toast for non-fatal HLS errors", () => {
    renderPlayer();

    act(() => {
      fireHlsEvent("hlsError", {}, { fatal: false, type: "networkError", details: "fragLoadError" });
    });

    expect(useToastStore.getState().toasts).toHaveLength(0);
  });

  it("shows fatal error overlay after MAX_FALLBACK_ATTEMPTS exhausted", async () => {
    renderPlayer();

    // Fire 4 fatal errors (MAX_FALLBACK_ATTEMPTS = 3, 4th should show overlay)
    act(() => {
      for (let i = 0; i < 4; i++) {
        fireHlsEvent("hlsError", {}, { fatal: true, type: "networkError", details: "fragLoadError" });
      }
    });

    expect(screen.getByText("Playback failed")).toBeInTheDocument();
  });

  it("does not show fatal error overlay while fallbacks remain", () => {
    renderPlayer();

    act(() => {
      fireHlsEvent("hlsError", {}, { fatal: true, type: "networkError", details: "fragLoadError" });
    });

    expect(screen.queryByText("Playback failed")).not.toBeInTheDocument();
  });

  it("retry button reloads the page after fatal error", async () => {
    const reloadMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { reload: reloadMock },
      writable: true,
    });

    renderPlayer();

    act(() => {
      for (let i = 0; i < 4; i++) {
        fireHlsEvent("hlsError", {}, { fatal: true, type: "networkError", details: "fragLoadError" });
      }
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(reloadMock).toHaveBeenCalled();
  });
});
