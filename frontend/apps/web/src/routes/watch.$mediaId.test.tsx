import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const params = vi.hoisted(() => ({ mediaId: "m1" }));
const navigate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => ({ ...opts, useParams: () => params }),
  useNavigate: () => navigate,
}));
const guest = vi.hoisted(() => ({ session: null as { token: string; roomId: string } | null, cleared: 0 }));
vi.mock("@/features/syncplay/guestSession", () => ({
  readGuestSession: () => guest.session,
  clearGuestSession: () => { guest.cleared++; },
}));
// The sync hook is a recorder: the page's callbacks are captured so the test
// can play the host's commands into the video element.
const sync = vi.hoisted(() => ({
  opts: null as any,
  state: { roomId: null as string | null, roomCode: null as string | null, members: [] as unknown[], connected: false, isHost: false },
  controls: { joinRoom: vi.fn(async () => {}), leaveRoom: vi.fn(async () => {}), sendResync: vi.fn(), sendCatchUp: vi.fn() },
}));
vi.mock("@/features/syncplay/useSyncPlay", () => ({
  useSyncPlay: (o: any) => {
    sync.opts = o;
    return [sync.state, sync.controls];
  },
}));
const player = vi.hoisted(() => ({ props: null as any }));
vi.mock("@/features/playback/HLSPlayer", () => ({
  HLSPlayer: (p: any) => {
    player.props = p;
    return React.createElement("video", { "data-testid": "video", ref: (el: HTMLVideoElement | null) => p.onVideoRef(el) });
  },
}));
vi.mock("@/features/syncplay/SyncPlayBar", () => ({
  SyncPlayBar: (p: any) =>
    React.createElement("div", { "data-testid": "bar", "data-room": p.roomId },
      React.createElement("button", { onClick: p.onLeave }, "leave"),
      React.createElement("button", { onClick: p.onResync }, "resync"),
      React.createElement("button", { onClick: p.onCatchUp }, "catchup")),
}));

import { Route } from "./watch.$mediaId.lazy";
const Page = (Route as unknown as { component: React.ComponentType }).component;

beforeEach(() => {
  vi.clearAllMocks();
  guest.session = { token: "guest-tok", roomId: "room-1" };
  guest.cleared = 0;
  sync.state = { roomId: null, roomCode: null, members: [], connected: false, isHost: false };
  player.props = null;
});

// A guest without a session is sent back to login; with one, the shared
// title plays on the forced transcode path with the guest token, the room is
// joined exactly once, the host's play/pause/seek land on the video element,
// and leaving forgets the guest session.
describe("guest watch page", () => {
  it("sends a guest without a session back to login and renders nothing", () => {
    guest.session = null;
    const { container } = render(<Page />);
    expect(container.firstChild).toBeNull();
    expect(navigate).toHaveBeenCalledWith({ to: "/login" });
    expect(sync.controls.joinRoom).not.toHaveBeenCalled();
  });

  it("plays on the forced transcode path with the guest token and joins the room once", async () => {
    render(<Page />);
    expect(player.props.streamInfo).toEqual({ playMethod: "PLAY_METHOD_TRANSCODE" });
    expect(player.props.streamToken).toBe("guest-tok");
    expect(player.props.mediaId).toBe("m1");
    expect(await player.props.onStreamTokenRefresh()).toBe("guest-tok");
    expect(sync.controls.joinRoom).toHaveBeenCalledTimes(1);
    expect(sync.controls.joinRoom).toHaveBeenCalledWith("room-1");
    expect(sync.opts.guestToken).toBe("guest-tok");
    expect(screen.queryByTestId("bar")).toBeNull();
  });

  it("applies the host's play, pause and seek to the video", () => {
    render(<Page />);
    const video = screen.getByTestId("video") as HTMLVideoElement;
    Object.defineProperty(video, "paused", { value: true, writable: true });
    video.play = vi.fn(async () => {});
    video.pause = vi.fn();
    act(() => sync.opts.onPlay(90_000));
    expect(video.currentTime).toBe(90);
    expect(video.play).toHaveBeenCalledTimes(1);
    act(() => sync.opts.onPause(120_500));
    expect(video.currentTime).toBe(120.5);
    expect(video.pause).toHaveBeenCalledTimes(1);
    act(() => sync.opts.onSeek(5_000));
    expect(video.currentTime).toBe(5);
  });

  it("shows the room bar once joined; leaving forgets the guest and returns to login", () => {
    sync.state = { ...sync.state, roomId: "room-1", roomCode: "ABCD", connected: true };
    render(<Page />);
    const video = screen.getByTestId("video") as HTMLVideoElement;
    video.currentTime = 12.34;
    expect(screen.getByTestId("bar").getAttribute("data-room")).toBe("room-1");
    fireEvent.click(screen.getByText("resync"));
    expect(sync.controls.sendResync).toHaveBeenCalledWith(12340);
    fireEvent.click(screen.getByText("catchup"));
    expect(sync.controls.sendCatchUp).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("leave"));
    expect(sync.controls.leaveRoom).toHaveBeenCalledTimes(1);
    expect(guest.cleared).toBe(1);
    expect(navigate).toHaveBeenCalledWith({ to: "/login" });
  });
});
