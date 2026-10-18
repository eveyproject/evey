import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

function flat(style: unknown): Record<string, unknown> {
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flat));
  return style && typeof style === "object" ? (style as Record<string, unknown>) : {};
}
vi.mock("react-native", () => ({
  View: ({ children, style }: any) => React.createElement("div", { "data-left": flat(style).left, "data-w": flat(style).width }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled, style }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-left": flat(style).left, "data-w": flat(style).width, "data-bg": flat(style).backgroundColor }, children),
  StyleSheet: { create: (s: any) => s },
}));
const video = vi.hoisted(() => {
  const listeners = new Set<(ev: { currentTime: number }) => void>();
  return {
    player: {
      currentTime: 0,
      duration: 0,
      // expo-video's default: 0 means the native player emits no timeUpdate at all.
      timeUpdateEventInterval: 0,
      pause: vi.fn(),
      addListener: (_: string, fn: (ev: { currentTime: number }) => void) => {
        listeners.add(fn);
        return { remove: () => listeners.delete(fn) };
      },
    },
    tick(sec: number) {
      if (this.player.timeUpdateEventInterval > 0) listeners.forEach((l) => l({ currentTime: sec }));
    },
  };
});
vi.mock("expo-video", () => ({
  useVideoPlayer: (_url: string, setup?: (p: unknown) => void) => {
    setup?.(video.player);
    return video.player;
  },
  VideoView: () => React.createElement("div", { "data-testid": "video" }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => (typeof f === "string" ? f : _k) }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { border: "#333", text: "#fff", textMuted: "#888", surfaceRaised: "#222", accent: "#f00" }, radii: { md: 8 } }),
}));

import { SegmentPlayer } from "./SegmentPlayer";

const segments = [
  { type: "intro", startMs: 10_000, endMs: 40_000 },
  { type: "credits", startMs: 100_000, endMs: 120_000 },
];

beforeEach(() => {
  vi.clearAllMocks();
  video.player.currentTime = 0;
  video.player.duration = 0;
  video.player.timeUpdateEventInterval = 0;
});

// The editor never auto-plays, marks from the live playhead only once a
// segment is selected, draws each segment over the known duration (the
// player's once it reports one, the catalogue's until then), and tapping a
// segment selects it and seeks to its start.
describe("SegmentPlayer", () => {
  it("pauses on load, draws the timeline over the fallback duration, and seeks on a segment tap", () => {
    const onSelectType = vi.fn();
    const onMark = vi.fn();
    render(<SegmentPlayer streamUrl="http://x/stream" durationMsFallback={200_000} segments={segments} selectedType={null} onSelectType={onSelectType} onMark={onMark} />);
    expect(video.player.pause).toHaveBeenCalled();
    expect(video.player.timeUpdateEventInterval).toBeGreaterThan(0);
    expect(screen.getByText("03:20")).toBeTruthy();
    const bars = Array.from(document.querySelectorAll("button[data-w]"));
    expect(bars.map((b) => [b.getAttribute("data-left"), b.getAttribute("data-w")])).toEqual([["5%", "15%"], ["50%", "10%"]]);
    expect(bars[0].getAttribute("data-bg")).toBe("#f0055");
    fireEvent.click(bars[1]);
    expect(onSelectType).toHaveBeenCalledWith("credits");
    expect(video.player.currentTime).toBe(100);

    const mark = screen.getByText("Mark start").closest("button") as HTMLButtonElement;
    expect(mark.disabled).toBe(true);
    expect(screen.getByText("Select a segment to mark")).toBeTruthy();
    fireEvent.click(mark);
    expect(onMark).not.toHaveBeenCalled();
  });

  it("marks from the live playhead once a segment is selected, and follows the player's duration", () => {
    const onMark = vi.fn();
    render(<SegmentPlayer streamUrl="http://x/stream" durationMsFallback={200_000} segments={segments} selectedType="intro" onSelectType={vi.fn()} onMark={onMark} />);
    expect(document.querySelector("button[data-bg='#f00']")).toBeTruthy();
    video.player.duration = 400;
    video.player.currentTime = 42.4;
    act(() => video.tick(42.4));
    expect(screen.getByText("00:42")).toBeTruthy();
    expect(screen.getByText("06:40")).toBeTruthy();
    fireEvent.click(screen.getByText("Mark end").closest("button")!);
    expect(onMark).toHaveBeenCalledWith("end", 42_400);
    fireEvent.click(screen.getByText("Mark start").closest("button")!);
    expect(onMark).toHaveBeenCalledWith("start", 42_400);
  });
});
