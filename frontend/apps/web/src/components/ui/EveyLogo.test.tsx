import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { EveyWordmark, EveyMark } from "./EveyLogo";

// The wordmark is an accessible image whose letters follow the text colour
// while the V stays brand red; the signature band is a mask cut so the
// surface shows through; two logos on one page must not share a mask id.
describe("EveyLogo (web)", () => {
  it("draws the wordmark as an accessible image with a red V behind a mask cut", () => {
    const { container } = render(<EveyWordmark height={40} className="logo" />);
    const svg = container.querySelector("svg")!;
    expect(svg.getAttribute("role")).toBe("img");
    expect(svg.getAttribute("aria-label")).toBe("Evey");
    expect(svg.getAttribute("height")).toBe("40");
    expect(svg.classList.contains("logo")).toBe(true);
    const fills = Array.from(svg.querySelectorAll("g path")).map((p) => p.getAttribute("fill"));
    expect(fills).toEqual(["currentColor", "#DC2626", "currentColor", "currentColor"]);
    const mask = svg.querySelector("mask")!;
    expect(svg.querySelector("g")?.getAttribute("mask")).toBe(`url(#${mask.id})`);
  });

  it("gives each logo its own mask id so two on a page do not cut each other", () => {
    const { container } = render(<><EveyWordmark /><EveyMark /><EveyMark height={12} /></>);
    const ids = Array.from(container.querySelectorAll("mask")).map((m) => m.id);
    expect(ids).toHaveLength(3);
    expect(new Set(ids).size).toBe(3);
    const marks = container.querySelectorAll("svg");
    expect(marks[2].getAttribute("width")).toBe("12");
    expect(marks[2].getAttribute("height")).toBe("12");
  });
});
