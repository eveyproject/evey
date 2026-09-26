import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children, testID }: any) => React.createElement("span", { "data-testid": testID }, children),
  Pressable: ({ children, onPress, disabled, testID, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: string) => f ?? k }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", surface: "#111", success: "#0f0", error: "#f00" }, radii: { sm: 4, md: 8 } }),
}));
vi.mock("../../hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv" }));
const audio = vi.hoisted(() => ({
  player: { replace: vi.fn(), play: vi.fn(), pause: vi.fn(), seekTo: vi.fn() },
  status: { currentTime: 12.5, playing: false, didJustFinish: false } as { currentTime: number; playing: boolean; didJustFinish: boolean },
}));
vi.mock("expo-audio", () => ({ useAudioPlayer: () => audio.player, useAudioPlayerStatus: () => audio.status }));
vi.mock("@evey/api", () => ({
  getFreshAccessToken: () => Promise.resolve("tok"),
  musicServiceGetTrack: () => Promise.resolve({ track: { title: "Some Track", artist: "Some Artist" } }),
}));

import { ListenTogetherRoom } from "./ListenTogetherRoom";

const controls = { sendPlay: vi.fn(), sendPause: vi.fn(), sendNext: vi.fn(), sendPrev: vi.fn(), sendSetIndex: vi.fn() };
function room(isHost: boolean, trackIndex = 0) {
  const state = { roomId: "r1", roomCode: "ABC123", members: [], connected: true, isHost, queue: ["t1", "t2", "t3"], trackIndex } as any;
  return React.createElement(ListenTogetherRoom, {
    state, controls: { ...controls }, playerRef: { current: null }, trackMeta: { current: new Map() }, onLeave: vi.fn(),
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  audio.status = { currentTime: 12.5, playing: false, didJustFinish: false };
});

describe("ListenTogetherRoom", () => {
  it("loads the queue's current track and lets the host drive the room", async () => {
    render(room(true));
    await waitFor(() => expect(audio.player.replace).toHaveBeenCalledWith("http://srv/api/v1/music/tracks/t1/stream?token=tok"));
    fireEvent.click(screen.getByTestId("room-play-pause"));
    expect(audio.player.play).toHaveBeenCalled();
    expect(controls.sendPlay).toHaveBeenCalledWith(12500);
    expect(screen.getByTestId("room-code").textContent).toBe("ABC123");
  });

  // The room echoes "next" before the new track has loaded: the index moves
  // while didJustFinish is still up, and a second "next" would skip a track.
  it("advances once when a track ends, even when the room's echo moves the index first", async () => {
    const view = render(room(true, 0));
    audio.status = { currentTime: 200, playing: false, didJustFinish: true };
    view.rerender(room(true, 0));
    view.rerender(room(true, 1));
    expect(controls.sendNext).toHaveBeenCalledTimes(1);
    audio.status = { currentTime: 0, playing: true, didJustFinish: false };
    view.rerender(room(true, 1));
    audio.status = { currentTime: 200, playing: false, didJustFinish: true };
    view.rerender(room(true, 1));
    expect(controls.sendNext).toHaveBeenCalledTimes(2);
  });

  it("a member follows: no transport controls, no auto-advance", () => {
    const view = render(room(false));
    expect(screen.queryByTestId("room-play-pause")).toBeNull();
    expect(screen.getByText("media.syncplay.followingHost")).toBeTruthy();
    audio.status = { currentTime: 200, playing: false, didJustFinish: true };
    view.rerender(room(false));
    expect(controls.sendNext).not.toHaveBeenCalled();
  });
});
