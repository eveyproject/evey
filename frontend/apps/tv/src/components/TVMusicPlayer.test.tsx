import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Image: ({ source }: any) => React.createElement("img", { alt: "", src: source?.uri }),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": String(!!hasTVPreferredFocus) }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
// The status is a tiny external store so a change re-renders the memoised
// player the way expo-audio's own hook would.
const audio = vi.hoisted(() => {
  type Status = { currentTime?: number; duration?: number; didJustFinish?: boolean; playing?: boolean };
  const listeners = new Set<() => void>();
  let status: Status = { currentTime: 30, duration: 120, didJustFinish: false };
  return {
    player: { replace: vi.fn(), play: vi.fn(), pause: vi.fn() },
    get: () => status,
    set: (next: Status) => {
      status = next;
      listeners.forEach((l) => l());
    },
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
});
vi.mock("expo-audio", async () => {
  const { useSyncExternalStore } = await import("react");
  return { useAudioPlayer: () => audio.player, useAudioPlayerStatus: () => useSyncExternalStore(audio.subscribe, audio.get) };
});
const keepAwake = vi.hoisted(() => ({ activate: vi.fn(async () => {}), deactivate: vi.fn(async () => {}) }));
vi.mock("expo-keep-awake", () => ({ activateKeepAwakeAsync: keepAwake.activate, deactivateKeepAwake: keepAwake.deactivate }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
// The store holds whatever the last API call left ("stale"); the player must
// ask the fetcher for a token that is valid NOW.
const freshToken = vi.hoisted(() => ({ fn: vi.fn(async () => "tok") }));
vi.mock("@evey/api", () => ({ getFreshAccessToken: freshToken.fn }));
vi.mock("@evey/store", async (orig) => ({
  ...(await orig<typeof import("@evey/store")>()),
  useAuthStore: Object.assign((sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "stale" }), { getState: () => ({ accessToken: "stale" }) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surfaceRaised: "#222", surface: "#111", border: "#333", accent: "#f00", text: "#fff", textMuted: "#888", onAccent: "#000" } }),
}));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("../lib/artwork", () => ({ coverArtUri: (s: string, u: string) => `${s}${u}` }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, lg: 18 }, ICON_SIZE: { xl: 28, "2xl": 36 } }));

import { TVMusicPlayer } from "./TVMusicPlayer";
import { useMusicQueueStore } from "../stores/musicQueue";

const tracks = [
  { id: "t1", title: "One", artist: "Band", album: "LP", duration: 100 },
  { id: "t2", title: "Two", artist: "Band", album: "LP", duration: 100 },
] as any[];

beforeEach(() => {
  vi.clearAllMocks();
  audio.set({ currentTime: 30, duration: 120, didJustFinish: false });
  useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false, shuffle: false });
});

// The global TV player follows the queue store: nothing without a track,
// the active track's stream is loaded once (with the token) and played when
// the store says so, a finished track advances the queue once, and close
// pauses and empties the queue.
describe("TVMusicPlayer", () => {
  it("renders nothing without a track", () => {
    const { container } = render(<TVMusicPlayer />);
    expect(container.innerHTML).toBe("");
  });

  it("loads the active track's stream with a fresh token, follows play/pause, and shows the progress", async () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    const { rerender } = render(<TVMusicPlayer />);
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalledWith("http://evey.local/api/v1/music/tracks/t1/stream?token=tok"));
    expect(audio.player.play).toHaveBeenCalled();
    expect(screen.getByText("One")).toBeTruthy();
    expect(screen.getByText("Band · LP")).toBeTruthy();
    expect(screen.getByLabelText("common.pause").getAttribute("data-preferred")).toBe("true");
    expect(document.querySelector("img")?.getAttribute("src")).toContain("/api/v1/music/tracks/t1/art?token=tok");

    fireEvent.click(screen.getByLabelText("common.pause"));
    rerender(<TVMusicPlayer />);
    expect(useMusicQueueStore.getState().isPlaying).toBe(false);
    expect(audio.player.pause).toHaveBeenCalled();
    expect(screen.getByLabelText("common.play")).toBeTruthy();
    expect(audio.player.replace).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByLabelText("common.next"));
    rerender(<TVMusicPlayer />);
    expect(screen.getByText("Two")).toBeTruthy();
    await waitFor(() => expect(audio.player.replace).toHaveBeenLastCalledWith("http://evey.local/api/v1/music/tracks/t2/stream?token=tok"));
    expect(freshToken.fn).toHaveBeenCalledTimes(2);
  });

  // An album outlives a 15-minute access token: the token pinned when a track
  // starts is the fresh one the fetcher hands back, never the store's leftover,
  // and the cover art carries the same one.
  it("pins the token the fetcher refreshed at track change, not the store's stale one", async () => {
    freshToken.fn.mockResolvedValueOnce("renewed");
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 1, isPlaying: true }));
    render(<TVMusicPlayer />);
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalledWith("http://evey.local/api/v1/music/tracks/t2/stream?token=renewed"));
    expect(document.querySelector("img")?.getAttribute("src")).toContain("/api/v1/music/tracks/t2/art?token=renewed");
    expect(document.querySelector("img")?.getAttribute("src")).not.toContain("stale");
  });

  it("advances once when a track finishes, and close pauses and empties the queue", async () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    const { rerender } = render(<TVMusicPlayer />);
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalled());
    act(() => audio.set({ currentTime: 100, duration: 100, didJustFinish: true }));
    expect(useMusicQueueStore.getState().currentIndex).toBe(1);
    // The flag staying true across renders must not skip a second track.
    act(() => audio.set({ currentTime: 100, duration: 100, didJustFinish: true }));
    expect(useMusicQueueStore.getState().currentIndex).toBe(1);

    fireEvent.click(screen.getByLabelText("common.close"));
    expect(audio.player.pause).toHaveBeenCalled();
    expect(useMusicQueueStore.getState().queue).toEqual([]);
    rerender(<TVMusicPlayer />);
    expect(screen.queryByText("Two")).toBeNull();
  });

  // A film opening clears the queue without touching the bar's close
  // button. The player still holds the album's last source: the pause has
  // to follow the queue emptying, or the album plays on under the film.
  it("silences the player when the queue is cleared from outside the bar", async () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    render(<TVMusicPlayer />);
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalled());
    expect(audio.player.pause).not.toHaveBeenCalled();
    act(() => useMusicQueueStore.getState().clear());
    expect(audio.player.pause).toHaveBeenCalled();
  });

  // The box treats music as an idle user: the screen is held only while the
  // player reports sound flowing, and released when it pauses or finishes.
  it("holds the screen awake while sound flows and releases it when it stops", () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    render(<TVMusicPlayer />);
    expect(keepAwake.activate).not.toHaveBeenCalled();

    act(() => audio.set({ currentTime: 30, duration: 120, playing: true }));
    expect(keepAwake.activate).toHaveBeenCalledTimes(1);
    expect(keepAwake.deactivate).not.toHaveBeenCalled();

    act(() => audio.set({ currentTime: 31, duration: 120, playing: true }));
    expect(keepAwake.activate).toHaveBeenCalledTimes(1);

    act(() => audio.set({ currentTime: 31, duration: 120, playing: false }));
    expect(keepAwake.deactivate).toHaveBeenCalledTimes(1);

    act(() => audio.set({ currentTime: 120, duration: 120, playing: false, didJustFinish: true }));
    expect(keepAwake.activate).toHaveBeenCalledTimes(1);
  });
});

// The bar carries the shuffle toggle the phone and the browser already had
// (owner, 2026-09-22): pressing it flips the queue's mode, and the button
// reads as on.
describe("TVMusicPlayer shuffle", () => {
  it("toggles the queue's shuffle mode from the bar", async () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    const { rerender } = render(<TVMusicPlayer />);
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalled());
    const btn = screen.getByLabelText("media.music.shuffle");
    expect(btn.getAttribute("aria-selected") ?? btn.getAttribute("data-selected")).not.toBe("true");
    fireEvent.click(btn);
    rerender(<TVMusicPlayer />);
    expect(useMusicQueueStore.getState().shuffle).toBe(true);
    fireEvent.click(screen.getByLabelText("media.music.shuffle"));
    expect(useMusicQueueStore.getState().shuffle).toBe(false);
  });
});
