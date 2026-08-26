import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  // Faithful to RN reality: a Modal keeps its children MOUNTED when
  // visible=false (it only hides the native presentation). A mock that
  // dropped them would make the hidden-case test below pass without the fix.
  Modal: ({ children, visible }: any) =>
    React.createElement("div", { "data-visible": String(visible) }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#aaa", accent: "#88f", border: "#333", onAccent: "#000" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 16, lg: 24, xl: 32 },
  }),
}));

vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, accessibilityState }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-selected": accessibilityState?.selected ? "true" : "false",
      },
      children,
    ),
}));

import { ChaptersPicker } from "./ChaptersPicker";

const CHAPTERS = [
  { title: "Some Chapter 1", startTimeMs: 0, endTimeMs: 60_000 },
  { title: "Some Chapter 2", startTimeMs: 60_000, endTimeMs: 120_000 },
];

// Ratchet for the whole TV picker family (Audio/Subtitle/Quality/Volume/
// SleepTimer/StreamInfo/TrackOptions share the pattern): a CLOSED picker must
// render NOTHING. RN's Modal keeps children mounted when visible=false, so
// every closed picker used to re-render on each of the player's ~2/s
// currentTime ticks for the whole playback.
describe("ChaptersPicker mount discipline", () => {
  it("renders nothing while hidden", () => {
    const { container } = render(
      <ChaptersPicker visible={false} onClose={() => {}} chapters={CHAPTERS} currentTimeMs={0} onSeek={() => {}} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders the chapter list when visible", () => {
    render(
      <ChaptersPicker visible={true} onClose={() => {}} chapters={CHAPTERS} currentTimeMs={0} onSeek={() => {}} />,
    );
    expect(screen.getByText("Some Chapter 1")).toBeTruthy();
    expect(screen.getByText("Some Chapter 2")).toBeTruthy();
  });
});

// Screen readers cannot infer the current chapter from the accent-coloured
// label alone - accessibilityState.selected must mirror the same
// findCurrentChapterIndex signal the visual checkmark uses.
describe("ChaptersPicker exposes the current chapter to assistive tech", () => {
  it("marks only the chapter containing currentTimeMs as selected", () => {
    render(
      <ChaptersPicker
        visible={true}
        onClose={() => {}}
        chapters={CHAPTERS}
        currentTimeMs={65_000}
        onSeek={() => {}}
      />,
    );
    expect(screen.getByLabelText("Some Chapter 1").getAttribute("data-selected")).toBe("false");
    expect(screen.getByLabelText("Some Chapter 2, selected").getAttribute("data-selected")).toBe("true");
  });
});
