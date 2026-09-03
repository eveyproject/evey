import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act, cleanup } from "@testing-library/react";
import React from "react";

// Going fullscreen rotates the window AND moves the safe-area insets. Those are
// two independent native events that reach JS one or more frames apart, so the
// control bar used to be laid out two or three times in a row, each with a
// different padding and a different volume breakpoint - the "icons that settle
// in several steps" the viewer reported. These tests drive the two events
// separately and assert the bar is never PAINTED in an intermediate geometry.

type Store<T> = { get: () => T; set: (v: T) => void; subscribe: (l: () => void) => () => void };

function makeStore<T>(initial: T): Store<T> {
  let value = initial;
  const listeners = new Set<() => void>();
  return {
    get: () => value,
    set: (v: T) => {
      value = v;
      listeners.forEach((l) => l());
    },
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => { listeners.delete(l); };
    },
  };
}

const PORTRAIT = { width: 412, height: 915, scale: 2.6, fontScale: 1 };
const LANDSCAPE = { width: 915, height: 412, scale: 2.6, fontScale: 1 };
const PORTRAIT_INSETS = { top: 24, bottom: 48, left: 0, right: 0 };
// Landscape on a notched phone: the cutout moves to the left edge, the gesture
// bar to the right, and the status-bar inset collapses.
const LANDSCAPE_INSETS = { top: 0, bottom: 24, left: 48, right: 36 };

const dimsStore = makeStore(PORTRAIT);
const insetsStore = makeStore(PORTRAIT_INSETS);

function flattenStyle(style: unknown): Record<string, unknown> {
  if (!style) return {};
  if (Array.isArray(style)) return style.reduce<Record<string, unknown>>((acc, s) => ({ ...acc, ...flattenStyle(s) }), {});
  return style as Record<string, unknown>;
}

// --- react-native ---------------------------------------------------------------------
vi.mock("react-native", () => {
  const el = (tag: string) =>
    ({ children, style, pointerEvents, testID, onPress, ...rest }: any) =>
      React.createElement(
        tag,
        {
          "data-style": JSON.stringify(flattenStyle(style)),
          "data-pointer-events": pointerEvents,
          "data-testid": testID,
          "aria-label": rest.accessibilityLabel,
          onClick: onPress,
        },
        children,
      );
  return {
    View: el("div"),
    Text: el("span"),
    ScrollView: el("div"),
    Pressable: el("button"),
    ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
    StatusBar: () => null,
    Modal: ({ children }: any) => React.createElement("div", null, children),
    Platform: { OS: "android", select: (o: Record<string, unknown>) => o.android },
    StyleSheet: {
      create: (o: unknown) => o,
      flatten: flattenStyle,
      absoluteFill: { position: "absolute", top: 0, left: 0, right: 0, bottom: 0 },
      absoluteFillObject: { position: "absolute", top: 0, left: 0, right: 0, bottom: 0 },
    },
    useWindowDimensions: () =>
      React.useSyncExternalStore(dimsStore.subscribe, dimsStore.get, dimsStore.get),
    Dimensions: { get: () => dimsStore.get(), addEventListener: () => ({ remove: () => undefined }) },
    BackHandler: { addEventListener: () => ({ remove: () => undefined }) },
    PanResponder: { create: () => ({ panHandlers: {} }) },
    AppState: { addEventListener: () => ({ remove: () => undefined }) },
  };
});

vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () =>
    React.useSyncExternalStore(insetsStore.subscribe, insetsStore.get, insetsStore.get),
}));

vi.mock("react-native-svg", () => ({
  default: ({ children }: any) => React.createElement("svg", null, children),
  Svg: ({ children }: any) => React.createElement("svg", null, children),
  Path: () => null,
  Text: ({ children }: any) => React.createElement("text", null, children),
}));

// --- expo ------------------------------------------------------------------------------
const lockAsyncMock = vi.fn(() => Promise.resolve());
vi.mock("expo-screen-orientation", () => ({
  lockAsync: (...a: unknown[]) => lockAsyncMock(...a),
  unlockAsync: vi.fn(() => Promise.resolve()),
  OrientationLock: { DEFAULT: 0, LANDSCAPE: 1, PORTRAIT_UP: 2 },
}));
vi.mock("expo-navigation-bar", () => ({ setVisibilityAsync: vi.fn(() => Promise.resolve()) }));
// ONE player instance for the whole screen, like the real useVideoPlayer: a
// fresh object per render would hand the track listener a new array identity on
// every pass and loop the screen forever.
const fakePlayer = {
  addListener: () => ({ remove: () => undefined }),
  playing: false,
  duration: 3600,
  currentTime: 0,
  volume: 1,
  muted: false,
  availableAudioTracks: [] as unknown[],
  play: () => undefined,
  pause: () => undefined,
};
vi.mock("expo-video", () => ({
  useVideoPlayer: () => fakePlayer,
  VideoView: () => null,
  isPictureInPictureSupported: () => false,
}));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: "media-1" }),
  router: { replace: vi.fn(), back: vi.fn(), canGoBack: () => true },
}));

// --- screen dependencies stubbed to keep this a layout test ------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { accent: "#f00", warning: "#fa0", text: "#fff", textMuted: "#888", bg: "#000", surface: "#111", border: "#333" },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, d?: unknown) => (typeof d === "string" ? d : k) }),
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "profile-1", accessToken: "tok" }),
  toast: { info: vi.fn(), error: vi.fn(), success: vi.fn() },
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: () => ({ data: { id: "media-1", type: "MEDIA_ITEM_TYPE_MOVIE", title: "Some Movie", durationMs: "3600000" } }),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] } }),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  useUserServiceGetCurrentUser: () => ({ data: { id: "user-1" } }),
  useUserServiceListProfiles: () => ({ data: { profiles: [] } }),
  createPlayerEventReporter: () => vi.fn(),
}));

vi.mock("../hooks/usePlaySession", () => ({
  usePlaySession: () => ({
    streamUrl: "http://server/stream.m3u8",
    streamToken: "tok",
    sessionId: "s1",
    streamInfo: { playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: [] },
    serverUrl: "http://server",
    isLoading: false,
    isError: false,
    shedReason: null,
    stopSession: vi.fn(),
    restartWithTracks: vi.fn(),
    progress: null,
  }),
}));
vi.mock("../hooks/useAudioCapsBeacon", () => ({ useAudioCapsBeacon: () => undefined }));
vi.mock("../hooks/useProgressReporter", () => ({ useProgressReporter: () => undefined }));
vi.mock("../hooks/usePlaybackSettings", () => ({ usePlaybackSettings: () => ({ data: { skipForwardSeconds: 30, skipBackwardSeconds: 30 } }) }));
vi.mock("../hooks/useChapters", () => ({ useChapters: () => ({ data: [] }) }));
vi.mock("../hooks/useNextEpisode", () => ({ useNextEpisode: () => ({ visible: false, nextEpisode: null, ended: false, cancel: vi.fn() }) }));
vi.mock("../hooks/useSleepTimer", () => ({ useSleepTimer: () => ({ state: null, setMinutes: vi.fn(), setEndOfEpisode: vi.fn(), cancel: vi.fn() }) }));
vi.mock("../hooks/usePlayerGestures", () => ({
  usePlayerGestures: () => ({
    panResponder: { panHandlers: {} },
    seekFlash: null,
    volumeIndicator: null,
    brightnessIndicator: null,
    cleanup: () => undefined,
  }),
}));
vi.mock("../hooks/useChromecast", () => ({ CastContext: null, CastButtonNative: null }));
vi.mock("../hooks/useCastSession", () => ({
  useCastSession: () => ({
    castConnected: false, castClient: null, castCurrentTime: 0, castDuration: 0,
    castIsPlaying: false, castVolume: 1, castIsMuted: false, castDeviceName: null, handleCast: vi.fn(),
  }),
}));
vi.mock("../hooks/useSyncPlay", () => ({
  useSyncPlay: () => ({
    state: { connected: false, roomCode: "", isHost: false, members: [], queue: [], trackIndex: 0 },
    createRoom: vi.fn(), joinByCode: vi.fn(), leaveRoom: vi.fn(),
    sendPlay: vi.fn(), sendPause: vi.fn(), sendSeek: vi.fn(), sendNext: vi.fn(), sendPrev: vi.fn(),
  }),
}));


vi.mock("../components/BottomSheet", () => ({ BottomSheet: () => null }));
vi.mock("../components/SleepTimerPicker", () => ({ SleepTimerPicker: () => null }));
vi.mock("../components/ChaptersPicker", () => ({ ChaptersPicker: () => null }));
vi.mock("../components/StreamInfoPanel", () => ({ StreamInfoPanel: () => null }));
vi.mock("../components/SkipIntroOverlay", () => ({ SkipIntroOverlay: () => null }));
vi.mock("../components/SyncPlayBar", () => ({ SyncPlayBar: () => null }));
vi.mock("../components/SyncPlayModal", () => ({ SyncPlayModal: () => null }));
vi.mock("../components/player/PlayerEpisodesPanel", () => ({ PlayerEpisodesPanel: () => null }));
vi.mock("../components/player/CastControllerScreen", () => ({ CastControllerScreen: () => null }));
vi.mock("../components/player/GestureIndicators", () => ({ GestureIndicators: () => null }));
vi.mock("../components/player/NextEpisodeOverlay", () => ({ NextEpisodeOverlay: () => null }));
vi.mock("../components/player/PlayerSeekBar", () => ({ PlayerSeekBar: () => null }));
vi.mock("../components/player/SubtitleOverlay", () => ({ SubtitleOverlay: () => null }));
vi.mock("../components/player/PlayerSettingsPanel", () => ({ PlayerSettingsPanel: () => null }));
vi.mock("../features/plugins/PluginSlot", () => ({ PluginSlot: () => null }));
vi.mock("../lib/audioBoost", () => ({ setAudioBoost: vi.fn(), releaseAudioBoost: vi.fn(() => Promise.resolve()) }));
vi.mock("../lib/deviceVolume", () => ({
  hasDeviceVolumeControl: () => false,
  readDeviceVolumeFraction: vi.fn(() => Promise.resolve(null)),
  readDeviceVolumeSteps: vi.fn(async () => 15),
  setDeviceVolumeFraction: vi.fn(),
}));
vi.mock("../lib/downloads", () => ({
  getDownloadPath: vi.fn(() => Promise.resolve(null)),
  getDownloadedMedia: vi.fn(() => Promise.resolve(null)),
  hasRenderableAudio: () => true,
}));
vi.mock("../lib/deviceCodecs", () => ({ getDeviceAudioCodecs: () => [] }));
vi.mock("../lib/subtitleStyle", () => ({
  DEFAULT_SUBTITLE_STYLE: { fontScale: 1 },
  getSubtitleStylePref: vi.fn(() => Promise.resolve(null)),
  saveSubtitleStylePref: vi.fn(),
}));
vi.mock("../lib/externalSubtitle", () => ({ pickExternalSubtitle: vi.fn() }));

import PlayerScreen from "../../app/(auth)/player/[id]";

// The bottom bar is the block the viewer watches jump: absolutely pinned to the
// bottom edge, padded from the safe-area insets.
function styledDivs(): { el: Element; style: Record<string, unknown> }[] {
  return Array.from(document.querySelectorAll("[data-style]")).map((el) => ({
    el,
    style: JSON.parse(el.getAttribute("data-style") || "{}") as Record<string, unknown>,
  }));
}

function findBottomBar() {
  return styledDivs().find(
    (d) => d.style.position === "absolute" && d.style.bottom === 0 && d.style.paddingBottom !== undefined,
  );
}

function findControlsOverlay() {
  return styledDivs().find((d) => d.style.backgroundColor === "rgba(0,0,0,0.4)");
}

interface Snapshot {
  painted: boolean;
  padding: string | null;
  volumeBarShown: boolean;
}

// What the eye actually sees: is the control layer painted at all, and with
// which geometry. `opacity: 0` / pointerEvents none counts as not painted.
function snapshot(): Snapshot {
  const overlay = findControlsOverlay();
  const bar = findBottomBar();
  const painted = !!overlay && overlay.style.opacity !== 0 && !!bar;
  return {
    painted,
    padding: bar
      ? `${bar.style.paddingTop ?? "-"}/${bar.style.paddingBottom}/${bar.style.paddingLeft}/${bar.style.paddingRight}`
      : null,
    // The volume slider + percentage only exist above the 600px breakpoint, so
    // they appear mid-rotation and push every icon on their right.
    volumeBarShown: !!document.body.textContent?.includes("%"),
  };
}

describe("player fullscreen layout", () => {
  let warnSpy: ReturnType<typeof vi.spyOn> | undefined;
  beforeEach(() => {
    dimsStore.set(PORTRAIT);
    insetsStore.set(PORTRAIT_INSETS);
    vi.useFakeTimers();
    // The screen reads the system brightness through a bare `require`, which
    // vi.mock cannot intercept and node cannot load here: the player's own
    // "not available" warning is the expected outcome, not a layout finding.
    const realWarn = console.warn.bind(console);
    warnSpy = vi.spyOn(console, "warn").mockImplementation((...args: unknown[]) => {
      if (args[0] === "expo-brightness not available") return;
      realWarn(...args);
    });
  });
  afterEach(() => {
    cleanup();
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
    warnSpy?.mockRestore();
  });

  // Mounting settles a couple of promises (download lookup, subtitle style),
  // so flush them before reading the first snapshot.
  async function mount() {
    await act(async () => { render(React.createElement(PlayerScreen)); });
  }

  it("paints the controls once when the window and the insets land in separate frames", async () => {
    await mount();
    const before = snapshot();
    expect(before.painted).toBe(true);

    // 1. the window rotates (Dimensions "change" reaches JS first)...
    act(() => { dimsStore.set(LANDSCAPE); });
    const afterResize = snapshot();

    // 2. ...then the safe-area insets arrive in their own native event.
    act(() => { insetsStore.set(LANDSCAPE_INSETS); });
    const afterInsets = snapshot();

    // 3. geometry settles.
    act(() => { vi.advanceTimersByTime(1000); });
    const settled = snapshot();

    expect(settled.painted).toBe(true);
    expect(settled).not.toEqual(before); // the rotation did change the layout

    // One step: every painted frame in between is either the portrait layout we
    // came from or the landscape one we end on. A painted third geometry IS the
    // "placement computed twice" being reported.
    for (const mid of [afterResize, afterInsets]) {
      if (mid.painted) expect([before, settled]).toContainEqual(mid);
    }
  });

  it("never paints the controls in an intermediate geometry when the insets land first", async () => {
    await mount();
    const before = snapshot();

    act(() => { insetsStore.set(LANDSCAPE_INSETS); });
    const afterInsets = snapshot();
    act(() => { dimsStore.set(LANDSCAPE); });
    const afterResize = snapshot();
    act(() => { vi.advanceTimersByTime(1000); });
    const settled = snapshot();

    expect(settled.painted).toBe(true);
    expect(settled).not.toEqual(before);
    for (const mid of [afterInsets, afterResize]) {
      if (mid.painted) expect([before, settled]).toContainEqual(mid);
    }
  });

  it("sizes the video stage from the native layout, not from a JS pixel measure", async () => {
    await mount();
    // A stage sized `width: screenW, height: screenH` can only follow the
    // rotation one JS commit AFTER the native views have already rotated, so
    // the whole overlay is drawn at the previous orientation's size in between.
    const pixelSized = styledDivs().filter(
      (d) => d.style.width === PORTRAIT.width && d.style.height === PORTRAIT.height,
    );
    expect(pixelSized).toHaveLength(0);
  });
});
