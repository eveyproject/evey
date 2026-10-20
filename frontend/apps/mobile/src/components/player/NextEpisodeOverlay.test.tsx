import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

function flat(style: unknown): Record<string, unknown> {
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flat));
  return style && typeof style === "object" ? (style as Record<string, unknown>) : {};
}
vi.mock("react-native", () => ({
  View: ({ children, style }: any) => React.createElement("div", { "data-w": flat(style).width, "data-bottom": flat(style).bottom }, children),
  Text: ({ children, accessibilityLiveRegion }: any) => React.createElement("span", { "data-live": accessibilityLiveRegion }, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, number>) => {
      if (k === "player.nextEpisodeOverlay.nextIn") return `Next in ${o!.seconds}s`;
      if (k === "player.nextEpisodeOverlay.episode") return `Episode ${o!.number}`;
      return k;
    },
  }),
}));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00" } }) }));
vi.mock("../../hooks/useNextEpisode", () => ({ COUNTDOWN_SEC: 5 }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10, base: 14 } }));

import { NextEpisodeOverlay } from "./NextEpisodeOverlay";

function state(over: Record<string, unknown> = {}) {
  return { nextEpisode: { id: "e2", title: "Second", indexNumber: 2, seasonIndexNumber: 1 }, ended: false, countdown: 5, playNow: vi.fn(), cancel: vi.fn(), ...over } as any;
}

// Before the credits end the card only says "up next" (polite); once ended
// it counts down assertively - a screen reader must interrupt because the
// action fires without the user - and fills a bar as the seconds run out.
describe("NextEpisodeOverlay (mobile)", () => {
  it("falls back to the localized episode number when the label would be empty", () => {
    render(<NextEpisodeOverlay nextEp={state({ nextEpisode: { id: "e0", indexNumber: 0 } })} />);
    expect(screen.getByText("Episode 0")).toBeTruthy();
  });

  it("hides without a next episode and sits at the caller's offset", () => {
    const { container } = render(<NextEpisodeOverlay nextEp={state({ nextEpisode: null })} />);
    expect(container.innerHTML).toBe("");
    render(<NextEpisodeOverlay nextEp={state()} bottomOffset={120} />);
    expect(document.querySelector("[data-bottom='120']")).toBeTruthy();
  });

  it("up next politely, then an assertive countdown with a filling bar", () => {
    const s = state();
    const { rerender } = render(<NextEpisodeOverlay nextEp={s} />);
    expect(screen.getByText("player.nextEpisodeOverlay.upNext").getAttribute("data-live")).toBe("polite");
    expect(screen.getByText("S01E02 · Second")).toBeTruthy();
    expect(document.querySelector("[data-w]")).toBeNull();
    fireEvent.click(screen.getByLabelText("player.nextEpisodeOverlay.playNow"));
    expect(s.playNow).toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText("common.cancel"));
    expect(s.cancel).toHaveBeenCalled();

    rerender(<NextEpisodeOverlay nextEp={state({ ended: true, countdown: 2 })} />);
    expect(screen.getByText("Next in 2s").getAttribute("data-live")).toBe("assertive");
    expect(document.querySelector("[data-w='60%']")).toBeTruthy();
    rerender(<NextEpisodeOverlay nextEp={state({ ended: true, countdown: -1, nextEpisode: { id: "e9", title: "" } })} />);
    expect(screen.getByText("Next in 0s")).toBeTruthy();
    expect(screen.getByText("player.nextEpisodeOverlay.nextEpisode")).toBeTruthy();
    expect(document.querySelector("[data-w='100%']")).toBeTruthy();
  });
});
