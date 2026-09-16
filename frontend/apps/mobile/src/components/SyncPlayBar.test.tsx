import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";

const share = vi.hoisted(() => ({ share: vi.fn() }));
vi.mock("react-native", () => ({
  View: ({ children, style }: any) => React.createElement("div", { "data-bg": Array.isArray(style) ? style.map((s: any) => s?.backgroundColor).find(Boolean) : undefined }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  Share: share,
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, f?: unknown) => {
      if (k === "media.syncplay.watchingWith") return `with ${(f as { names: string }).names}`;
      if (k === "media.syncplay.watchingWithMore") return `with ${(f as { names: string }).names} +${(f as { count: number }).count}`;
      return typeof f === "string" ? f : k;
    },
  }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { overlay: "#0008", success: "#0a0", error: "#f33", text: "#fff", textMuted: "#888", surfaceRaised: "#222" } }),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10 }, ICON_SIZE: { sm: 16 } }));

import { SyncPlayBar } from "./SyncPlayBar";

const members = (n: number) => Array.from({ length: n }, (_, i) => ({ userId: `u${i}`, username: `user${i}` })) as any[];

beforeEach(() => {
  vi.clearAllMocks();
  share.share.mockResolvedValue({});
});

// The bar names up to three companions (+N beyond), shows the connection as a
// dot, gives the HOST the queue chips and the shareable code (a tap shares it
// and says "copied" for two seconds), and everyone the leave chip.
describe("SyncPlayBar", () => {
  it("names the companions and the connection state", () => {
    const { rerender } = render(<SyncPlayBar roomCode="ABC123" isHost={false} members={members(2)} connected onLeave={vi.fn()} />);
    expect(screen.getByText("with user0, user1")).toBeTruthy();
    expect(document.querySelector("[data-bg='#0a0']")).toBeTruthy();
    rerender(<SyncPlayBar roomCode="ABC123" isHost={false} members={members(5)} connected={false} onLeave={vi.fn()} />);
    expect(screen.getByText("with user0, user1, user2 +2")).toBeTruthy();
    expect(document.querySelector("[data-bg='#f33']")).toBeTruthy();
    rerender(<SyncPlayBar roomCode={null} isHost={false} members={[]} connected onLeave={vi.fn()} />);
    expect(screen.getByText("Watch together room")).toBeTruthy();
  });

  it("the host gets the queue chips clamped at both ends, and shares the code", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const onNext = vi.fn();
    const onPrev = vi.fn();
    const onLeave = vi.fn();
    render(<SyncPlayBar roomCode="ABC123" isHost members={members(1)} connected onLeave={onLeave} queue={["a", "b", "c"]} trackIndex={0} onNext={onNext} onPrev={onPrev} />);
    expect(screen.getByText("1/3")).toBeTruthy();
    expect((screen.getByLabelText("Previous") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByLabelText("Next"));
    expect(onNext).toHaveBeenCalled();
    fireEvent.click(screen.getByText("ABC123"));
    await waitFor(() => expect(share.share).toHaveBeenCalledWith({ message: "ABC123" }));
    await waitFor(() => expect(screen.getByText("Copied")).toBeTruthy());
    act(() => vi.advanceTimersByTime(2000));
    expect(screen.getByText("ABC123")).toBeTruthy();
    fireEvent.click(screen.getByText("Leave"));
    expect(onLeave).toHaveBeenCalled();
    vi.useRealTimers();
  });

  it("a guest only sees the position and the leave chip", () => {
    render(<SyncPlayBar roomCode="ABC123" isHost={false} members={members(1)} connected onLeave={vi.fn()} queue={["a", "b"]} trackIndex={1} />);
    expect(screen.getByText("2/2")).toBeTruthy();
    expect(screen.queryByLabelText("Next")).toBeNull();
    expect(screen.queryByText("ABC123")).toBeNull();
    expect(screen.getByText("Leave")).toBeTruthy();
  });
});
