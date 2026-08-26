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

vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, xs: 10, base: 14, lg: 18, xl: 22 } }));

import { QualityPicker } from "./QualityPicker";

// Screen readers cannot infer the active rendition from the accent colour
// alone - accessibilityState.selected must mirror the same `active` signal
// (q.value === currentQuality) the visual checkmark uses.
describe("QualityPicker exposes the selected quality to assistive tech", () => {
  it("marks only the row matching currentQuality as selected", () => {
    render(<QualityPicker visible currentQuality={720} onSelect={() => {}} onClose={() => {}} />);
    expect(screen.getByLabelText("720p, selected").getAttribute("data-selected")).toBe("true");
    expect(screen.getByLabelText("Auto").getAttribute("data-selected")).toBe("false");
    expect(screen.getByLabelText("1080p").getAttribute("data-selected")).toBe("false");
    expect(screen.getByLabelText("480p").getAttribute("data-selected")).toBe("false");
    expect(screen.getByLabelText("360p").getAttribute("data-selected")).toBe("false");
  });

  it("marks Auto as selected when currentQuality is 0", () => {
    render(<QualityPicker visible currentQuality={0} onSelect={() => {}} onClose={() => {}} />);
    expect(screen.getByLabelText("Auto, selected").getAttribute("data-selected")).toBe("true");
    expect(screen.getByLabelText("1080p").getAttribute("data-selected")).toBe("false");
  });
});
