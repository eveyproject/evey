import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// The guest player (mirror of web's GuestWatch): no guest session sends the
// viewer back to login; with one, the media plays on the forced transcode
// path with the guest token as bearer, the room is joined exactly once, the
// host's play/pause/seek land on the native player, the guest's own
// play/pause is announced to the room only while connected, and leaving
// forgets the guest session.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StatusBar: () => null,
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s, absoluteFill: {}, absoluteFillObject: {} },
}));
vi.mock("react-native-safe-area-context", () => ({ useSafeAreaInsets: () => ({ top: 0, bottom: 0 }) }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("expo-router", () => ({ router, useLocalSearchParams: () => ({ id: "m1" }) }));
const player = vi.hoisted(() => ({
  playing: false, currentTime: 0, duration: 100, timeUpdateEventInterval: 0,
  play: vi.fn(), pause: vi.fn(),
  addListener: vi.fn(() => ({ remove: vi.fn() })),
  source: null as unknown,
}));
vi.mock("expo-video", () => ({
  useVideoPlayer: (source: unknown, setup?: (p: typeof player) => void) => {
    player.source = source;
    setup?.(player);
    return player;
  },
  VideoView: () => React.createElement("div", { "data-testid": "video" }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-screen-orientation", () => ({ unlockAsync: vi.fn(), lockAsync: vi.fn(), OrientationLock: { DEFAULT: 0 } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${s}s` }));
vi.mock("../../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00", text: "#fff" } }) }));
vi.mock("../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));
vi.mock("../../src/hooks/usePlaySession", () => ({
  buildStreamUrl: (server: string, id: string, info: { playMethod: string }, token: string) => `${server}/stream/${id}?m=${info.playMethod}&t=${token}`,
}));
const sync = vi.hoisted(() => ({
  opts: null as any,
  state: { roomId: null as string | null, roomCode: null as string | null, members: [] as unknown[], connected: false, isHost: false },
  joinRoom: vi.fn(async () => {}), leaveRoom: vi.fn(async () => {}), sendPlay: vi.fn(), sendPause: vi.fn(),
}));
vi.mock("../../src/hooks/useSyncPlay", () => ({
  useSyncPlay: (o: any) => {
    sync.opts = o;
    return { state: sync.state, joinRoom: sync.joinRoom, leaveRoom: sync.leaveRoom, sendPlay: sync.sendPlay, sendPause: sync.sendPause };
  },
}));
vi.mock("../../src/components/SyncPlayBar", () => ({
  SyncPlayBar: ({ roomCode, onLeave }: any) => React.createElement("div", { "data-testid": "bar" }, roomCode, React.createElement("button", { onClick: onLeave }, "leave")),
}));
const guest = vi.hoisted(() => ({ session: null as { token: string; roomId: string } | null, cleared: 0 }));
vi.mock("../../src/features/syncplay/guestSession", () => ({
  readGuestSession: () => guest.session,
  clearGuestSession: () => { guest.cleared++; },
}));

import GuestWatch from "./[id]";

beforeEach(() => {
  vi.clearAllMocks();
  guest.session = { token: "gtok", roomId: "room-1" };
  guest.cleared = 0;
  sync.state = { roomId: null, roomCode: null, members: [], connected: false, isHost: false };
  player.playing = false;
  player.currentTime = 0;
  player.source = null;
});

describe("guest watch screen (mobile)", () => {
  it("sends a guest without a session back to login and plays nothing", () => {
    guest.session = null;
    const { container } = render(<GuestWatch />);
    expect(container.firstChild).toBeNull();
    expect(router.replace).toHaveBeenCalledWith("/login");
    expect(sync.joinRoom).not.toHaveBeenCalled();
    expect(player.source).toBeNull();
  });

  it("plays on the forced transcode path with the guest bearer and joins the room once", () => {
    const { rerender } = render(<GuestWatch />);
    expect(player.source).toEqual({
      uri: "https://s/stream/m1?m=PLAY_METHOD_TRANSCODE&t=gtok", contentType: "hls", headers: { Authorization: "Bearer gtok" },
    });
    expect(player.play).toHaveBeenCalled();
    expect(sync.opts.guestToken).toBe("gtok");
    rerender(<GuestWatch />);
    expect(sync.joinRoom).toHaveBeenCalledTimes(1);
    expect(sync.joinRoom).toHaveBeenCalledWith("room-1");
    expect(screen.queryByTestId("bar")).toBeNull();
  });

  it("applies the host's commands, announces its own only while connected, and leaving forgets the guest", () => {
    render(<GuestWatch />);
    act(() => sync.opts.onPlay(90_000));
    expect(player.currentTime).toBe(90);
    act(() => sync.opts.onPause(120_000));
    expect(player.currentTime).toBe(120);
    expect(player.pause).toHaveBeenCalledTimes(1);
    act(() => sync.opts.onSeek(5_000));
    expect(player.currentTime).toBe(5);

    // Not connected: a local pause is not broadcast. The label follows the
    // player's events (none fired here), the action follows its live state.
    player.playing = true;
    fireEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(player.pause).toHaveBeenCalledTimes(2);
    expect(sync.sendPause).not.toHaveBeenCalled();
  });

  it("broadcasts play and pause once connected, and leaving forgets the guest session", () => {
    sync.state = { roomId: "room-1", roomCode: "ABC123", members: [], connected: true, isHost: false };
    render(<GuestWatch />);
    expect(screen.getByTestId("bar").textContent).toContain("ABC123");
    player.playing = false;
    player.currentTime = 7;
    fireEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(sync.sendPlay).toHaveBeenCalledWith(7000);
    fireEvent.click(screen.getByText("leave"));
    expect(sync.leaveRoom).toHaveBeenCalledTimes(1);
    expect(guest.cleared).toBe(1);
    expect(router.replace).toHaveBeenCalledWith("/login");
  });
});
