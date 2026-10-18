import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, act } from "@testing-library/react";
import React from "react";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";

// --- React Native mock -----------------------------------------------------
// useTVEventHandler captures the screen's own callback so tests can simulate a
// remote keypress directly, the same way playerListeners lets a test simulate
// a native player event.
let capturedBackHandler: (() => boolean) | null = null;
let capturedTVHandler: ((evt: { eventType: string; eventKeyAction?: number }) => void) | null = null;
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
  useWindowDimensions: () => ({ width: 1920, height: 1080 }),
  useTVEventHandler: (cb: (evt: { eventType: string; eventKeyAction?: number }) => void) => { capturedTVHandler = cb; },
  // The screen resolves native tags to wire the controls' neighbours; jsdom
  // has none, and null is exactly what the screen already tolerates.
  findNodeHandle: () => null,
  // BACK is claimed by the screen so a submenu closes instead of the player
  // exiting; the captured handler lets a test press it.
  BackHandler: {
    addEventListener: (_evt: string, cb: () => boolean) => {
      capturedBackHandler = cb;
      return { remove: () => { capturedBackHandler = null; } };
    },
  },
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
  VideoView: () => React.createElement("div", { "data-testid": "video-view" }),
}));
// Mirrors the real hook's contract where the tests depend on it: no source, no
// player; otherwise one player per source identity.
vi.mock("@evey/client/native/useSourcedVideoPlayer", () => ({
  useSourcedVideoPlayer: (source: { uri?: string } | null, setup?: (p: unknown) => void) => {
    if (source === null) return null;
    const uri = source?.uri ?? null;
    if (uri !== lastSourceUri) {
      lastSourceUri = uri;
      setup?.(playerMock);
    }
    return playerMock;
  },
}));

// --- react-i18next mock ----------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
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
  // The screen subscribes to navigation events to know it is being popped
  // (the hardware BACK key never reaches its own handler).
  useNavigation: () => ({ addListener: () => () => {} }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", error: "#f33", overlay: "rgba(0,0,0,0.6)" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store mock -------------------------------------------------------------
vi.mock("@evey/store", async (importOriginal) => ({
  // Pure preference helpers (autoplayNextEpisode, ...) stay real; only the
  // store is stubbed.
  ...(await importOriginal<typeof import("@evey/store")>()),
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
// The screen persists the auto-play choice through the query cache; mocked
// flat like the data hooks (no QueryClientProvider here).
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceUpdateProfile: () => ({ mutateAsync: vi.fn() }),
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
  streamUrl: "http://srv.test/stream.m3u8" as string | null,
  isAuthError: false,
  mediaStreams: [] as { type: string; index: number; language?: string; isDefault?: boolean; codec?: string }[],
  progress: null as { positionMs?: string; durationMs?: string; played?: boolean } | null,
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
    progress: sessionState.progress,
  }),
}));

// --- misc hook mocks ---------------------------------------------------------------
// The decoder telemetry's own behaviour is tested in @evey/client; here only
// what the screen hands it matters (the restart when ac3go is too slow).
let decoderTelemetryOptions: { onTooSlow?: (reason: string) => void } | null = null;
vi.mock("../../../src/hooks/useDecoderTelemetry", () => ({
  useDecoderTelemetry: (_id: string, _r: unknown, _p: unknown, opts: { onTooSlow?: (reason: string) => void }) => {
    decoderTelemetryOptions = opts;
  },
}));
vi.mock("../../../src/hooks/useAudioCapsBeacon", () => ({ useAudioCapsBeacon: () => {} }));
vi.mock("../../../src/hooks/useQoEBeacon", () => ({ useQoEBeacon: () => {} }));
const progressGetters: { getPositionMs?: () => number } = {};
vi.mock("../../../src/hooks/useProgressReporter", () => ({
  useProgressReporter: (_sessionId: unknown, getPositionMs: () => number) => {
    progressGetters.getPositionMs = getPositionMs;
  },
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
let capturedNextEpArgs: { currentTime: number; duration: number } | null = null;
const nextEpisodeStateMock = vi.fn();
vi.mock("../../../src/hooks/useNextEpisode", () => ({
  useNextEpisode: (args: { onNavigate: (episodeId: string) => void; currentTime: number; duration: number }) => {
    capturedOnNavigate = args.onNavigate;
    capturedNextEpArgs = args;
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
let lastSubtitlePickerProps: { currentIndex: number | null; onSelect: (i: number | null) => void } | null = null;
vi.mock("../../../src/components/SubtitlePicker", () => ({
  SubtitlePicker: (p: { currentIndex: number | null; onSelect: (i: number | null) => void }) => {
    lastSubtitlePickerProps = p;
    return null;
  },
}));
// Captured, not rendered: what the screen hands the overlay IS the assertion -
// a track drawn client-side must reach it, and must not reach the server.
let lastOverlayProps: { subtitleIndex: number | null } | null = null;
vi.mock("../../../src/components/SubtitleOverlay", () => ({
  SubtitleOverlay: (p: { subtitleIndex: number | null }) => {
    lastOverlayProps = p;
    overlayRenders++;
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
// Captures the screen's onSkip wiring so the skip-clamp (M2) test below can
// drive it directly, without needing the (segment-gated) button UI itself.
let lastSkipIntroProps: { onSkip: (toSec: number) => void } | null = null;
// Counts how often the screen re-renders: every child is re-rendered with it.
let overlayRenders = 0;
vi.mock("../../../src/components/SkipIntroButton", () => ({
  SkipIntroButton: (p: { onSkip: (toSec: number) => void }) => {
    lastSkipIntroProps = p;
    return null;
  },
}));
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
  // The released-player test turns currentTime into a throwing getter; every
  // test starts from a plain field at zero, whatever ran before.
  Object.defineProperty(playerMock, "currentTime", { value: 0, writable: true, configurable: true });
  playerMock.duration = 0;
  lastSourceUri = null;
  sessionState.progress = null;
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
  lastOverlayProps = null;
  lastTrickplayPreviewProps = null;
  lastSkipIntroProps = null;
  overlayRenders = 0;
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

  // While the credits countdown runs, BACK is the viewer refusing the next
  // episode, not leaving the player (prod 2026-10-15).
  it("cancels a running credits countdown on back instead of exiting", () => {
    canGoBackMock.mockReturnValue(true);
    render(<PlayerScreen />);
    const cancel = vi.fn();
    const ref = (lastSkipIntroProps as { cancelCountdownRef?: { current: (() => void) | null } } | null)?.cancelCountdownRef;
    expect(ref).toBeDefined();
    ref!.current = cancel;
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(backMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalledWith(expect.stringContaining("/media/"));

    ref!.current = null;
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    expect(backMock).toHaveBeenCalled();
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
  // Presses accumulate and ONE seek commits them: four taps of +30s is a
  // single two-minute jump, not four restarts of the stream (owner,
  // 2026-09-19 - "I want to seek by more than 30 seconds").
  it("adds up the presses and commits one seek when they stop", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    // Nothing has moved yet: the viewer is still pressing.
    expect(playerMock.currentTime).toBe(0);
    act(() => { vi.advanceTimersByTime(700); });
    // 30 + 30 + 60: the third tap of one gesture takes a bigger step (see
    // tapSeekStep) so a long way ahead does not take twenty taps.
    expect(playerMock.currentTime).toBe(120);
    vi.useRealTimers();
  });

  it("starts a new gesture at the plain step once a seek has committed", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(120);
    playerMock.currentTime = 120;
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(150);
    vi.useRealTimers();
  });

  // Some sets never send longRight and repeat the plain key while it is
  // held: a second "down" for the same direction before any "up" is that
  // repeat, and scrubs like a long press would instead of being ignored.
  it("scrubs on a repeated plain key too, and commits once on release", () => {
    vi.useFakeTimers();
    playerMock.currentTime = 0;
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 0 }); });
    act(() => { vi.advanceTimersByTime(400); });
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 0 }); });
    act(() => { vi.advanceTimersByTime(1500); });
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 1 }); });
    expect(playerMock.currentTime).toBe(0);
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBeGreaterThan(80);
    expect(playerMock.currentTime).toBeLessThan(400);
    vi.useRealTimers();
  });

  it("seeks backward the same way, and never past the start", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "left" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(0);
    vi.useRealTimers();
  });

  it("uses the accelerated step for a held longLeft/longRight", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "longRight" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(120); // 30s * 4x multiplier
    vi.useRealTimers();
  });

  // Android (react-native-tvos) delivers a tap as ONE "right" event with key
  // action 1, and a held key as "longRight" with action 0 then action 1 on
  // release (measured on the box, 2026-09-20). A tap is one skip; a hold
  // scrubs for as long as it lasts and commits once.
  it("treats a key release as one skip, and a stray key down as nothing", () => {
    vi.useFakeTimers();
    playerMock.currentTime = 0;
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 0 }); });
    act(() => { vi.advanceTimersByTime(120); });
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 1 }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(30);
    vi.useRealTimers();
  });

  it("scrubs between the long-press events and commits once on release", () => {
    vi.useFakeTimers();
    playerMock.currentTime = 0;
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "longRight", eventKeyAction: 0 }); });
    act(() => { vi.advanceTimersByTime(1500); });
    act(() => { capturedTVHandler?.({ eventType: "longRight", eventKeyAction: 1 }); });
    expect(playerMock.currentTime).toBe(0); // nothing seeks until the presses settle
    act(() => { vi.advanceTimersByTime(700); });
    // 1s at a minute per second then 0.5s at four: about 180s, ONE seek -
    // not two x4 jumps (holdScrubRate).
    expect(playerMock.currentTime).toBeGreaterThan(160);
    expect(playerMock.currentTime).toBeLessThan(200);
    vi.useRealTimers();
  });

  // The trickplay thumbnail (TrickplaySeekPreview) is the only feedback the
  // user gets while holding left/right, since there's no live video preview
  // during a D-pad seek - it must track the same locally-computed preview
  // position the progress bar shows (seekPreviewSec), not the stale
  // pre-seek playhead.
  it("passes the live seek-preview position through to the trickplay thumbnail", () => {
    render(<PlayerScreen />);
    expect(lastTrickplayPreviewProps).toMatchObject({ mediaId: "ep-1", previewTimeSec: null });

    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    expect(lastTrickplayPreviewProps).toMatchObject({ mediaId: "ep-1", previewTimeSec: 30 });
  });

  // The thumbnail is warmed while the controls are on screen, so the first
  // seek shows a picture instead of waiting for the manifest and a sheet.
  it("asks the trickplay thumbnail to warm up while the controls show", () => {
    render(<PlayerScreen />);
    expect(lastTrickplayPreviewProps).toMatchObject({ warm: true });
  });
});

// A film starts, the album stops: the music player is mounted for the whole
// signed-in app and kept playing under the video until the viewer closed it
// by hand (owner, 2026-09-22).
describe("music and video (TV)", () => {
  it("clears the music queue when the player opens", () => {
    useMusicQueueStore.getState().setQueue([{ id: "t1", title: "T", artist: "A", album: "B", duration: 100 }], 0);
    expect(useMusicQueueStore.getState().isPlaying).toBe(true);
    render(<PlayerScreen />);
    expect(useMusicQueueStore.getState().queue).toEqual([]);
    expect(useMusicQueueStore.getState().isPlaying).toBe(false);
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

  // The options are an icon on the timeline now, not a text hint floating in
  // the middle of the screen (owner, 2026-09-19).
  it("shows the settings control with the timeline", () => {
    render(<PlayerScreen />);
    expect(screen.getByLabelText("More")).toBeTruthy();
  });

  // A pointer has no "down key". The emulator is driven with a mouse and some
  // remotes ship a touchpad, so the icon answers a click too.
  it("opens the options on a click of that control, without any key press", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("More"));
    expect(trackMenuProps.current.visible).toBe(true);
  });

  // Audio and subtitles moved INTO that menu when the control bar that used to
  // carry them was replaced by the settings icon.
  it("offers audio and subtitles inside the options menu", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("More"));
    expect(trackMenuProps.current.hasAudioOptions).toBe(true);
    act(() => { (trackMenuProps.current.onOpenAudio as () => void)(); });
    expect(trackMenuProps.current.visible).toBe(false); // the menu steps aside for the picker
  });

  it("offers the transport controls the remote can land on", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    expect(screen.getByLabelText(/player\.controls\.(play|pause)/)).toBeTruthy();
    // Named with their seconds (an options object, so the key here).
    expect(screen.getByLabelText("player.controls.rewind")).toBeTruthy();
    expect(screen.getByLabelText("player.controls.forward")).toBeTruthy();
  });

  // BACK used to close the submenu AND pop the player: the viewer landed back
  // on the media page while the film was still supposed to be playing.
  it("closes the settings menu on BACK instead of leaving the player", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("More"));
    expect(trackMenuProps.current.visible).toBe(true);
    let consumed = false;
    act(() => { consumed = capturedBackHandler?.() ?? false; });
    expect(consumed).toBe(true);
    expect(trackMenuProps.current.visible).toBe(false);
    expect(backMock).not.toHaveBeenCalled();
  });

  // One step at a time: a panel opened FROM the settings menu returns to that
  // menu, and only the menu returns to the film (owner, 2026-09-19).
  it("walks back up one menu level at a time", () => {
    withTwoAudioTracks();
    render(<PlayerScreen />);
    fireEvent.click(screen.getByLabelText("More"));
    act(() => { (trackMenuProps.current.onOpenAudio as () => void)(); });
    expect(trackMenuProps.current.visible).toBe(false);

    act(() => { capturedBackHandler?.(); });
    expect(trackMenuProps.current.visible).toBe(true); // back to the settings menu

    act(() => { capturedBackHandler?.(); });
    expect(trackMenuProps.current.visible).toBe(false); // and only then out
    expect(backMock).not.toHaveBeenCalled();
  });

  // The whole point of the redesign: changing the audio track is one press
  // away and labelled, instead of a long press nothing announced.
  it("stands the remote down while the options are open", () => {
    vi.useFakeTimers();
    withTwoAudioTracks();
    render(<PlayerScreen />);
    // The menu mock renders nothing, so the proof it owns the D-pad is that a
    // plain right no longer seeks.
    fireEvent.click(screen.getByLabelText("More"));
    const before = playerMock.currentTime;
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(before);
    vi.useRealTimers();
  });

  // While the bar has focus its buttons own left/right; seeking there would
  // move the film behind the viewer's back as they walk the row.
  it("stops seeking while the controls hold focus, and resumes after leaving them", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);

    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    const before = playerMock.currentTime;
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(before);

    // BACK leaves the row, and the film is seekable again. UP stays inside it:
    // walking from the settings icon back to the play button must not fold
    // the overlay away.
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    // Absolute, not relative: the target comes from the screen's own playhead
    // (0 in this fixture), not from whatever the shared mock last held.
    expect(playerMock.currentTime).toBe(30);
    vi.useRealTimers();
  });

  // The timeline is a control of its own in the row (owner, 2026-09-22): DOWN
  // from the play button reaches it, and while it holds the focus left/right
  // move the playhead instead of walking the buttons, OK lands the jump at
  // once, and leaving it hands the row back to the buttons.
  it("seeks from the timeline while it holds the focus, and OK lands the jump at once", () => {
    vi.useFakeTimers();
    playerMock.currentTime = 0;
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    const timeline = screen.getByLabelText("Playback progress");
    act(() => { fireEvent.focus(timeline); });

    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 1 }); });
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 1 }); });
    expect(screen.getByText("+1:00")).toBeTruthy(); // two taps, one pending jump
    expect(playerMock.currentTime).toBe(0);
    fireEvent.click(timeline); // OK: no waiting for the presses to settle
    expect(playerMock.currentTime).toBe(60);

    // Off the timeline, the row's buttons own left/right again.
    act(() => { fireEvent.blur(timeline); });
    act(() => { capturedTVHandler?.({ eventType: "right", eventKeyAction: 1 }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(60);
    vi.useRealTimers();
  });

  it("toggles play/pause on OK over the timeline when nothing is pending", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "down" }); });
    const timeline = screen.getByLabelText("Playback progress");
    act(() => { fireEvent.focus(timeline); });
    const before = pauseMock.mock.calls.length + playMock.mock.calls.length;
    fireEvent.click(timeline);
    expect(pauseMock.mock.calls.length + playMock.mock.calls.length).toBe(before + 1);
  });

  // Speed used to fire on a bare up press - the easiest key to hit by accident
  // on a remote, and the hardest effect to understand once it happened.
  it("no longer changes speed on a bare up press", () => {
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "up" }); });
    expect(screen.queryByText("1.25x")).toBeNull();
  });
});

// B4: the 30s "up next" card only informs - it must not go deaf on the D-pad
// the way the other overlays (pickers/menus) legitimately do. Only the real
// end-of-episode countdown (nextEp.ended) claims that same exclusivity.
describe("next-episode card does not steal the D-pad early (TV)", () => {
  const cardState = (overrides: { ended: boolean; cancel?: () => void }) => ({
    visible: true,
    ended: overrides.ended,
    countdown: 5,
    nextEpisode: { id: "ep-2", officialRating: "R" },
    previousEpisode: null,
    playNow: vi.fn(),
    playPrevious: vi.fn(),
    cancel: overrides.cancel ?? vi.fn(),
  });

  it("still seeks while the 30s card is showing but the countdown hasn't started", () => {
    vi.useFakeTimers();
    nextEpisodeStateMock.mockReturnValue(cardState({ ended: false }));
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(30);
    vi.useRealTimers();
  });

  it("blocks the D-pad once the real end-of-episode countdown is running", () => {
    vi.useFakeTimers();
    nextEpisodeStateMock.mockReturnValue(cardState({ ended: true }));
    render(<PlayerScreen />);
    const before = playerMock.currentTime;
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(before);
    vi.useRealTimers();
  });

  it("cancels the card on back/menu instead of exiting the player while it is visible", () => {
    const cancel = vi.fn();
    nextEpisodeStateMock.mockReturnValue(cardState({ ended: false, cancel }));
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    expect(cancel).toHaveBeenCalled();
    expect(backMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
  });

  // The last episode of a season: the video ended, there is no next episode,
  // so the card renders nothing - and nothing may own the D-pad. The owner's
  // television froze on the credits with every key swallowed (2026-09-20).
  it("leaves the player on back when the episode ended and there is nothing to play next", () => {
    nextEpisodeStateMock.mockReturnValue({
      visible: false, ended: true, countdown: 0, nextEpisode: null, previousEpisode: null,
      playNow: vi.fn(), playPrevious: vi.fn(), cancel: vi.fn(),
    });
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "back" }); });
    expect(stopSessionMock).toHaveBeenCalled();
    expect(backMock).toHaveBeenCalled();
  });

  it("keeps seeking alive after the end when no card is on screen", () => {
    vi.useFakeTimers();
    playerMock.currentTime = 0;
    nextEpisodeStateMock.mockReturnValue({
      visible: false, ended: true, countdown: 0, nextEpisode: null, previousEpisode: null,
      playNow: vi.fn(), playPrevious: vi.fn(), cancel: vi.fn(),
    });
    render(<PlayerScreen />);
    act(() => { capturedTVHandler?.({ eventType: "left" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(0); // clamped at the start, but the key was HEARD: no throw, no swallow
    act(() => { capturedTVHandler?.({ eventType: "right" }); });
    act(() => { vi.advanceTimersByTime(700); });
    expect(playerMock.currentTime).toBe(30);
    vi.useRealTimers();
  });
});

// The end of a film held its last frame: only a key press left the player.
describe("the end of the stream (TV)", () => {
  const endState = (nextEpisode: { id: string } | null) => ({
    visible: false, ended: false, countdown: 5, nextEpisode, previousEpisode: null,
    playNow: vi.fn(), playPrevious: vi.fn(), cancel: vi.fn(),
  });

  it("leaves the player once a film has played to its end", () => {
    nextEpisodeStateMock.mockReturnValue(endState(null));
    render(<PlayerScreen />);
    act(() => { playerListeners.get("playToEnd")?.(); });
    expect(stopSessionMock).toHaveBeenCalled();
    expect(backMock).toHaveBeenCalled();
  });

  // While the stream URL is still negotiated there is no native player, so no
  // listener exists for an empty player's "end" to reach (on Android, expo-video
  // reports a sourceless player as ended at once: mobile beta.80 left every film
  // before its first byte on it, 2026-10-16).
  it("installs no player listener before the stream URL is known", () => {
    nextEpisodeStateMock.mockReturnValue(endState(null));
    sessionState.streamUrl = null;
    playerListeners.clear();
    try {
      render(<PlayerScreen />);
      expect(playerListeners.has("playToEnd")).toBe(false);
      expect(playerListeners.size).toBe(0);
      expect(screen.queryByTestId("video-view")).toBeNull();
      expect(backMock).not.toHaveBeenCalled();
    } finally {
      sessionState.streamUrl = "http://srv.test/stream.m3u8";
    }
  });

  it("hands over to the up-next countdown when another episode follows", () => {
    nextEpisodeStateMock.mockReturnValue(endState({ id: "ep-2" }));
    render(<PlayerScreen />);
    playerMock.duration = 2700;
    act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 2699.2 }); });
    act(() => { playerListeners.get("playToEnd")?.(); });
    expect(backMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
    // The last tick stopped short: the clock is pinned on the end, which is
    // what starts the up-next countdown.
    expect(capturedNextEpArgs?.currentTime).toBe(2700);
    playerMock.duration = 0;
  });
});

// M2: an unclamped segment end past (or too close to) the true duration
// jumped straight into a black frame or the credits scroll.
describe("skip-intro clamp (TV)", () => {
  // Twice a second, this screen used to re-render whole for a number nothing
  // was showing. The clock reaches state finely only while something follows
  // it; a weak television pays the rest of the film nothing.
  it("stops re-rendering on every clock tick once the controls are away", () => {
    vi.useFakeTimers();
    render(<PlayerScreen />);
    playerMock.duration = 1200;
    playerMock.playing = true;
    act(() => { playerListeners.get("statusChange")?.({ status: "readyToPlay" }); });
    act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 1 }); });
    // Controls up: every tick is followed.
    const withControls = (() => {
      overlayRenders = 0;
      for (let i = 0; i < 6; i++) act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 10 + i * 0.5 }); });
      return overlayRenders;
    })();
    expect(withControls).toBeGreaterThanOrEqual(6);

    // Controls hidden: only the ticks that cross a second commit.
    // The controls fold themselves after their own timeout (4s).
    act(() => { vi.advanceTimersByTime(4100); });
    const hidden = (() => {
      overlayRenders = 0;
      for (let i = 0; i < 6; i++) act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 100 + i * 0.25 }); });
      return overlayRenders;
    })();
    expect(hidden).toBeLessThan(withControls);
    vi.useRealTimers();
  });

  it("clamps the skip target to duration - 5s once duration is known", () => {
    render(<PlayerScreen />);
    playerMock.duration = 100;
    act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 10 }); }); // syncs the duration state
    act(() => { lastSkipIntroProps?.onSkip(999); });
    expect(playerMock.currentTime).toBe(95);
  });

  it("leaves an in-bounds target untouched", () => {
    render(<PlayerScreen />);
    playerMock.duration = 100;
    act(() => { playerListeners.get("timeUpdate")?.({ currentTime: 10 }); });
    act(() => { lastSkipIntroProps?.onSkip(30); });
    expect(playerMock.currentTime).toBe(30);
  });

  it("does not clamp before duration is known", () => {
    render(<PlayerScreen />);
    act(() => { lastSkipIntroProps?.onSkip(999); });
    expect(playerMock.currentTime).toBe(999);
  });
});

// B3: a native "loading" status fires on every micro-stall - flashing a
// spinner over those reads as "the app is struggling" when nothing is wrong.
describe("buffering overlay (TV)", () => {
  it("shows the overlay only after a sustained stall once playback has started", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      playerMock.playing = true;
      act(() => { playerListeners.get("playingChange")?.(); });

      act(() => { playerListeners.get("statusChange")?.({ status: "loading" }); });
      expect(screen.queryByTestId("spinner")).toBeNull();

      act(() => { vi.advanceTimersByTime(1200); });
      expect(screen.getByTestId("spinner")).toBeTruthy();

      act(() => { playerListeners.get("statusChange")?.({ status: "readyToPlay" }); });
      expect(screen.queryByTestId("spinner")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("never shows the overlay for a stall before playback has ever started", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      act(() => { playerListeners.get("statusChange")?.({ status: "loading" }); });
      act(() => { vi.advanceTimersByTime(5000); });
      expect(screen.queryByTestId("spinner")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
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
  // The row gets a longer grace than a passing wake - but it is a grace, not
  // a lease: a remote put down ends up giving the picture back (owner,
  // 2026-09-19, "the controls never leave the screen").
  it("holds the controls longer while the row has focus, then hides them anyway", () => {
    vi.useFakeTimers();
    try {
      render(<PlayerScreen />);
      act(() => { capturedTVHandler?.({ eventType: "down" }); }); // opens the row
      expect(screen.getByLabelText("player.controls.play")).toBeTruthy();

      act(() => { vi.advanceTimersByTime(5000) }); // past the plain 4s timeout
      expect(screen.getByLabelText("player.controls.play")).toBeTruthy();

      act(() => { vi.advanceTimersByTime(4000) }); // past the focused one
      expect(screen.queryByLabelText("player.controls.play")).toBeNull();
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

      act(() => { capturedTVHandler?.({ eventType: "back" }); }); // leaves the row
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
// The progress reporter calls these getters from its CLEANUP, which runs while
// the screen is being popped and the native player may already be released.
// Reading it raw threw from an effect cleanup, and React blamed the screen: the
// viewer pressed Back and got an error card instead of the media page.
describe("released player during teardown (TV)", () => {
  it("reports a position of zero instead of throwing once the player is released", () => {
    render(<PlayerScreen />);
    expect(progressGetters.getPositionMs).toBeTypeOf("function");
    playerMock.currentTime = 42;
    expect(progressGetters.getPositionMs!()).toBe(42000);

    // What a released expo-video proxy does on any access.
    Object.defineProperty(playerMock, "currentTime", {
      configurable: true,
      get() {
        const err = new Error(
          "The 1st argument cannot be cast to type expo.modules.video.player.VideoPlayer (received class java.lang.Integer)",
        );
        (err as { code?: string }).code = "ERR_USING_RELEASED_SHARED_OBJECT";
        throw err;
      },
    });
    expect(() => progressGetters.getPositionMs!()).not.toThrow();
    expect(progressGetters.getPositionMs!()).toBe(0);
  });
});

// La fuite de prod du 08-09, côté TV : `?subtitleTrack=` n'est pas « le
// sous-titre choisi », c'est l'ordre de l'incruster dans les images, et une
// incrustation interdit le palier copie. Sur un remux 4K HEVC HDR, changer de
// piste audio suffisait à transformer un DirectPlay en échelle transcodée
// tonemappée, parce que le redémarrage emportait au passage un sous-titre PGS
// que la session n'avait jamais eu.
describe("le sous-titre ne fuit plus dans le flux (TV)", () => {
  const pgs = () => {
    sessionState.mediaStreams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 0, language: "eng", isDefault: true },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "fra" },
      { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 2, language: "fre", codec: "hdmv_pgs_subtitle" },
    ];
  };
  const vobsub = () => {
    sessionState.mediaStreams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 0, language: "eng", isDefault: true },
      { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 2, language: "fre", codec: "dvd_subtitle" },
    ];
  };

  it("une piste PGS part à l'overlay et ne redémarre rien", () => {
    pgs();
    render(<PlayerScreen />);
    act(() => { lastSubtitlePickerProps?.onSelect(2); });
    // Ordinal RELATIF aux sous-titres (0), pas l'index absolu du flux (2).
    expect(lastOverlayProps?.subtitleIndex).toBe(0);
    expect(restartWithTracksMock).not.toHaveBeenCalled();
  });

  it("changer de piste audio n'ajoute pas l'incrustation que la session n'a pas", () => {
    pgs();
    render(<PlayerScreen />);
    act(() => { lastSubtitlePickerProps?.onSelect(2); });
    act(() => { lastAudioPickerProps?.onSelect(1); });
    expect(restartWithTracksMock).toHaveBeenCalledWith(1, undefined, undefined);
  });

  it("un VobSub, qu'on ne sait pas décoder, reste une incrustation serveur", () => {
    vobsub();
    render(<PlayerScreen />);
    act(() => { lastSubtitlePickerProps?.onSelect(2); });
    expect(lastOverlayProps?.subtitleIndex).toBeNull();
    expect(restartWithTracksMock).toHaveBeenCalledWith(0, 2, undefined);
  });

  it("quitter un VobSub redémarre sans incrustation", () => {
    vobsub();
    render(<PlayerScreen />);
    act(() => { lastSubtitlePickerProps?.onSelect(2); });
    restartWithTracksMock.mockClear();
    act(() => { lastSubtitlePickerProps?.onSelect(null); });
    expect(restartWithTracksMock).toHaveBeenCalledWith(0, undefined, undefined);
  });
});

// 2026-10-01, a Chromecast HD: too slow for ac3go, the picture crawled and the
// app stopped. The watchdog hands over; the screen restarts the same tracks,
// where the viewer is, now negotiated with the audio transcoded.
describe("ac3go too slow (TV)", () => {
  it("restarts the stream on the current tracks and keeps playing", () => {
    sessionState.mediaStreams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "fre", codec: "eac3" },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2, language: "eng", codec: "eac3" },
    ];
    playerMock.playing = true;
    render(<PlayerScreen />);
    act(() => { lastAudioPickerProps?.onSelect(2); });
    restartWithTracksMock.mockClear();
    act(() => { decoderTelemetryOptions?.onTooSlow?.("rtf=0.80/ur=0,rtf=0.80/ur=0"); });
    expect(restartWithTracksMock).toHaveBeenCalledWith(2, undefined, undefined);
  });
});

// Where playback begins: the rule of every client (resolveStartPositionMs).
describe("resume position (TV)", () => {
  it("starts over a title stopped in its last seconds", () => {
    sessionState.progress = { positionMs: "89000", durationMs: "90006" };
    render(<PlayerScreen />);
    expect(playerMock.currentTime).toBe(0);
  });

  it("resumes a title stopped mid-way", () => {
    sessionState.progress = { positionMs: "60000", durationMs: "90006" };
    render(<PlayerScreen />);
    expect(playerMock.currentTime).toBe(60);
  });
});
