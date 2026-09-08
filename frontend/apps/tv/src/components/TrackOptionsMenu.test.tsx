import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  Modal: ({ children, visible }: any) =>
    React.createElement("div", { "data-visible": String(visible) }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#111", text: "#fff", textMuted: "#aaa", border: "#333" },
  }),
}));

vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
}));

import { TrackOptionsMenu } from "./TrackOptionsMenu";

const CHAPTERS = { visible: true, onClose: () => {}, hasChapters: true, onOpenChapters: () => {} };

describe("TrackOptionsMenu accessibility", () => {
  // Every row painted its value on screen and named only the label to the
  // screen reader: "Quality", never "Quality, 1080p". The answer was the half
  // that was dropped.
  it("speaks the row's value, not just its label", () => {
    render(
      <TrackOptionsMenu
        visible
        onClose={() => {}}
        onOpenQuality={() => {}}
        qualityLabel="1080p"
      />,
    );
    expect(screen.getByLabelText("player.settings.quality, 1080p")).toBeTruthy();
  });

  it("leaves a valueless row's label alone", () => {
    render(<TrackOptionsMenu visible onClose={() => {}} hasAudioOptions onOpenAudio={() => {}} />);
    expect(screen.getByLabelText("player.settings.audio")).toBeTruthy();
  });

  // The current chapter was stated in exactly one place, inside the picker.
  // Anyone who had not opened it - which is everyone, until they do - had no
  // way to learn where they were.
  it("names the current chapter on the chapters row", () => {
    render(<TrackOptionsMenu {...CHAPTERS} currentChapterTitle="The heist" />);
    expect(screen.getByLabelText("Chapters, The heist")).toBeTruthy();
    expect(screen.getByText("The heist")).toBeTruthy();
  });

  it("falls back to the bare label when the chapter has no title", () => {
    render(<TrackOptionsMenu {...CHAPTERS} />);
    expect(screen.getByLabelText("Chapters")).toBeTruthy();
  });
});

describe("TrackOptionsMenu autoplay next row", () => {
  it("does not appear without onToggleAutoplayNext", () => {
    render(<TrackOptionsMenu visible onClose={() => {}} />);
    expect(screen.queryByText("player.settings.autoplayNext")).toBeNull();
  });

  it("shows Enabled/Disabled and toggles on press", () => {
    const onToggle = vi.fn();
    render(
      <TrackOptionsMenu
        visible
        onClose={() => {}}
        autoplayNext
        onToggleAutoplayNext={onToggle}
      />,
    );
    expect(screen.getByLabelText("player.settings.autoplayNext, common.enabled")).toBeTruthy();

    screen.getByLabelText("player.settings.autoplayNext, common.enabled").click();
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  it("shows Disabled when autoplayNext is false", () => {
    render(
      <TrackOptionsMenu
        visible
        onClose={() => {}}
        autoplayNext={false}
        onToggleAutoplayNext={() => {}}
      />,
    );
    expect(screen.getByLabelText("player.settings.autoplayNext, common.disabled")).toBeTruthy();
  });
});
