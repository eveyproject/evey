import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// Style arrays are flattened the way RN does, then the three values the
// tests read (fill width/height, fill colour) are exposed as attributes.
function flat(style: unknown): Record<string, unknown> {
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flat));
  return style && typeof style === "object" ? (style as Record<string, unknown>) : {};
}
vi.mock("react-native", () => ({
  View: ({ children, style }: any) => {
    const st = flat(style);
    return React.createElement("div", { "data-w": st.width, "data-h": st.height, "data-bg": st.backgroundColor }, children);
  },
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));

function fills(container: HTMLElement) {
  return Array.from(container.querySelectorAll("div")).map((d) => ({ w: d.getAttribute("data-w"), h: d.getAttribute("data-h"), bg: d.getAttribute("data-bg") }));
}
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#88f", warning: "#fa0" } }) }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, base: 14, lg: 18, xl: 22 } }));

import { GestureIndicators } from "./GestureIndicators";

const base = { seekFlash: null, scrubPreview: null, volumeIndicator: null, brightnessIndicator: null, skipFwd: 30, skipBwd: 10 };

// The overlays read what the gesture will do: a double tap flashes the skip
// amount on its side, a scrub shows the signed delta and the landing clock
// (hours dropped under an hour, the fill clamped to the track), the volume
// bar caps at 100% and turns amber past it, and nothing renders at rest.
describe("GestureIndicators", () => {
  it("renders nothing at rest", () => {
    const { container } = render(<GestureIndicators {...base} />);
    expect(container.innerHTML).toBe("");
  });

  it("flashes the configured skip on the side that was tapped", () => {
    const { rerender } = render(<GestureIndicators {...base} seekFlash="left" />);
    expect(screen.getByText("« 10")).toBeTruthy();
    rerender(<GestureIndicators {...base} seekFlash="right" />);
    expect(screen.getByText("30 »")).toBeTruthy();
  });

  it("shows the scrub delta and landing time, clamped", () => {
    const { rerender, container } = render(<GestureIndicators {...base} scrubPreview={{ targetSec: 724, deltaSec: -95, durationSec: 2700 }} />);
    expect(screen.getByText("-1:35")).toBeTruthy();
    expect(container.textContent).toContain("12:04 / 45:00");
    expect(fills(container).some((f) => f.w === "26.814814814814813%")).toBe(true);

    rerender(<GestureIndicators {...base} scrubPreview={{ targetSec: 5400, deltaSec: 3661, durationSec: 5000 }} />);
    expect(screen.getByText("+1:01:01")).toBeTruthy();
    expect(container.textContent).toContain("1:30:00 / 1:23:20");
    expect(fills(container).some((f) => f.w === "100%")).toBe(true);

    rerender(<GestureIndicators {...base} scrubPreview={{ targetSec: 10, deltaSec: 0, durationSec: 0 }} />);
    expect(fills(container).some((f) => f.w === "0%")).toBe(true);
    expect(screen.getByText("+0:00")).toBeTruthy();
  });

  it("volume caps the bar at 100 and marks a boost, brightness reads as a percentage", () => {
    const { rerender, container } = render(<GestureIndicators {...base} volumeIndicator={1.5} />);
    expect(screen.getByText("150")).toBeTruthy();
    expect(fills(container).some((f) => f.h === "100%" && f.bg === "#fa0")).toBe(true);

    rerender(<GestureIndicators {...base} volumeIndicator={0.4} />);
    expect(screen.getByText("40")).toBeTruthy();
    expect(fills(container).some((f) => f.h === "40%" && f.bg !== "#fa0")).toBe(true);
    expect(fills(container).some((f) => f.bg === "#fa0")).toBe(false);

    rerender(<GestureIndicators {...base} brightnessIndicator={0.755} />);
    expect(screen.getByText("76")).toBeTruthy();
    expect(fills(container).some((f) => f.h === "76%")).toBe(true);
  });
});
