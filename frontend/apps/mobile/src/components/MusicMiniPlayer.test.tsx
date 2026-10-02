import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

// The mini-player showed a bare progress bar with no numeric elapsed/duration
// readout, unlike web's GlobalAudioPlayer (which flanks its seek bar with
// formatTime(currentTime) / formatTime(duration)). This proves the mobile
// mini-player now shows the same MM:SS / MM:SS readout, reusing the shared
// @evey/api formatter rather than a local copy.

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  AppState: { currentState: "active", addEventListener: () => ({ remove: vi.fn() }) },
  Platform: { OS: "android", select: (obj: any) => obj.android },
}));

// --- expo-image / expo-router / expo-audio -----------------------------------------------
vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({
  router: { push: vi.fn() },
}));

let audioStatus: { currentTime?: number; duration?: number; playing?: boolean; didJustFinish?: boolean; isLoaded?: boolean } = {
  currentTime: 65,
  duration: 245,
  playing: true,
  didJustFinish: false,
  isLoaded: true,
};
const audioPlayer = { replace: vi.fn(), play: vi.fn(), pause: vi.fn(), seekTo: vi.fn() };
vi.mock("expo-audio", () => ({
  useAudioPlayer: () => audioPlayer,
  useAudioPlayerStatus: () => audioStatus,
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surfaceRaised: "#111", border: "#333", text: "#fff",
      textMuted: "#888", accent: "#f00",
    },
  }),
}));

// --- misc hooks/libs not under test -----------------------------------------------------
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../lib/musicScrobble", () => ({ shouldRecordScrobble: () => false }));

// --- @evey/store -------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ accessToken: "tok-1" }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

// --- music queue store -------------------------------------------------------------------
const track = { id: "trk-1", title: "Some Track", artist: "Some Artist", album: "Some Album", duration: 245 };
let queueState = {
  queue: [track],
  currentIndex: 0,
  isPlaying: true,
  shuffle: false,
  source: null as unknown,
  togglePlay: vi.fn(),
  toggleShuffle: vi.fn(),
  next: vi.fn(),
  previous: vi.fn(),
  clear: vi.fn(),
};
vi.mock("../stores/musicQueue", () => ({
  useMusicQueueStore: (selector: (s: typeof queueState) => unknown) => selector(queueState),
}));

// --- @evey/api ----------------------------------------------------------------------------
// formatTime is real (pure, no network) - only the fetchers are stubbed. The
// remote-command poll is held open by default, as the server holds it.
const freshToken = vi.hoisted(() => ({ fn: vi.fn(async () => "renewed") }));
const commandPoll = vi.hoisted(() => ({ fn: vi.fn((_req: { url: string }) => new Promise<Response>(() => {})) }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn().mockResolvedValue({}),
  eveyFetchRaw: commandPoll.fn,
  getFreshAccessToken: freshToken.fn,
}));

import { MusicMiniPlayer } from "./MusicMiniPlayer";

beforeEach(() => {
  vi.clearAllMocks();
  audioStatus = { currentTime: 65, duration: 245, playing: true, didJustFinish: false, isLoaded: true };
  queueState = {
    queue: [track],
    currentIndex: 0,
    isPlaying: true,
    shuffle: false,
    source: null,
    togglePlay: vi.fn(),
    toggleShuffle: vi.fn(),
    next: vi.fn(),
    previous: vi.fn(),
    clear: vi.fn(),
  };
});

describe("MusicMiniPlayer - elapsed/duration readout", () => {
  it("shows the current position and track duration as MM:SS", () => {
    render(<MusicMiniPlayer />);

    expect(screen.getByText("1:05")).toBeTruthy();
    expect(screen.getByText("4:05")).toBeTruthy();
  });

  it("shows a dash for duration before the player has one", () => {
    audioStatus = { currentTime: 0, duration: 0, playing: false, didJustFinish: false, isLoaded: false };
    queueState = { ...queueState, queue: [{ ...track, duration: 0 }] };

    render(<MusicMiniPlayer />);

    expect(screen.getByText("0:00")).toBeTruthy();
    expect(screen.getByText("-")).toBeTruthy();
  });
});

// An album outlives a 15-minute access token: the stream and the cover art
// carry the token the fetcher refreshed at track change, never the store's
// leftover ("tok-1" above).
describe("MusicMiniPlayer - stream credential", () => {
  it("pins a fresh token per track instead of the store's stale one", async () => {
    render(<MusicMiniPlayer />);
    await waitFor(() => expect(audioPlayer.replace).toHaveBeenCalledWith("http://test-server/api/v1/music/tracks/trk-1/stream?token=renewed"));
    expect(document.querySelector("img")?.getAttribute("src")).toContain("/art?token=renewed");
    expect(freshToken.fn).toHaveBeenCalledTimes(1);
  });
});

// The video player clears the queue when it opens; the mini-player's close
// button is not involved, so the pause has to follow the queue emptying or
// the song plays on under the film. The store mock here is not a
// subscription and the player is memoised, so the emptied queue is shown to
// a fresh mount rather than through a re-render.
describe("MusicMiniPlayer - queue cleared from outside", () => {
  it("pauses the player it holds when there is no track to play", async () => {
    const { rerender } = render(<MusicMiniPlayer key="playing" />);
    await waitFor(() => expect(audioPlayer.replace).toHaveBeenCalled());
    expect(audioPlayer.pause).not.toHaveBeenCalled();
    queueState = { ...queueState, queue: [], currentIndex: -1, isPlaying: false };
    rerender(<MusicMiniPlayer key="cleared" />);
    expect(audioPlayer.pause).toHaveBeenCalled();
  });
});

// The admin's pause reaches the phone on the command long poll, not on the
// next 10s heartbeat.
describe("MusicMiniPlayer - admin remote command", () => {
  it("pauses on a command delivered by the poll", async () => {
    commandPoll.fn.mockImplementationOnce(async (req) => {
      expect(req.url).toMatch(/^\/api\/v1\/music\/nowplaying\/music-mobile-.+\/command$/);
      return new Response(JSON.stringify({ remoteCommand: "pause" }), { status: 200 });
    });
    render(<MusicMiniPlayer />);
    await waitFor(() => expect(queueState.togglePlay).toHaveBeenCalledTimes(1));
  });
});
