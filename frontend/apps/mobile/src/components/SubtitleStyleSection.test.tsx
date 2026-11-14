import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, testID, ...p }: any) => React.createElement("span", { "data-testid": testID, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState, disabled }: any) =>
    React.createElement(
      "button",
      { onClick: disabled ? undefined : onPress, "aria-label": accessibilityLabel, "aria-pressed": !!accessibilityState?.selected, disabled },
      children,
    ),
  Switch: ({ value, onValueChange, disabled, accessibilityLabel }: any) =>
    React.createElement("button", {
      role: "switch", "aria-checked": !!value, "aria-label": accessibilityLabel, disabled,
      onClick: disabled ? undefined : () => onValueChange?.(!value),
    }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { accent: "#f00", border: "#333", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#999" },
    radii: { md: 8 },
    spacing: { md: 16 },
  }),
}));

import { SubtitleStyleSection } from "./SubtitleStyleSection";

const saved = (onSave: ReturnType<typeof vi.fn>) => JSON.parse(onSave.mock.calls.at(-1)![0].subtitle_style);

describe("SubtitleStyleSection (mobile)", () => {
  const onSave = vi.fn();
  beforeEach(() => onSave.mockClear());

  // The whole style travels in one preference: a change to one field must
  // carry the others as the profile holds them, not reset them.
  it("saves the profile's style with only the picked field changed", () => {
    const stored = { fontSize: 150, color: "#fde047", bgColor: "#1e3a5f", bgOpacity: 50, position: "top", shadow: true };
    render(<SubtitleStyleSection preferences={{ subtitle_style: JSON.stringify(stored), theme: "amoled" }} onSave={onSave} />);
    fireEvent.click(screen.getByLabelText("player.colors.cyan"));
    expect(saved(onSave)).toEqual({ ...stored, color: "#22d3ee" });
    expect(Object.keys(onSave.mock.calls[0][0])).toEqual(["subtitle_style"]);
  });

  it("starts from the defaults for a profile that never chose", () => {
    render(<SubtitleStyleSection preferences={undefined} onSave={onSave} />);
    expect(screen.getByLabelText("100 %").getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("switch"));
    expect(saved(onSave)).toEqual({ fontSize: 100, color: "#ffffff", bgColor: "#000000", bgOpacity: 75, position: "bottom", shadow: true });
  });

  it("does nothing while disabled", () => {
    render(<SubtitleStyleSection preferences={undefined} onSave={onSave} disabled />);
    fireEvent.click(screen.getByLabelText("200 %"));
    fireEvent.click(screen.getByRole("switch"));
    expect(onSave).not.toHaveBeenCalled();
  });
});
