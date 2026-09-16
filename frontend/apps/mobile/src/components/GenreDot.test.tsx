import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ style, accessibilityElementsHidden, importantForAccessibility }: any) =>
    React.createElement("div", { "data-size": style.width, "data-radius": style.borderRadius, "data-bg": style.backgroundColor, "data-hidden": String(accessibilityElementsHidden), "data-ifa": importantForAccessibility }),
}));

import { genreDotColor } from "@evey/api";
import { GenreDot } from "./GenreDot";

// The mobile twin of the web dot: same shared hash so a genre wears the same
// colour on both, a circle of the asked size, hidden from assistive tech.
describe("GenreDot (mobile)", () => {
  it("is a circle of the asked size in the shared genre colour, hidden from assistive tech", () => {
    const { container, rerender } = render(<GenreDot label="Drama" />);
    const dot = () => container.firstElementChild as HTMLElement;
    expect(dot().getAttribute("data-size")).toBe("8");
    expect(dot().getAttribute("data-radius")).toBe("4");
    expect(dot().getAttribute("data-bg")).toBe(genreDotColor("Drama"));
    expect(dot().getAttribute("data-hidden")).toBe("true");
    expect(dot().getAttribute("data-ifa")).toBe("no-hide-descendants");
    rerender(<GenreDot label="Drama" size={12} />);
    expect(dot().getAttribute("data-size")).toBe("12");
    expect(dot().getAttribute("data-radius")).toBe("6");
  });
});
