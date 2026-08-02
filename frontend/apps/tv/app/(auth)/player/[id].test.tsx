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
const getCurrentUserMock = vi.fn((..._args: unknown[]): { data: unknown } => ({ data: undefined }));
const listProfilesMock = vi.fn((..._args: unknown[]): { data: unknown } => ({ data: undefined }));
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
  isAuthError: false,
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
    isAuthError: sessionState.isAuthError,
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
// Captured rather than rendered: the episode/quality/speed entries moved into
// this menu when the undiscoverable long-press bindings were removed, and the
// tests below drive them through the props the player hands over.
const trackMenuProps: { current: Record<string, unknown> } = { current: {} };
vi.mock("../../../src/components/TrackOptionsMenu", () => ({
  TrackOptionsMenu: (p: Record<string, unknown>) => {
    trackMenuProps.current = p;
    return null;
  },
}));
vi.mock("../../../src/components/ChaptersPicker", () => ({ ChaptersPicker: () => null }));
vi.mock("../../../src/components/SleepTimerPicker", () => ({ SleepTimerPicker: () => null }));
vi.mock("../../../src/components/VolumePicker", () => ({ VolumePicker: () => null }));
vi.mock("../../../src/components/StreamInfoPanel", () => ({ StreamInfoPanel: () => null }));
vi.mock("../../../src/components/SkipIntroButton", () => ({ SkipIntroButton: () => null }));
vi.mock("../../../src/components/NextEpisodeOverlay", () => ({ NextEpisodeOverlay: () => null }));
vi.mock("../../../src/components/SyncplaySessionBadge", () => ({ SyncplaySessionBadge: () => null }));
// TrickplaySeekPreview captures its own props so the remote-seek tests below
// can assert the screen wires the live preview position through to it,
// without needing this suite to also mock a manifest fetch.
let lastTrickplayPreviewProps: { mediaId: string; previewTimeSec: number | null } | null = null;
vi.mock("../../../src/components/TrickplaySeekPreview", () => ({
  TrickplaySeekPreview: (p: { mediaId: string; previewTimeSec: number | null }) => {
    lastTrickplayPreviewProps = p;
    return null;
  },
}));

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

// --- useSyncplay mock ---------------------------------------------------------------
// Disconnected/no-op by default: none of the existing suites below opt into a
// SyncPlay session, so this must never throw or touch the player on its own.
vi.mock("../../../src/hooks/useSyncplay", () => ({
  useSyncplay: () => ({
    state: { roomId: null, connected: false, isHost: false, members: [] },
    connect: vi.fn(),
    leaveRoom: vi.fn(),
    sendPlay: vi.fn(),
    sendPause: vi.fn(),
  }),
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
  lastTrickplayPreviewProps = null;
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
// An expired session used to land on "Failed to load stream" - the owner spent
// a morning hunting a codec bug that did not exist, on a television whose only
// problem was that it had been signed out. Retrying an expired token can only
// fail again, so the screen must offer signing in instead.
describe("expired session (TV)", () => {
  it("says the session expired rather than blaming the stream", () => {
    sessionState.isAuthError = true;
    try {
      render(<PlayerScreen />);
      expect(screen.getByText("Your session has expired")).toBeTruthy();
      expect(screen.queryByText("Failed to load stream")).toBeNull();
    } finally {
      sessionState.isAuthError = false;
    }
  });

  it("offers signing in, not a retry that cannot succeed", () => {
    sessionState.isAuthError = true;
    try {
      render(<PlayerScreen />);
      expect(screen.queryByText("player.retry")).toBeNull();
      fireEvent.click(screen.getByText("auth.login.signIn"));
      expect(replaceMock).toHaveBeenCalledWith("/login");
    } finally {
      sessionState.isAuthError = false;
    }
  });
});

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

// Manual previous/next episode - distinct from the auto-advance countdown.
// These used to hang off long-up/long-down, bindings nothing on screen
// announced; they are entries in the More menu now. The parental gating they
// carry is the part that matters and is unchanged.
describe("manual previous/next episode (TV)", () => {
  const openMenu = () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
  };

  it("navigates to the next episode when no PIN is required", () => {
    openMenu();
    act(() => { (trackMenuProps.current.onNextEpisode as () => void)(); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");
  });

  it("navigates to the previous episode when no PIN is required", () => {
    openMenu();
    act(() => { (trackMenuProps.current.onPreviousEpisode as () => void)(); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-0");
  });

  it("gates the previous episode independently of the next episode's rating", () => {
    // Next episode ("R") is not gated, but the previous one ("G") is - proves
    // the two directions use independent parental gates.
    needsPinMock.mockImplementation((rating) => rating === "G");
    openMenu();

    act(() => { (trackMenuProps.current.onNextEpisode as () => void)(); });
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/ep-2");

    replaceMock.mockClear();
    act(() => { (trackMenuProps.current.onPreviousEpisode as () => void)(); });
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

  // The trickplay thumbnail (TrickplaySeekPreview) is the only feedback the
  // user gets while holding left/right, since there's no live video preview
  // during a D-pad seek - it must track the same locally-computed preview
  // position the progress bar shows (seekPreviewSec), not the stale
  // pre-seek playhead.
  it("passes the live seek-preview position through to the trickplay thumbnail", () => {
    render(<PlayerScreen />);
    expect(lastTrickplayPreviewProps).toEqual({ mediaId: "ep-1", previewTimeSec: null });

    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(lastTrickplayPreviewProps).toEqual({ mediaId: "ep-1", previewTimeSec: 30 });
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

// The owner could not work out how to drive this player at all: every action
// hid behind a binding nothing on screen announced (long-OK opened the track
// menu, down opened quality, up cycled speed, long-up/long-down changed
// episode), and the only visible things - the play glyph, the seek bubbles,
// the episode chevrons - were decorative Views.
describe("action bar discoverability (TV)", () => {
  const withTwoAudioTracks = () => {
    sessionState.mediaStreams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 0, language: "fra", isDefault: true },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "eng" },
    ];
  };

  it("tells the viewer how to reach the options before they press anything", () => {
    render(<PlayerScreen />);
    // The hint is the whole fix for "I have no idea how to control this".
    expect(screen.getByText("More", { exact: false })).toBeTruthy();
  });

  // A pointer has no "down key". The emulator is driven with a mouse and some
  // remotes ship a touchpad, so the hint answers a click too.
  it("opens the bar on a click of the hint, without any key press", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("More"));
    expect(screen.getByLabelText("player.settings.audio")).toBeTruthy();
  });

  it("opens the bar on the down key and offers audio and more", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    expect(screen.getByLabelText("player.settings.audio")).toBeTruthy();
    expect(screen.getByLabelText("More")).toBeTruthy();
  });

  // The whole point of the redesign: changing the audio track is one press
  // away and labelled, instead of a long press nothing announced.
  it("opens the audio picker straight from the bar", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    // The picker mock renders nothing, so the proof it opened is that the
    // remote handler stands down for it (anyOverlayOpen) - a plain right no
    // longer seeks.
    fireEvent.click(screen.getByLabelText("player.settings.audio"));
    playerMock.seekBy.mockClear();
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(playerMock.seekBy).not.toHaveBeenCalled();
  });

  // While the bar has focus its buttons own left/right; seeking there would
  // move the film behind the viewer's back as they walk the row.
  it("stops seeking while the bar holds focus, and resumes after leaving it", () => {
    render(<PlayerScreen />);

    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    playerMock.seekBy.mockClear();
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(playerMock.seekBy).not.toHaveBeenCalled();

    // Up leaves the bar, and the film is seekable again.
    act(() => { capturedTVHandler?.({ eventType: "up" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(playerMock.seekBy).toHaveBeenCalledWith(30);
  });

  // Speed used to fire on a bare up press - the easiest key to hit by accident
  // on a remote, and the hardest effect to understand once it happened.
  it("no longer changes speed on a bare up press", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "up" }); });
    expect(screen.queryByText("1.25x")).toBeNull();
  });
});

// "Maison du dragon" is a series: it has no media_files row, so the stream
// endpoint 404s and the player blamed the stream. The home hero handed the
// player whatever it was spotlighting, series included.
// Android TV stops delivering key events (remote AND pointer) once no native
// view holds focus. The screen keeps a full-screen Pressable under every
// overlay for exactly that reason - these tests lock its contract down.
describe("focus-holding player surface (TV)", () => {
  it("renders a focusable full-screen surface under the overlays", () => {
    render(<PlayerScreen />);
    expect(screen.getByLabelText("Player surface")).toBeTruthy();
  });

  it("toggles play/pause on press while the controls are visible", () => {
    render(<PlayerScreen />);
    playMock.mockClear();
    playerMock.playing = false;
    fireEvent.click(screen.getByLabelText("Player surface"));
    expect(playMock).toHaveBeenCalled();

    pauseMock.mockClear();
    playerMock.playing = true;
    fireEvent.click(screen.getByLabelText("Player surface"));
    expect(pauseMock).toHaveBeenCalled();
  });

  it("wakes the controls instead of toggling playback when they are already hidden", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      act(() => { vi.advanceTimersByTime(4000); }); // CONTROLS_TIMEOUT elapses
      expect(screen.queryByLabelText("More")).toBeNull();

      playMock.mockClear();
      pauseMock.mockClear();
      fireEvent.click(screen.getByLabelText("Player surface"));

      expect(playMock).not.toHaveBeenCalled();
      expect(pauseMock).not.toHaveBeenCalled();
      expect(screen.getByLabelText("More")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  // On real hardware, pressing OK while the surface holds native focus fires
  // BOTH the surface's own onPress and the global remote handler's "select" -
  // the global handler must stand down while the surface has focus, or
  // play/pause fires twice for one press.
  it("does not double-toggle play/pause when the global handler sees select while the surface is focused", () => {
    render(<PlayerScreen />);
    fireEvent.focus(screen.getByLabelText("Player surface"));
    playMock.mockClear();
    pauseMock.mockClear();
    playerMock.playing = false;

    fireEvent.click(screen.getByLabelText("Player surface")); // the surface's own onPress
    act(() => { capturedTVHandler?.({ eventType: "select" }); }); // the global handler, same keypress

    expect(playMock).toHaveBeenCalledTimes(1);
    expect(pauseMock).not.toHaveBeenCalled();
  });
});

// The action bar used to keep hiding on the fixed 4s timer even while the
// viewer was still walking it with the remote - it vanished mid-navigation.
describe("sticky action bar (TV)", () => {
  it("keeps the action bar visible well past the hide timeout while it holds focus", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      act(() => { capturedTVHandler?.({ eventType: "down" }); }); // opens the bar
      expect(screen.getByLabelText("player.controls.play")).toBeTruthy();

      act(() => { vi.advanceTimersByTime(4000 * 3); });

      expect(screen.getByLabelText("player.controls.play")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it("re-arms the hide timer once the bar loses focus, and hides after it elapses", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      act(() => { capturedTVHandler?.({ eventType: "down" }); }); // opens the bar
      expect(screen.getByLabelText("player.controls.play")).toBeTruthy();

      act(() => { capturedTVHandler?.({ eventType: "up" }); }); // leaves the bar
      act(() => { vi.advanceTimersByTime(4000); });

      expect(screen.queryByLabelText("player.controls.play")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("unplayable container items (TV)", () => {
  it("sends a series to its detail page instead of trying to stream it", () => {
    getMediaItemMock.mockReturnValue({
      data: { id: "sr-1", type: "MEDIA_ITEM_TYPE_SERIES", title: "Maison du dragon" },
    });
    render(<PlayerScreen />);
    expect(replaceMock).toHaveBeenCalledWith("/media/ep-1");
  });

  it("does the same for a season", () => {
    getMediaItemMock.mockReturnValue({
      data: { id: "sn-1", type: "MEDIA_ITEM_TYPE_SEASON", title: "Saison 1" },
    });
    render(<PlayerScreen />);
    expect(replaceMock).toHaveBeenCalledWith("/media/ep-1");
  });

  it("leaves an episode alone", () => {
    render(<PlayerScreen />);
    expect(replaceMock).not.toHaveBeenCalledWith(expect.stringContaining("/media/"));
  });
});
