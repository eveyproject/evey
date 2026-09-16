import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Modal: ({ children, visible }: any) => (visible ? children : null),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => {
      if (typeof fallback === "object" && fallback !== null && "count" in (fallback as object)) {
        return `${(fallback as { count: number }).count} min`;
      }
      return typeof fallback === "string" ? fallback : key;
    },
  }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", accent: "#88f", border: "#333" } }),
}));
vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { base: 14, lg: 18, xl: 22 } }));

import { SleepTimerPicker } from "./SleepTimerPicker";

function mount(state: { kind: "minutes" | "endOfEpisode"; minutesLeft: number | null } | null) {
  const props = { onClose: vi.fn(), onPickMinutes: vi.fn(), onPickEndOfEpisode: vi.fn(), onCancel: vi.fn() };
  render(<SleepTimerPicker visible state={state} {...props} />);
  return props;
}

// The sheet lists the presets and "end of episode", marks the running timer
// as selected (a 28-minute remainder sits on the 30 preset) and puts the
// D-pad focus on it, only offers "cancel" while a timer runs, and every
// pick closes the sheet after firing.
describe("SleepTimerPicker", () => {
  it("renders nothing while closed", () => {
    const { container } = render(
      <SleepTimerPicker visible={false} state={null} onClose={vi.fn()} onPickMinutes={vi.fn()} onPickEndOfEpisode={vi.fn()} onCancel={vi.fn()} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("without a timer: presets, end of episode, no cancel, focus on the first row", () => {
    const props = mount(null);
    expect(screen.getByLabelText("15 min").getAttribute("data-preferred")).toBe("true");
    expect(screen.queryByLabelText("Cancel timer")).toBeNull();
    fireEvent.click(screen.getByLabelText("45 min"));
    expect(props.onPickMinutes).toHaveBeenCalledWith(45);
    expect(props.onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByLabelText("End of episode"));
    expect(props.onPickEndOfEpisode).toHaveBeenCalled();
    expect(props.onClose).toHaveBeenCalledTimes(2);
  });

  it("a running minutes timer is selected on the preset it sits under, and can be cancelled", () => {
    const props = mount({ kind: "minutes", minutesLeft: 29.5 });
    const active = screen.getByLabelText("30 min, selected");
    expect(active.getAttribute("data-preferred")).toBe("true");
    expect(screen.getByLabelText("15 min").getAttribute("data-preferred")).toBe("false");
    fireEvent.click(screen.getByLabelText("Cancel timer"));
    expect(props.onCancel).toHaveBeenCalled();
    expect(props.onClose).toHaveBeenCalled();
  });

  it("an end-of-episode timer is selected on its own row", () => {
    mount({ kind: "endOfEpisode", minutesLeft: null });
    expect(screen.getByLabelText("End of episode, selected").getAttribute("data-preferred")).toBe("true");
    expect(screen.queryByLabelText("30 min, selected")).toBeNull();
  });
});
