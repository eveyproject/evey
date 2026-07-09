import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
  useWindowDimensions: () => ({ width: 1920, height: 1080 }),
  useTVEventHandler: () => {},
}));

// --- expo-video mock ---------------------------------------------------------
const playMock = vi.fn();
const pauseMock = vi.fn();
vi.mock("expo-video", () => ({
  useVideoPlayer: () => ({
    addListener: () => ({ remove: () => {} }),
    play: (...args: unknown[]) => playMock(...args),
    pause: (...args: unknown[]) => pauseMock(...args),
    playing: false,
    currentTime: 0,
    duration: 0,
  }),
  VideoView: () => React.createElement("div", { "data-testid": "video-view" }),
}));

// --- expo-router mock ---------------------------------------------------------
const replaceMock = vi.fn();
const backMock = vi.fn();
const localSearchParamsMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => replaceMock(...args), back: (...args: unknown[]) => backMock(...args) },
  useLocalSearchParams: () => localSearchParamsMock(),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", error: "#f33", overlay: "rgba(0,0,0,0.6)" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store mock -------------------------------------------------------------
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: any) => selector({ profileId: "prof-1" }),
}));

// --- @evey/api mock --------------------------------------------------------------
const getMediaItemMock = vi.fn();
const markAsWatchedMock = vi.fn();
const getWatchProgressMock = vi.fn();
vi.mock("@evey/api", () => ({
  useLibraryServiceGetMediaItem: (...args: unknown[]) => getMediaItemMock(...args),
  usePlaybackServiceMarkAsWatched: (...args: unknown[]) => markAsWatchedMock(...args),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => getWatchProgressMock(...args),
}));

// --- usePlaySession mock -----------------------------------------------------------
const stopSessionMock = vi.fn();
vi.mock("../../../src/hooks/usePlaySession", () => ({
  usePlaySession: () => ({
    streamUrl: "http://srv.test/stream.m3u8",
    sessionId: "sess-1",
    streamInfo: { mediaStreams: [] },
    isLoading: false,
    isError: false,
    stopSession: stopSessionMock,
    restartWithTracks: vi.fn(),
    restartAtPosition: vi.fn(),
    transcodeOffset: 0,
    progress: null,
  }),
}));

// --- misc hook mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useProgressReporter", () => ({
  useProgressReporter: () => {},
}));
vi.mock("../../../src/hooks/usePlaybackSettings", () => ({
  usePlaybackSettings: () => ({ data: { skipForwardSeconds: 30, skipBackwardSeconds: 30 } }),
}));

// --- useNextEpisode mock: captures the onNavigate callback the screen wires
// up so tests can simulate both the auto-advance countdown and the "play
// now" button firing, the two paths that must be PIN-gated. -------------------
let capturedOnNavigate: ((episodeId: string) => void) | null = null;
const nextEpisodeStateMock = vi.fn();
vi.mock("../../../src/hooks/useNextEpisode", () => ({
  useNextEpisode: (args: { onNavigate: (episodeId: string) => void }) => {
    capturedOnNavigate = args.onNavigate;
    return nextEpisodeStateMock();
  },
}));

// --- trivial component mocks --------------------------------------------------------
vi.mock("../../../src/components/QualityPicker", () => ({ QualityPicker: () => null }));
vi.mock("../../../src/components/AudioTrackPicker", () => ({ AudioTrackPicker: () => null }));
vi.mock("../../../src/components/SubtitlePicker", () => ({ SubtitlePicker: () => null }));
vi.mock("../../../src/components/TrackOptionsMenu", () => ({ TrackOptionsMenu: () => null }));
vi.mock("../../../src/components/SkipIntroButton", () => ({ SkipIntroButton: () => null }));
vi.mock("../../../src/components/NextEpisodeOverlay", () => ({ NextEpisodeOverlay: () => null }));

// --- ParentalPINModal mock -------------------------------------------------------
vi.mock("../../../src/components/ParentalPINModal", () => ({
  ParentalPINModal: ({ visible, onVerified, onClose }: any) =>
    visible
      ? React.createElement(
          "div",
          { "data-testid": "pin-modal" },
          React.createElement("button", { onClick: onVerified, "aria-label": "verify-pin" }, "verify"),
          React.createElement("button", { onClick: onClose, "aria-label": "cancel-pin" }, "cancel"),
        )
      : null,
}));

// --- useParentalGate mock ---------------------------------------------------------
// Rating-aware: the screen mounts TWO gates (the media being played, and the
// resolved next episode). Keying the mock on the rating lets each test drive
// exactly one of them.
const needsPinMock = vi.fn((_rating?: string) => false);
vi.mock("../../../src/hooks/useParentalGate", () => ({
  useParentalGate: (rating?: string) => ({ needsPin: () => needsPinMock(rating), profileId: "prof-1" }),
}));

import PlayerScreen from "./[id]";

beforeEach(() => {
  vi.clearAllMocks();
  capturedOnNavigate = null;
  needsPinMock.mockImplementation(() => false);
  localSearchParamsMock.mockReturnValue({ id: "ep-1" });
  getMediaItemMock.mockReturnValue({ data: { id: "ep-1", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Episode 1" } });
  markAsWatchedMock.mockReturnValue({ mutate: vi.fn() });
  getWatchProgressMock.mockReturnValue({ data: undefined });
  nextEpisodeStateMock.mockReturnValue({
    visible: false,
    ended: false,
    countdown: 5,
    nextEpisode: { id: "ep-2", officialRating: "R" },
    playNow: () => capturedOnNavigate?.("ep-2"),
    cancel: vi.fn(),
  });
});

describe("PlayerScreen next-episode parental gate (TV)", () => {
  it("advances to the next episode directly when no PIN is required", () => {
    render(<PlayerScreen />);
    act(() => { capturedOnNavigate?.("ep-2"); });
    expect(stopSessionMock).toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });

  it("blocks the auto-advance behind a PIN modal instead of navigating", () => {
    needsPinMock.mockImplementation((rating) => rating === "R"); // next episode only
    render(<PlayerScreen />);
    act(() => { capturedOnNavigate?.("ep-2"); });
    expect(replaceMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });

  it("navigates to the next episode only after the PIN is verified", () => {
    needsPinMock.mockImplementation((rating) => rating === "R"); // next episode only
    render(<PlayerScreen />);
    act(() => { capturedOnNavigate?.("ep-2"); });
    expect(replaceMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByLabelText("verify-pin"));
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");
  });

  it("also gates the manual 'play now' next-episode button", () => {
    needsPinMock.mockImplementation((rating) => rating === "R"); // next episode only
    render(<PlayerScreen />);
    // Simulates the NextEpisodeOverlay's onPlayNow, which routes through the
    // same onNavigate callback captured above.
    act(() => { capturedOnNavigate?.("ep-2"); });
    expect(replaceMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });

  it("does not navigate if the PIN modal is cancelled", () => {
    needsPinMock.mockImplementation((rating) => rating === "R"); // next episode only
    render(<PlayerScreen />);
    act(() => { capturedOnNavigate?.("ep-2"); });
    fireEvent.click(screen.getByLabelText("cancel-pin"));
    expect(replaceMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });
});

// The player is reachable without the media detail screen: the home hero, a
// deep link, and the remote "loadAndPlay" device command all land here
// directly. The screen itself must gate the media it is about to play.
describe("player entry gate", () => {
  beforeEach(() => {
    getMediaItemMock.mockReturnValue({
      data: { id: "ep-1", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Episode 1", officialRating: "NC-17" },
    });
    nextEpisodeStateMock.mockReturnValue({
      visible: false, ended: false, countdown: 5, nextEpisode: undefined, playNow: vi.fn(), cancel: vi.fn(),
    });
  });

  it("prompts for the PIN and pauses playback when the media itself is gated", () => {
    needsPinMock.mockImplementation((rating) => rating === "NC-17");
    render(<PlayerScreen />);
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
    expect(pauseMock).toHaveBeenCalled();
  });

  it("resumes playback once the PIN is verified", () => {
    needsPinMock.mockImplementation((rating) => rating === "NC-17");
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("verify-pin"));
    expect(playMock).toHaveBeenCalled();
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });

  it("leaves the player when the PIN prompt is cancelled", () => {
    needsPinMock.mockImplementation((rating) => rating === "NC-17");
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("cancel-pin"));
    expect(backMock).toHaveBeenCalled();
  });

  it("plays straight away when the media is not gated", () => {
    needsPinMock.mockImplementation(() => false);
    render(<PlayerScreen />);
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });
});
