import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";
import { DEFAULT_SUBTITLE_STYLE } from "../../lib/subtitleStyle";

// subtitleStyle.ts imports expo-secure-store at module scope for its
// persistence helpers - unused by this panel (persistence lives in the player
// screen), but the real module requires the native runtime and crashes
// outside it if not stubbed here.
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Text: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("span", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Pressable: ({ children, onPress, accessibilityLabel, style, disabled, ...rest }: Record<string, unknown>) =>
    React.createElement(
      "button",
      {
        ...rest,
        onClick: disabled ? undefined : (onPress as () => void),
        disabled: disabled as boolean,
        "aria-label": accessibilityLabel as string,
        "data-style": JSON.stringify(style),
      },
      children as React.ReactNode,
    ),
  ScrollView: ({ children, style }: Record<string, unknown>) =>
    React.createElement("div", { "data-style": JSON.stringify(style) }, children as React.ReactNode),
  StyleSheet: { create: (s: Record<string, unknown>) => s, hairlineWidth: 1 },
  useWindowDimensions: () => ({ width: 400, height: 800 }),
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: { name: string }) => React.createElement("span", { "data-icon": name }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      surface: "#111",
      border: "#222",
      surfaceRaised: "#333",
      accent: "#818cf8",
      onAccent: "#000",
      text: "#fff",
      textMuted: "#888",
    },
  }),
}));

// Real key -> English string lookup (from i18n/en/player.yaml): most calls in
// PlayerSettingsPanel don't pass a fallback, so a fallback-only mock would
// just echo the raw key back and every accessibilityLabel assertion below
// would be testing nothing.
const TRANSLATIONS: Record<string, string> = {
  "player.settings.subtitleStyle": "Subtitle Style",
  "player.settings.subtitleSync": "Subtitle sync",
  "player.settings.subtitleSyncHint": "Positive = subtitles later. Applied instantly.",
  "player.subtitleStyle.size": "Size",
  "player.subtitleStyle.textColor": "Text Color",
  "player.subtitleStyle.background": "Background",
  "player.subtitleStyle.position": "Position",
  "player.subtitleStyle.top": "Top",
  "player.subtitleStyle.bottom": "Bottom",
  "player.subtitleStyle.textShadow": "Text shadow",
  "player.colors.white": "White",
  "player.colors.yellow": "Yellow",
  "player.colors.green": "Green",
  "player.colors.cyan": "Cyan",
  "player.colors.orange": "Orange",
  "player.bgColors.black": "Black",
  "player.bgColors.darkGrey": "Dark grey",
  "player.bgColors.navy": "Navy",
  "player.bgColors.none": "None",
  "common.back": "Back",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => TRANSLATIONS[key] ?? fallback ?? key }),
}));

import { PlayerSettingsPanel } from "./PlayerSettingsPanel";

const baseProps = {
  audioStreams: [],
  currentAudioIndex: 0,
  onAudioChange: vi.fn(),
  subtitleStreams: [],
  currentSubtitleIndex: null,
  onSubtitleChange: vi.fn(),
  currentQuality: 0,
  onQualityChange: vi.fn(),
  playbackSpeed: 1,
  onSpeedChange: vi.fn(),
  brightness: 1,
  onBrightnessChange: vi.fn(),
  onClose: vi.fn(),
};

describe("PlayerSettingsPanel - subtitle style", () => {
  it("hides the Subtitle Style entry when onSubtitleStyleChange is not provided", () => {
    const { queryByLabelText } = render(<PlayerSettingsPanel {...baseProps} />);
    expect(queryByLabelText("Subtitle Style")).toBeNull();
  });

  it("navigates into the subtitle-style submenu", () => {
    const { getByLabelText, getByText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={vi.fn()} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    expect(getByText("Size")).toBeTruthy();
    expect(getByText("Text Color")).toBeTruthy();
    expect(getByText("Background")).toBeTruthy();
    expect(getByText("Position")).toBeTruthy();
  });

  it("picking a text color swatch reports the patch", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Yellow"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ color: "#fde047" });
  });

  it("picking the 'none' background swatch also zeroes the opacity", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("None"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ bgColor: "#000000", bgOpacity: 0 });
  });

  it("toggling position reports the patch", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Top"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ position: "top" });
  });

  it("toggling the shadow checkbox flips the current value", () => {
    const onSubtitleStyleChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel {...baseProps} subtitleStyle={DEFAULT_SUBTITLE_STYLE} onSubtitleStyleChange={onSubtitleStyleChange} />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Text shadow"));
    expect(onSubtitleStyleChange).toHaveBeenCalledWith({ shadow: true });
  });

  it("does not close the panel when a style option is picked (live preview)", () => {
    const onClose = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        onClose={onClose}
        subtitleStyle={DEFAULT_SUBTITLE_STYLE}
        onSubtitleStyleChange={vi.fn()}
      />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Yellow"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("subtitle offset stepper steps by 100ms and resets to zero", () => {
    const onSubtitleOffsetChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerSettingsPanel
        {...baseProps}
        subtitleStyle={DEFAULT_SUBTITLE_STYLE}
        onSubtitleStyleChange={vi.fn()}
        subtitleOffsetMs={200}
        onSubtitleOffsetChange={onSubtitleOffsetChange}
      />,
    );
    fireEvent.click(getByLabelText("Subtitle Style"));
    fireEvent.click(getByLabelText("Subtitle sync +"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(300);
    fireEvent.click(getByLabelText("Subtitle sync -"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(100);
    fireEvent.click(getByLabelText("Subtitle sync 0"));
    expect(onSubtitleOffsetChange).toHaveBeenCalledWith(0);
  });
});
