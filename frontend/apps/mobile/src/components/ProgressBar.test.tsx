import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, style }: any) => React.createElement("div", { "data-width": style?.width, "data-bg": style?.backgroundColor }, children),
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00", divider: "#333" } }) }));

import { ProgressBar } from "./ProgressBar";

// The fill is the percentage clamped to 0..100 (a resume past the end or a
// negative position must not draw outside the track), in the accent unless
// a colour is given.
describe("ProgressBar (mobile)", () => {
  it("clamps the fill and takes the accent by default", () => {
    const { container, rerender } = render(<ProgressBar percent={42} />);
    const fill = () => container.querySelector("[data-width]") as HTMLElement;
    expect(fill().getAttribute("data-width")).toBe("42%");
    expect(fill().getAttribute("data-bg")).toBe("#f00");
    rerender(<ProgressBar percent={140} color="#0f0" />);
    expect(fill().getAttribute("data-width")).toBe("100%");
    expect(fill().getAttribute("data-bg")).toBe("#0f0");
    rerender(<ProgressBar percent={-5} />);
    expect(fill().getAttribute("data-width")).toBe("0%");
  });
});
