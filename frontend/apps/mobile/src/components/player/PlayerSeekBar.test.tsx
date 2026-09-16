import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

// The View mock forwards the responder handlers as DOM-callable props so the
// test can drive a drag with synthetic native events.
vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel, accessibilityValue, onAccessibilityAction, onLayout, onResponderGrant, onResponderMove, onResponderRelease, onResponderTerminate, style }: any) =>
    React.createElement(
      "div",
      {
        "aria-label": accessibilityLabel,
        "data-now": accessibilityValue?.now,
        "data-text": accessibilityValue?.text,
        "data-left": Array.isArray(style) ? style.map((s: any) => s?.left).find((l: unknown) => l !== undefined) : style?.left,
        ref: (el: any) => {
          if (!el) return;
          el.__rn = { onAccessibilityAction, onLayout, onResponderGrant, onResponderMove, onResponderRelease, onResponderTerminate };
        },
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, string>) => (o ? `${k}(${o.current}/${o.total})` : k) }),
}));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00" } }) }));
vi.mock("../TrickplayPreview", () => ({ TrickplayPreview: () => React.createElement("div", { "data-testid": "trickplay" }) }));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${s}s` }));

import { PlayerSeekBar } from "./PlayerSeekBar";

function mount(over: Partial<React.ComponentProps<typeof PlayerSeekBar>> = {}) {
  const h = { onPreview: vi.fn(), onCommit: vi.fn(), onScrubbingChange: vi.fn() };
  render(<PlayerSeekBar mediaId="m1" progress={0.25} currentTime={30} effectiveDuration={120} chapters={[]} {...h} {...over} />);
  const bar = screen.getAllByLabelText("player.controls.seek").at(-1) as HTMLDivElement & { __rn: Record<string, (e: unknown) => void> };
  act(() => bar.__rn.onLayout({ nativeEvent: { layout: { width: 200 } } }));
  return { bar, ...h };
}

beforeEach(() => vi.clearAllMocks());

// A drag previews the fraction under the finger against the MEASURED bar
// width and commits the last moved fraction (never the release coordinate,
// unreliable on Android); a responder taken away ends the scrub without a
// commit; TalkBack steps 10 s per action and reads a position, not a number.
describe("PlayerSeekBar", () => {
  it("previews during the drag, commits the last moved fraction, and clears the scrub", () => {
    const { bar, onPreview, onCommit, onScrubbingChange } = mount();
    expect(onScrubbingChange).toHaveBeenLastCalledWith(false);
    act(() => bar.__rn.onResponderGrant({ nativeEvent: { locationX: 50 } }));
    expect(onPreview).toHaveBeenLastCalledWith(0.25);
    expect(onScrubbingChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByTestId("trickplay")).toBeTruthy();
    act(() => bar.__rn.onResponderMove({ nativeEvent: { locationX: 150 } }));
    expect(onPreview).toHaveBeenLastCalledWith(0.75);
    act(() => bar.__rn.onResponderMove({ nativeEvent: { locationX: 900 } }));
    expect(onPreview).toHaveBeenLastCalledWith(1);
    act(() => bar.__rn.onResponderRelease({ nativeEvent: { locationX: 2 } }));
    expect(onCommit).toHaveBeenCalledWith(1);
    expect(onScrubbingChange).toHaveBeenLastCalledWith(false);
    expect(screen.queryByTestId("trickplay")).toBeNull();
  });

  it("an interrupted drag ends the scrub without committing", () => {
    const { bar, onCommit, onScrubbingChange } = mount();
    act(() => bar.__rn.onResponderGrant({ nativeEvent: { locationX: 100 } }));
    act(() => bar.__rn.onResponderTerminate());
    expect(onCommit).not.toHaveBeenCalled();
    expect(onScrubbingChange).toHaveBeenLastCalledWith(false);
  });

  it("reads as a position for the screen reader and steps 10 s per action, clamped", () => {
    const { bar, onCommit } = mount();
    expect(bar.getAttribute("data-now")).toBe("25");
    expect(bar.getAttribute("data-text")).toBe("player.controls.seekPosition(30s/120s)");
    act(() => bar.__rn.onAccessibilityAction({ nativeEvent: { actionName: "increment" } }));
    expect(onCommit).toHaveBeenLastCalledWith(0.25 + 10 / 120);
    act(() => bar.__rn.onAccessibilityAction({ nativeEvent: { actionName: "decrement" } }));
    expect(onCommit).toHaveBeenLastCalledWith(0.25 - 10 / 120);

    const edge = mount({ progress: 0.98 });
    act(() => edge.bar.__rn.onAccessibilityAction({ nativeEvent: { actionName: "increment" } }));
    expect(edge.onCommit).toHaveBeenLastCalledWith(1);
    const none = mount({ effectiveDuration: 0 });
    act(() => none.bar.__rn.onAccessibilityAction({ nativeEvent: { actionName: "increment" } }));
    expect(none.onCommit).not.toHaveBeenCalled();
  });

  it("places a chapter marker per chapter inside the film", () => {
    mount({ chapters: [{ startTimeMs: 30_000 }, { startTimeMs: 0 }, { startTimeMs: 200_000 }] as any });
    const lefts = Array.from(document.querySelectorAll("[data-left]")).map((e) => e.getAttribute("data-left"));
    expect(lefts).toContain("25%");
    expect(lefts.filter((l) => l === "0%" || l === "166.66666666666669%")).toHaveLength(0);
  });
});
