import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, focusable, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-focusable": String(focusable), ...p },
      children,
    ),
  Modal: ({ children, visible }: any) => (visible ? children : null),
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
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        // Booleans vanish through unknown DOM props: surface it as data-* so
        // the focus claim is observable.
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : "false",
      },
      children,
    ),
}));

vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, xs: 10, base: 14, lg: 18, xl: 22 } }));

import { SubtitlePicker, type SubtitleStream } from "./SubtitlePicker";

const STREAMS: SubtitleStream[] = [
  { index: 0, trackIndex: 0, language: "English" },
  { index: 1, trackIndex: 1, language: "French" },
];

// #H7: the player pre-selects a subtitle track from the profile's language on
// the nominal path, so "no track selected" is the exception, not the rule.
// The picker must still hand the D-pad a focused row on open, on whichever
// row is actually active - not always the first one.
describe("SubtitlePicker hands the D-pad the active row", () => {
  it("focuses the active track's row when one is pre-selected", () => {
    render(
      <SubtitlePicker visible streams={STREAMS} currentIndex={1} onSelect={() => {}} onClose={() => {}} />,
    );
    expect(screen.getByLabelText("English").getAttribute("data-tv-preferred-focus")).toBe("false");
    expect(screen.getByLabelText("French, selected").getAttribute("data-tv-preferred-focus")).toBe("true");
    expect(screen.getByLabelText("player.settings.off").getAttribute("data-tv-preferred-focus")).toBe("false");
  });

  it("focuses the Off row when no track is selected", () => {
    render(
      <SubtitlePicker visible streams={STREAMS} currentIndex={null} onSelect={() => {}} onClose={() => {}} />,
    );
    expect(screen.getByLabelText("player.settings.off, selected").getAttribute("data-tv-preferred-focus")).toBe("true");
    expect(screen.getByLabelText("English").getAttribute("data-tv-preferred-focus")).toBe("false");
    expect(screen.getByLabelText("French").getAttribute("data-tv-preferred-focus")).toBe("false");
  });

  it("keeps the backdrop and sheet out of the D-pad focus chain", () => {
    render(
      <SubtitlePicker visible streams={STREAMS} currentIndex={null} onSelect={() => {}} onClose={() => {}} />,
    );
    const nonFocusable = screen.getAllByRole("button").filter((el) => el.getAttribute("data-focusable") === "false");
    expect(nonFocusable.length).toBe(2);
  });
});
