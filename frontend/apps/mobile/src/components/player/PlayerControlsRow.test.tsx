import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityValue }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-value": accessibilityValue?.text ?? "",
      },
      children,
    ),
  ActivityIndicator: ({ accessibilityLabel, accessibilityLiveRegion, accessibilityRole }: any) =>
    React.createElement("div", {
      "aria-label": accessibilityLabel,
      "aria-live": accessibilityLiveRegion,
      role: accessibilityRole,
    }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("@expo/vector-icons/MaterialIcons", () => ({ default: () => null }));
vi.mock("react-native-svg", () => ({
  default: ({ children }: any) => React.createElement("svg", null, children),
  Path: () => null,
  Text: ({ children }: any) => React.createElement("span", null, children),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => String(s) }));
vi.mock("@evey/theme", () => ({
  FONT_SIZE: { xs: 13, sm: 15, base: 16 },
  ICON_SIZE: { sm: 14, md: 16, lg: 20, xl: 24 },
}));
vi.mock("./PlayerVolumeControl", () => ({ PlayerVolumeControl: () => null }));

import { PlayerControlsRow } from "./PlayerControlsRow";

const noop = () => {};
const base = {
  isPlaying: true,
  isBuffering: false,
  currentTime: 10,
  duration: 100,
  volume: 1,
  muted: false,
  prevEpisode: null,
  nextEpisode: null,
  hasPrevChapter: false,
  hasNextChapter: false,
  hasChapters: true,
  settingsOpen: false,
  sleepTimerActive: false,
  pipSupported: false,
  onSeekBy: noop,
  onTogglePlayPause: noop,
  onToggleMute: noop,
  onVolumeChange: noop,
  onPrevChapter: noop,
  onNextChapter: noop,
  onOpenChapters: noop,
} as any;

describe("PlayerControlsRow accessibility", () => {
  // A spinner says "wait" to anyone who can see it and nothing at all to
  // anyone who cannot: a stall was entirely silent, and the viewer was left
  // wondering whether their tap had even registered.
  it("names and announces the buffering spinner", () => {
    render(<PlayerControlsRow {...base} isBuffering />);
    const spinner = screen.getByLabelText("player.buffering");
    expect(spinner.getAttribute("aria-live")).toBe("polite");
    expect(spinner.getAttribute("role")).toBe("progressbar");
  });

  it("shows no spinner when playback is not stalled", () => {
    render(<PlayerControlsRow {...base} />);
    expect(screen.queryByLabelText("player.buffering")).toBeNull();
  });

  // Settings, chapters and the rest sit in the bar itself: a "..." button in
  // front of them cost a tap on every use (tablet, 2026-10-03).
  it("offers the settings without opening anything first", () => {
    const onToggleSettings = vi.fn();
    render(<PlayerControlsRow {...base} onToggleSettings={onToggleSettings} />);
    expect(screen.queryByLabelText("player.controls.more")).toBeNull();
    fireEvent.click(screen.getByLabelText("player.controls.settings"));
    expect(onToggleSettings).toHaveBeenCalledTimes(1);
  });

  // The current chapter was stated in exactly one place, inside the picker.
  it("reads the current chapter off the chapters button", () => {
    render(<PlayerControlsRow {...base} currentChapterTitle="The heist" />);
    expect(screen.getByLabelText("player.controls.chapters").getAttribute("data-value")).toBe("The heist");
  });

  it("leaves the chapters button bare when no chapter is current", () => {
    render(<PlayerControlsRow {...base} />);
    expect(screen.getByLabelText("player.controls.chapters").getAttribute("data-value")).toBe("");
  });
});
