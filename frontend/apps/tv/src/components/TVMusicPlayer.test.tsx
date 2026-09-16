import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Image: ({ source }: any) => React.createElement("img", { alt: "", src: source?.uri }),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": String(!!hasTVPreferredFocus) }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons", () => ({ Ionicons: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
// The status is a tiny external store so a change re-renders the memoised
// player the way expo-audio's own hook would.
const audio = vi.hoisted(() => {
  type Status = { currentTime?: number; duration?: number; didJustFinish?: boolean };
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
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }) }));
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
  useMusicQueueStore.setState({ queue: [], currentIndex: -1, isPlaying: false });
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

  it("loads the active track's stream with the token, follows play/pause, and shows the progress", () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    const { rerender } = render(<TVMusicPlayer />);
    expect(audio.player.replace).toHaveBeenCalledWith("http://evey.local/api/v1/music/tracks/t1/stream?token=tok");
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
    expect(audio.player.replace).toHaveBeenLastCalledWith("http://evey.local/api/v1/music/tracks/t2/stream?token=tok");
  });

  it("advances once when a track finishes, and close pauses and empties the queue", () => {
    act(() => useMusicQueueStore.setState({ queue: tracks, currentIndex: 0, isPlaying: true }));
    const { rerender } = render(<TVMusicPlayer />);
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
});
