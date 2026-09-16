import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => {
      if (typeof fallback === "object" && fallback !== null && "count" in (fallback as object)) return `${(fallback as { count: number }).count} min`;
      return typeof fallback === "string" ? fallback : key;
    },
  }),
}));
vi.mock("./BottomSheet", () => ({ BottomSheet: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { border: "#333", text: "#fff", accent: "#f00" } }) }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, lg: 18 } }));

import { SleepTimerPicker } from "./SleepTimerPicker";

function mount(state: { kind: "minutes" | "endOfEpisode"; minutesLeft: number | null } | null) {
  const h = { onClose: vi.fn(), onPickMinutes: vi.fn(), onPickEndOfEpisode: vi.fn(), onCancel: vi.fn() };
  render(<SleepTimerPicker visible state={state} {...h} />);
  return h;
}

// Mobile twin of the TV sheet: presets and end of episode, the running timer
// ticked on the preset it sits under, cancel only while a timer runs, every
// pick closes the sheet.
describe("SleepTimerPicker (mobile)", () => {
  it("picks a preset or the end of the episode and closes; no cancel without a timer", () => {
    const h = mount(null);
    expect(screen.queryByLabelText("Cancel timer")).toBeNull();
    expect(screen.queryByText("✓")).toBeNull();
    fireEvent.click(screen.getByLabelText("45 min"));
    expect(h.onPickMinutes).toHaveBeenCalledWith(45);
    expect(h.onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByLabelText("End of episode"));
    expect(h.onPickEndOfEpisode).toHaveBeenCalled();
    expect(h.onClose).toHaveBeenCalledTimes(2);
  });

  it("ticks the running timer on its preset and offers cancel", () => {
    const h = mount({ kind: "minutes", minutesLeft: 29.2 });
    const row = screen.getByLabelText("30 min");
    expect(row.textContent).toContain("✓");
    expect(screen.getByLabelText("15 min").textContent).not.toContain("✓");
    fireEvent.click(screen.getByLabelText("Cancel timer"));
    expect(h.onCancel).toHaveBeenCalled();
    expect(h.onClose).toHaveBeenCalled();
    mount({ kind: "endOfEpisode", minutesLeft: null });
    expect(screen.getAllByLabelText("End of episode").at(-1)!.textContent).toContain("✓");
  });

  it("renders nothing while hidden", () => {
    const { container } = render(<SleepTimerPicker visible={false} state={null} onClose={vi.fn()} onPickMinutes={vi.fn()} onPickEndOfEpisode={vi.fn()} onCancel={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });
});
