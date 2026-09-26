import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { SyncPlayBar } from "./SyncPlayBar";
import type { SyncPlayMember } from "./useSyncPlay";

const members: SyncPlayMember[] = [
  { userId: "u1", username: "Alice" },
  { userId: "u2", username: "Bob" },
];

describe("SyncPlayBar", () => {
  const onLeave = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    // navigator.clipboard is a prototype getter: define an own property over it.
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });
  });

  it("renders member names", () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={members} connected onLeave={onLeave} />,
    );
    expect(screen.getByText(/Watching with: Alice, Bob/)).toBeInTheDocument();
  });

  it("shows fallback text when no members", () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={[]} connected onLeave={onLeave} />,
    );
    expect(screen.getByText("Watch Together room")).toBeInTheDocument();
  });

  it("truncates to first 3 members and shows +N more", () => {
    const many: SyncPlayMember[] = [
      { userId: "u1", username: "Alice" },
      { userId: "u2", username: "Bob" },
      { userId: "u3", username: "Carol" },
      { userId: "u4", username: "Dave" },
      { userId: "u5", username: "Eve" },
    ];
    render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={many} connected onLeave={onLeave} />,
    );
    expect(screen.getByText(/Alice, Bob, Carol \+2 more/)).toBeInTheDocument();
  });

  it("calls onLeave when Leave button is clicked", () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={members} connected onLeave={onLeave} />,
    );
    fireEvent.click(screen.getByText("Leave"));
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("copies share URL when Share link is clicked and shows 'Copied!'", async () => {
    render(
      <SyncPlayBar roomId="room-abc" roomCode={null} isHost={false} members={members} connected onLeave={onLeave} />,
    );
    fireEvent.click(screen.getByText("Share link"));
    await waitFor(() => expect(screen.getByText("Copied!")).toBeInTheDocument());
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      expect.stringContaining("room=room-abc"),
    );
  });

  it("shows connected indicator via title", () => {
    const { container } = render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={members} connected onLeave={onLeave} />,
    );
    const dot = container.querySelector("[title='Connected']");
    expect(dot).toBeInTheDocument();
  });

  it("shows disconnected indicator when not connected", () => {
    const { container } = render(
      <SyncPlayBar roomId="room-1" roomCode={null} isHost={false} members={members} connected={false} onLeave={onLeave} />,
    );
    const dot = container.querySelector("[title='Connecting...']");
    expect(dot).toBeInTheDocument();
  });

  it("shows room code button when host", () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode="A3F7K9" isHost members={members} connected onLeave={onLeave} />,
    );
    expect(screen.getByText("A3F7K9")).toBeInTheDocument();
  });

  it("does not show room code when not host", () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode="A3F7K9" isHost={false} members={members} connected onLeave={onLeave} />,
    );
    expect(screen.queryByText("A3F7K9")).not.toBeInTheDocument();
  });

  it("copies room code when code button clicked", async () => {
    render(
      <SyncPlayBar roomId="room-1" roomCode="A3F7K9" isHost members={members} connected onLeave={onLeave} />,
    );
    fireEvent.click(screen.getByText("A3F7K9"));
    await waitFor(() => expect(screen.getByText("Copied!")).toBeInTheDocument());
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("A3F7K9");
  });
});
