import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...rest }: Record<string, unknown>) => React.createElement("div", { "aria-label": rest.accessibilityLabel as string }, children as React.ReactNode),
  Text: ({ children }: Record<string, unknown>) => React.createElement("span", null, children as React.ReactNode),
  Pressable: ({ children, onPress, accessibilityLabel }: Record<string, unknown>) =>
    React.createElement("button", { onClick: onPress as () => void, "aria-label": accessibilityLabel as string }, children as React.ReactNode),
  StatusBar: () => null,
  StyleSheet: { create: (s: Record<string, unknown>) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: { name: string }) => React.createElement("span", { "data-icon": name }) }));
vi.mock("../../hooks/useChromecast", () => ({ CastButtonNative: null }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { accent: "#818cf8", text: "#fff", textMuted: "#888", surface: "#111" } }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, arg?: unknown) => {
      if (typeof arg === "string") return arg;
      const opts = (arg ?? {}) as { seconds?: number };
      return opts.seconds !== undefined ? `${key}:${opts.seconds}` : key;
    },
  }),
}));

import { CastControllerScreen } from "./CastControllerScreen";

// The cast controller is the player's fallback UI while a receiver plays: the
// transport buttons must drive the receiver through the callbacks, clamped at
// the start, and the volume row only exists when the SDK can set a volume.
const baseProps = {
  title: "Film",
  deviceName: "Living room",
  currentTime: 100,
  duration: 600,
  isPlaying: true,
  skipFwd: 30,
  skipBwd: 10,
  onSeekToPosition: vi.fn(),
  onPlayPause: vi.fn(),
  onBack: vi.fn(),
};

describe("CastControllerScreen", () => {
  it("skips relative to the current position and never before the start", () => {
    const onSeekToPosition = vi.fn();
    const { getByLabelText, rerender } = render(<CastControllerScreen {...baseProps} onSeekToPosition={onSeekToPosition} />);
    fireEvent.click(getByLabelText("player.controls.forward:30"));
    expect(onSeekToPosition).toHaveBeenLastCalledWith(130);
    fireEvent.click(getByLabelText("player.controls.rewind:10"));
    expect(onSeekToPosition).toHaveBeenLastCalledWith(90);

    rerender(<CastControllerScreen {...baseProps} currentTime={4} onSeekToPosition={onSeekToPosition} />);
    fireEvent.click(getByLabelText("player.controls.rewind:10"));
    expect(onSeekToPosition).toHaveBeenLastCalledWith(0);
  });

  it("labels the play/pause button by the receiver state and reports the tap", () => {
    const onPlayPause = vi.fn();
    const { getByLabelText, queryByLabelText, rerender } = render(<CastControllerScreen {...baseProps} onPlayPause={onPlayPause} />);
    fireEvent.click(getByLabelText("player.controls.pause"));
    expect(onPlayPause).toHaveBeenCalledTimes(1);
    rerender(<CastControllerScreen {...baseProps} isPlaying={false} onPlayPause={onPlayPause} />);
    expect(queryByLabelText("player.controls.pause")).toBeNull();
    expect(getByLabelText("player.controls.play")).toBeTruthy();
  });

  it("shows the volume row only when the SDK can set a volume, and mute toggles", () => {
    const { queryByLabelText } = render(<CastControllerScreen {...baseProps} />);
    expect(queryByLabelText("Mute")).toBeNull();
    expect(queryByLabelText("Volume")).toBeNull();

    const onSetMuted = vi.fn();
    const { getByLabelText, rerender } = render(
      <CastControllerScreen {...baseProps} volume={0.5} muted={false} onSetVolume={vi.fn()} onSetMuted={onSetMuted} />,
    );
    expect(getByLabelText("Volume")).toBeTruthy();
    fireEvent.click(getByLabelText("Mute"));
    expect(onSetMuted).toHaveBeenCalledWith(true);
    rerender(<CastControllerScreen {...baseProps} volume={0.5} muted={true} onSetVolume={vi.fn()} onSetMuted={onSetMuted} />);
    fireEvent.click(getByLabelText("Unmute"));
    expect(onSetMuted).toHaveBeenLastCalledWith(false);
  });

  it("names the receiver and lets the viewer leave", () => {
    const onBack = vi.fn();
    const { getByText } = render(<CastControllerScreen {...baseProps} onBack={onBack} />);
    expect(getByText("Living room")).toBeTruthy();
    fireEvent.click(getByText("player.goBack"));
    expect(onBack).toHaveBeenCalledTimes(1);
  });
});
