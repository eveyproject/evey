import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => {
      if (key === "player.nextEpisodeOverlay.episode") return `Episode ${(opts as { number: number }).number}`;
      if (key === "player.nextEpisodeOverlay.nextIn") return `Next in ${(opts as { seconds: number }).seconds}s`;
      return typeof opts === "string" ? opts : key;
    },
  }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", border: "#333", text: "#fff", textMuted: "#aaa", accent: "#88f", onAccent: "#000", bg: "#000" } }),
}));
vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-preferred": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, base: 14, xl: 22 } }));

import { NextEpisodeOverlay } from "./NextEpisodeOverlay";

const next = { id: "e2", title: "The Second One", indexNumber: 2 } as any;

// Before the credits end the card says "up next"; once the episode has ended
// it counts down (never below zero), the focus lands on "play now", and a
// next episode without a number still gets a label.
describe("NextEpisodeOverlay", () => {
  it("renders nothing when hidden or without a next episode", () => {
    const a = render(<NextEpisodeOverlay visible={false} ended={false} countdown={5} nextEpisode={next} onPlayNow={vi.fn()} onCancel={vi.fn()} />);
    expect(a.container.innerHTML).toBe("");
    const b = render(<NextEpisodeOverlay visible ended={false} countdown={5} nextEpisode={null} onPlayNow={vi.fn()} onCancel={vi.fn()} />);
    expect(b.container.innerHTML).toBe("");
  });

  it("up next while the episode plays, a countdown once it ended", () => {
    const onPlayNow = vi.fn();
    const onCancel = vi.fn();
    const { rerender } = render(<NextEpisodeOverlay visible ended={false} countdown={5} nextEpisode={next} onPlayNow={onPlayNow} onCancel={onCancel} />);
    expect(screen.getByText("Up next")).toBeTruthy();
    expect(screen.getByText("Episode 2 - The Second One")).toBeTruthy();
    expect(screen.getByLabelText("Play now").getAttribute("data-preferred")).toBe("true");
    fireEvent.click(screen.getByLabelText("Play now"));
    expect(onPlayNow).toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText("Cancel"));
    expect(onCancel).toHaveBeenCalled();

    rerender(<NextEpisodeOverlay visible ended countdown={3} nextEpisode={next} onPlayNow={onPlayNow} onCancel={onCancel} />);
    expect(screen.getByText("Next in 3s")).toBeTruthy();
    rerender(<NextEpisodeOverlay visible ended countdown={-2} nextEpisode={next} onPlayNow={onPlayNow} onCancel={onCancel} />);
    expect(screen.getByText("Next in 0s")).toBeTruthy();
  });

  it("labels an unnumbered next episode by its title alone", () => {
    render(<NextEpisodeOverlay visible ended={false} countdown={5} nextEpisode={{ id: "e9", title: "Special" } as any} onPlayNow={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByText("Next episode - Special")).toBeTruthy();
  });
});
