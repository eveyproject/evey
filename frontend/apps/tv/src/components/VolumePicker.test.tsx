import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Modal: ({ children, visible }: any) => (visible ? children : null),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", accent: "#88f", border: "#333" } }),
}));
vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { base: 14, lg: 18, xl: 22 } }));

import { VolumePicker } from "./VolumePicker";

function mount(volumePct: number, muted: boolean) {
  const props = { onClose: vi.fn(), onSelect: vi.fn(), onToggleMute: vi.fn() };
  render(<VolumePicker visible volumePct={volumePct} muted={muted} {...props} />);
  return props;
}

// A remote has no slider: the sheet lists presets, marks the current level
// (unless muted - then nothing is "selected" and the row reads "Unmute"),
// keeps the focus on the mute row, and a pick closes the sheet while the
// mute toggle leaves it open.
describe("VolumePicker", () => {
  it("renders nothing while closed", () => {
    const { container } = render(<VolumePicker visible={false} volumePct={50} muted={false} onClose={vi.fn()} onSelect={vi.fn()} onToggleMute={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("marks the current level and closes on a pick", () => {
    const props = mount(75, false);
    expect(screen.getByLabelText("75%, selected")).toBeTruthy();
    expect(screen.getByLabelText("Mute").getAttribute("data-preferred")).toBe("true");
    fireEvent.click(screen.getByLabelText("25%"));
    expect(props.onSelect).toHaveBeenCalledWith(25);
    expect(props.onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByLabelText("Mute"));
    expect(props.onToggleMute).toHaveBeenCalled();
    expect(props.onClose).toHaveBeenCalledTimes(1);
  });

  it("muted: no level is selected and the row offers unmute", () => {
    mount(75, true);
    expect(screen.queryByLabelText("75%, selected")).toBeNull();
    expect(screen.getByLabelText("75%")).toBeTruthy();
    expect(screen.getByLabelText("Unmute")).toBeTruthy();
  });
});
