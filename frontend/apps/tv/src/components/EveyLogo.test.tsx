import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

// The wordmark keeps its aspect ratio from the height, colours the letters
// with the theme colour while the V stays brand red, and paints the
// signature band only when told what surface it sits on - a mask would
// render nothing at all on Android.
vi.mock("react-native-svg", () => ({
  default: ({ children, height, width, accessibilityLabel }: any) => React.createElement("svg", { height, width, "aria-label": accessibilityLabel }, children),
  G: ({ children }: any) => React.createElement("g", null, children),
  Path: ({ fill }: any) => React.createElement("path", { fill }),
  Rect: ({ fill }: any) => React.createElement("rect", { fill }),
}));

import { EveyWordmark, EveyMark } from "./EveyLogo";

describe("EveyLogo (tv)", () => {
  it("draws the wordmark at the asked height with letters in the theme colour and a red V", () => {
    const { container } = render(<EveyWordmark height={40} color="#abc" />);
    const svg = container.querySelector("svg")!;
    expect(svg.getAttribute("height")).toBe("40");
    expect(Number(svg.getAttribute("width"))).toBeCloseTo((40 * 3420) / 808, 3);
    expect(svg.getAttribute("aria-label")).toBe("Evey");
    const fills = Array.from(container.querySelectorAll("path")).map((p) => p.getAttribute("fill"));
    expect(fills).toEqual(["#abc", "#DC2626", "#abc", "#abc"]);
    expect(container.querySelector("rect")).toBeNull();
  });

  it("paints the band only with a surface colour, on both the wordmark and the mark", () => {
    const { container, rerender } = render(<EveyWordmark cutColor="#111" />);
    expect(container.querySelector("rect")?.getAttribute("fill")).toBe("#111");
    rerender(<EveyMark height={28} />);
    expect(container.querySelector("svg")?.getAttribute("width")).toBe("28");
    expect(container.querySelector("rect")).toBeNull();
    rerender(<EveyMark cutColor="#222" />);
    expect(container.querySelector("rect")?.getAttribute("fill")).toBe("#222");
  });
});
