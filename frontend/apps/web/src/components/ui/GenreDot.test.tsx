import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { genreDotColor } from "@evey/api";
import { GenreDot } from "./GenreDot";

// The dot is decoration a screen reader skips, wears the shared genre hash
// colour so the same genre matches on every client, and keeps a ring in
// the surrounding text colour so it stays visible on any surface.
describe("GenreDot (web)", () => {
  it("is hidden from assistive tech and coloured by the shared genre hash", () => {
    const { container } = render(<GenreDot label="Drama" />);
    const dot = container.firstElementChild as HTMLElement;
    expect(dot.getAttribute("aria-hidden")).toBe("true");
    // jsdom normalises hsl() to rgb(): compare through the same normaliser.
    const probe = document.createElement("span");
    probe.style.backgroundColor = genreDotColor("Drama");
    expect(dot.style.backgroundColor).toBe(probe.style.backgroundColor);
    expect(dot.style.boxShadow).toContain("currentColor");
    const other = render(<GenreDot label="Comedy" />).container.firstElementChild as HTMLElement;
    expect(other.style.backgroundColor).not.toBe(dot.style.backgroundColor);
  });
});
