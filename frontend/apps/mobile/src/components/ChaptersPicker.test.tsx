import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("./BottomSheet", () => ({ BottomSheet: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { border: "#333", text: "#fff", textMuted: "#888", accent: "#f00" } }) }));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${s}s` }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10, sm: 12, lg: 18 } }));

import { ChaptersPicker } from "./ChaptersPicker";

const chapters = [
  { startTimeMs: 0, title: "Opening" },
  { startTimeMs: 60_000, title: "" },
  { startTimeMs: 300_000, title: "Finale" },
] as any[];

// The current chapter is the last one whose start is behind the playhead
// (the final chapter runs to the end); an untitled chapter is numbered;
// a tap seeks to the chapter start in seconds and closes the sheet.
describe("ChaptersPicker", () => {
  it("ticks the chapter under the playhead, numbers untitled ones, seeks and closes on a tap", () => {
    const onSeek = vi.fn();
    const onClose = vi.fn();
    const { rerender } = render(<ChaptersPicker visible chapters={chapters} currentTimeMs={90_000} onSeek={onSeek} onClose={onClose} />);
    expect(screen.getByLabelText("media.chapter 2").textContent).toContain("✓");
    expect(screen.getByLabelText("Opening").textContent).not.toContain("✓");
    expect(screen.getByText("60s")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Finale"));
    expect(onSeek).toHaveBeenCalledWith(300);
    expect(onClose).toHaveBeenCalled();

    rerender(<ChaptersPicker visible chapters={chapters} currentTimeMs={9_000_000} onSeek={onSeek} onClose={onClose} />);
    expect(screen.getByLabelText("Finale").textContent).toContain("✓");
  });

  it("renders nothing while hidden", () => {
    const { container } = render(<ChaptersPicker visible={false} chapters={chapters} currentTimeMs={0} onSeek={vi.fn()} onClose={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });
});
