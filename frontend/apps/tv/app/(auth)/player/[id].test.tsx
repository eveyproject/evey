import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
// useTVEventHandler captures the screen's own callback so tests can simulate a
// remote keypress directly, the same way playerListeners lets a test simulate
// a native player event.
let capturedTVHandler: ((evt: { eventType: string }) => void) | null = null;
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
  useWindowDimensions: () => ({ width: 1920, height: 1080 }),
  useTVEventHandler: (cb: (evt: { eventType: string }) => void) => { capturedTVHandler = cb; },
}));

// --- expo-video mock ---------------------------------------------------------
// Mirrors the real hook's contract where the tests depend on it: the player is
// recreated (and the setup callback re-runs) only when the source identity
// changes, not on every render - that recreation is exactly what the
// track-change tests below exercise. Listeners are captured so a test can
// deliver native events (statusChange with an error payload) to the screen.
const playMock = vi.fn();
const pauseMock = vi.fn();
const playerListeners = new Map<string, (payload?: unknown) => void>();
const playerMock = {
  addListener: (name: string, cb: (payload?: unknown) => void) => {
    playerListeners.set(name, cb);
    return { remove: () => {} };
  },
  play: (...args: unknown[]) => playMock(...args),
  pause: (...args: unknown[]) => pauseMock(...args),
  playing: false,
  currentTime: 0,
  duration: 0,
  timeUpdateEventInterval: 0,
  seekBy: vi.fn(),
};
let lastSourceUri: string | null = null;
vi.mock("expo-video", () => ({
  useVideoPlayer: (source: { uri?: string } | null, setup?: (p: unknown) => void) => {
    const uri = source?.uri ?? null;
    if (uri !== lastSourceUri) {
      lastSourceUri = uri;
      setup?.(playerMock);
    }
    return playerMock;
  },
  VideoView: () => React.createElement("div", { "data-testid": "video-view" }),
}));

// --- react-i18next mock ----------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- @expo/vector-icons mock ---------------------------------------------------
// Real @expo/vector-icons build/*.js files use bare-relative ESM imports
// (e.g. `import createIconSet from './createIconSet'`) with no package.json
// "type": "module" - vitest's SSR module resolution can't resolve those
// extensionless imports, so any test that renders a component importing this
// package (unmocked) fails with "Cannot find module ... imported from
// AntDesign.js", unrelated to any app logic. Pre-existing gap, first hit here
// because this screen is the first TV test to render a real (un-mocked)
// Ionicons usage.
vi.mock("@expo/vector-icons", () => ({
  Ionicons: () => null,
}));

// --- expo-router mock ---------------------------------------------------------
const replaceMock = vi.fn();
const backMock = vi.fn();
const localSearchParamsMock = vi.fn();
// canGoBack defaults to true so the existing back()-based tests below are
// unaffected by the playerExit contract (see src/hooks/playerExit.ts) - a
// dedicated describe block further down drives the false branch.
const canGoBackMock = vi.fn(() => true);
vi.mock("expo-router", () => ({
  router: {
    replace: (...args: unknown[]) => replaceMock(...args),
    back: (...args: unknown[]) => backMock(...args),
    canGoBack: () => canGoBackMock(),
  },
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
// No profile/preferences data by default - track resolution falls back to the
// manifest's isDefault stream, matching this suite's prior behaviour. The
// dedicated language-preference tests below override these with their own
// vi.mocked(...).mockReturnValue.
const getCurrentUserMock = vi.fn(() => ({ data: undefined }));
const listProfilesMock = vi.fn(() => ({ data: undefined }));
vi.mock("@evey/api", () => ({
  useLibraryServiceGetMediaItem: (...args: unknown[]) => getMediaItemMock(...args),
  usePlaybackServiceMarkAsWatched: (...args: unknown[]) => markAsWatchedMock(...args),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => getWatchProgressMock(...args),
  useUserServiceGetCurrentUser: (...args: unknown[]) => getCurrentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
}));

// --- usePlaySession mock -----------------------------------------------------------
// streamUrl is mutable so a test can drive a track-change restart: the real
// restartWithTracks tears the session down and comes back with a fresh URL,
// which recreates the native player. The mock mutates the URL; the test then
// rerenders to observe the recreated player's behavior. mediaStreams is
// likewise mutable so the profile-language-preference tests can supply
// audio/subtitle tracks without a dedicated mock module.
const stopSessionMock = vi.fn();
const sessionState = {
  streamUrl: "http://srv.test/stream.m3u8",
  mediaStreams: [] as { type: string; index: number; language?: string; isDefault?: boolean }[],
};
const restartWithTracksMock = vi.fn(() => {
  sessionState.streamUrl = "http://srv.test/stream.m3u8?restarted=1";
});
vi.mock("../../../src/hooks/usePlaySession", () => ({
  usePlaySession: () => ({
    streamUrl: sessionState.streamUrl,
    sessionId: "sess-1",
    streamInfo: { mediaStreams: sessionState.mediaStreams },
    isLoading: false,
    isError: false,
    stopSession: stopSessionMock,
    restartWithTracks: restartWithTracksMock,
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
// useChapters is a real react-query hook (unmocked, it would need a
// QueryClientProvider this test never sets up) - mocked flat like every other
// data hook here. findCurrentChapterIndex is a plain re-export, safe to keep real.
vi.mock("../../../src/hooks/useChapters", async () => {
  const actual = await vi.importActual<typeof import("../../../src/hooks/useChapters")>(
    "../../../src/hooks/useChapters",
  );
  return { ...actual, useChapters: () => ({ data: [] }) };
});
const sleepTimerStateMock = { state: null, setMinutes: vi.fn(), setEndOfEpisode: vi.fn(), cancel: vi.fn() };
vi.mock("../../../src/hooks/useSleepTimer", () => ({
  useSleepTimer: () => sleepTimerStateMock,
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
// QualityPicker exposes its onSelect so tests can drive a real track-change
// restart through the screen's own handler (visible-gating is irrelevant here).
vi.mock("../../../src/components/QualityPicker", () => ({
  QualityPicker: ({ onSelect }: { onSelect: (q: number) => void }) =>
    React.createElement("button", { "aria-label": "pick-quality", onClick: () => onSelect(720) }, "720p"),
}));
// AudioTrackPicker/SubtitlePicker capture currentIndex (and, for audio,
// onSelect) so the profile language preference tests can assert which
// stream the screen resolved as selected - and simulate a manual pick -
// without needing the (visible-gated) picker UI itself.
let lastAudioPickerProps: { currentIndex: number; onSelect: (i: number) => void } | null = null;
vi.mock("../../../src/components/AudioTrackPicker", () => ({
  AudioTrackPicker: (p: { currentIndex: number; onSelect: (i: number) => void }) => {
    lastAudioPickerProps = p;
    return null;
  },
}));
let lastSubtitlePickerProps: { currentIndex: number | null } | null = null;
vi.mock("../../../src/components/SubtitlePicker", () => ({
  SubtitlePicker: (p: { currentIndex: number | null }) => {
    lastSubtitlePickerProps = p;
    return null;
  },
}));
vi.mock("../../../src/components/TrackOptionsMenu", () => ({ TrackOptionsMenu: () => null }));
vi.mock("../../../src/components/ChaptersPicker", () => ({ ChaptersPicker: () => null }));
vi.mock("../../../src/components/SleepTimerPicker", () => ({ SleepTimerPicker: () => null }));
vi.mock("../../../src/components/VolumePicker", () => ({ VolumePicker: () => null }));
vi.mock("../../../src/components/StreamInfoPanel", () => ({ StreamInfoPanel: () => null }));
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
  playerListeners.clear();
  playerMock.playing = false;
  lastSourceUri = null;
  capturedTVHandler = null;
  canGoBackMock.mockReturnValue(true);
  sessionState.streamUrl = "http://srv.test/stream.m3u8";
  sessionState.mediaStreams = [];
  restartWithTracksMock.mockImplementation(() => {
    sessionState.streamUrl = "http://srv.test/stream.m3u8?restarted=1";
  });
  getCurrentUserMock.mockReturnValue({ data: undefined });
  listProfilesMock.mockReturnValue({ data: undefined });
  lastAudioPickerProps = null;
  lastSubtitlePickerProps = null;
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
    previousEpisode: { id: "ep-0", officialRating: "G" },
    playNow: () => capturedOnNavigate?.("ep-2"),
    playPrevious: () => capturedOnNavigate?.("ep-0"),
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

// A native playback failure (decoder error, 401 on a segment, broken manifest)
// arrives through statusChange - NOT through usePlaySession's isError, which
// only covers the session-start network call. Swallowing it left a black
// screen with live controls and no way to know playback was dead.
describe("native player errors (TV)", () => {
  it("shows the error screen when the native player reports a fatal error", () => {
    render(<PlayerScreen />);
    act(() => {
      playerListeners.get("statusChange")?.({ status: "error", error: { message: "decoder died" } });
    });
    expect(screen.getByText("Failed to load stream")).toBeTruthy();
  });

  it("ignores non-error status transitions", () => {
    render(<PlayerScreen />);
    act(() => {
      playerListeners.get("statusChange")?.({ status: "readyToPlay", oldStatus: "loading" });
    });
    expect(screen.queryByText("Failed to load stream")).toBeNull();
  });
});

// Changing quality/audio/subtitles tears the session down and brings up a new
// native player, whose setup used to call play() unconditionally - a paused
// screen sprang back to life on every track change.
describe("track change keeps the pause state (TV)", () => {
  it("does not auto-play after a track restart while paused", () => {
    const { rerender } = render(<PlayerScreen />);
    expect(playMock).toHaveBeenCalledTimes(1); // initial entry autoplays

    playerMock.playing = false; // user paused before changing quality
    fireEvent.click(screen.getByLabelText("pick-quality"));
    rerender(<PlayerScreen />); // new streamUrl -> player recreated -> setup re-runs

    expect(restartWithTracksMock).toHaveBeenCalled();
    expect(playMock).toHaveBeenCalledTimes(1); // still paused: no second play()
  });

  it("resumes after a track restart while playing", () => {
    const { rerender } = render(<PlayerScreen />);
    playerMock.playing = true;
    fireEvent.click(screen.getByLabelText("pick-quality"));
    rerender(<PlayerScreen />);
    expect(playMock).toHaveBeenCalledTimes(2);
  });
});

// A fatal native error otherwise offered no way forward (item 8): retrying
// re-issues the same session start the picker rows already use.
describe("retry after a fatal native error (TV)", () => {
  it("clears the error and restarts the session on retry", () => {
    render(<PlayerScreen />);
    act(() => {
      playerListeners.get("statusChange")?.({ status: "error", error: { message: "decoder died" } });
    });
    expect(screen.getByText("Failed to load stream")).toBeTruthy();

    fireEvent.click(screen.getByText("player.retry"));
    expect(restartWithTracksMock).toHaveBeenCalled();
    expect(screen.queryByText("Failed to load stream")).toBeNull();
  });

  // The player is recreated per media (mediaId changes on next-episode
  // navigation), but nativeError previously survived the change - a decoder
  // error on episode 1 stayed glued to episode 2's screen.
  it("resets a stale native error when the media changes", () => {
    const { rerender } = render(<PlayerScreen />);
    act(() => {
      playerListeners.get("statusChange")?.({ status: "error", error: { message: "decoder died" } });
    });
    expect(screen.getByText("Failed to load stream")).toBeTruthy();

    localSearchParamsMock.mockReturnValue({ id: "ep-2" });
    getMediaItemMock.mockReturnValue({ data: { id: "ep-2", type: "MEDIA_ITEM_TYPE_EPISODE", title: "Episode 2" } });
    rerender(<PlayerScreen />);

    expect(screen.queryByText("Failed to load stream")).toBeNull();
  });
});

// Same contract as the web/mobile back button (src/hooks/playerExit.ts): pop
// the history entry when there is one, otherwise land on the media page.
describe("player exit contract (TV)", () => {
  it("pops history on menu/back when one exists", () => {
    canGoBackMock.mockReturnValue(true);
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "menu" }); });
    expect(backMock).toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalledWith(expect.stringContaining("/media/"));
  });

  it("replaces toward the media page when there is nothing to pop", () => {
    canGoBackMock.mockReturnValue(false);
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    expect(backMock).not.toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/media/ep-1");
  });
});

// Manual previous/next episode controls (item 2) - distinct from the
// auto-advance countdown, reachable via the long-press D-pad keys.
describe("manual previous/next episode (TV)", () => {
  it("navigates to the next episode on longDown when no PIN is required", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "longDown" }); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");
  });

  it("navigates to the previous episode on longUp when no PIN is required", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "longUp" }); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-0");
  });

  it("gates the previous episode independently of the next episode's rating", () => {
    // Next episode ("R") is not gated, but the previous one ("G") is - proves
    // the two directions use independent parental gates.
    needsPinMock.mockImplementation((rating) => rating === "G");
    render(<PlayerScreen />);

    act(() => { capturedTVHandler?.({ eventType: "longDown" }); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");

    replaceMock.mockClear();
    act(() => { capturedTVHandler?.({ eventType: "longUp" }); });
    expect(replaceMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });
});

// Accelerated seek (item 1): longLeft/longRight move by a larger step than a
// plain left/right, computed by playerSeek.ts's seekDeltaForKey.
describe("remote seek (TV)", () => {
  it("seeks by the plain skip amount on left/right", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(playerMock.seekBy).toHaveBeenCalledWith(30);
    act(() => { capturedTVHandler?.({ eventType: "left" }); });
    expect(playerMock.seekBy).toHaveBeenCalledWith(-30);
  });

  it("seeks by an accelerated step on a held longLeft/longRight", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "longRight" }); });
    expect(playerMock.seekBy).toHaveBeenCalledWith(120); // 30s * 4x multiplier
    act(() => { capturedTVHandler?.({ eventType: "longLeft" }); });
    expect(playerMock.seekBy).toHaveBeenCalledWith(-120);
  });
});

// Profile audio/subtitle language preference (audio_languages/subtitle_languages,
// shared web/mobile/tv convention) must resolve the initial track, not just the
// manifest's isDefault flag - mirrors the mobile player's equivalent wiring.
describe("profile language preference (TV)", () => {
  beforeEach(() => {
    getCurrentUserMock.mockReturnValue({ data: { id: "user-1" } });
    listProfilesMock.mockReturnValue({
      data: {
        profiles: [
          { id: "prof-1", isDefault: true, preferences: { audio_languages: "de", subtitle_languages: "fr" } },
        ],
      },
    });
    sessionState.mediaStreams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 0, language: "eng", isDefault: true },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "ger" },
      { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 2, language: "fre" },
      { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 3, language: "eng" },
    ];
  });

  it("selects the audio stream matching the profile's preferred language over isDefault", () => {
    render(<PlayerScreen />);
    // 639-2 "ger" stream must be picked for a 639-1 "de" preference, ahead of
    // the manifest's own isDefault ("eng") stream.
    expect(lastAudioPickerProps?.currentIndex).toBe(1);
  });

  it("selects the subtitle stream matching the profile's preferred language", () => {
    render(<PlayerScreen />);
    // 639-2 "fre" stream must be picked for a 639-1 "fr" preference.
    expect(lastSubtitlePickerProps?.currentIndex).toBe(2);
  });

  it("falls back to isDefault when no stream matches the preferred language", () => {
    listProfilesMock.mockReturnValue({
      data: {
        profiles: [{ id: "prof-1", isDefault: true, preferences: { audio_languages: "ja" } }],
      },
    });
    render(<PlayerScreen />);
    expect(lastAudioPickerProps?.currentIndex).toBe(0);
  });

  it("does not re-apply the preference over a track the user picked manually", () => {
    const { rerender } = render(<PlayerScreen />);
    expect(lastAudioPickerProps?.currentIndex).toBe(1); // preference applied first

    act(() => { lastAudioPickerProps?.onSelect(0); }); // user manually picks the English track
    expect(lastAudioPickerProps?.currentIndex).toBe(0);

    // A track/quality restart re-derives audioStreams (new array reference) -
    // without the userPickedAudioRef guard this used to silently revert the
    // user's pick back to the profile-preferred "de" track.
    sessionState.mediaStreams = [...sessionState.mediaStreams];
    rerender(<PlayerScreen />);
    expect(lastAudioPickerProps?.currentIndex).toBe(0);
  });
});
