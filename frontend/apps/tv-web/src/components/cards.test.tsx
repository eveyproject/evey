import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));

import { GridCard, PosterCard } from "./cards";

const badges = (c: HTMLElement) => [...c.querySelectorAll(".tech-row .tech-badge")].map((b) => b.textContent);

// The home rows and the library grid draw the same badges, from one component.
describe("quality badges", () => {
  it("are on the home's poster card", () => {
    const { container } = render(<PosterCard title="Film" width={180} resolution="4K" hdr onPress={() => {}} />);
    expect(badges(container)).toEqual(["4K", "HDR"]);
  });

  it("are on the library's grid card, raised over a progress bar", () => {
    const { container } = render(<GridCard title="Film" resolution="1080p" percent={40} onPress={() => {}} />);
    expect(badges(container)).toEqual(["1080p"]);
    expect(container.querySelector(".tech-row")?.className).toContain("raised");
  });

  it("are absent without a resolution or HDR (a series)", () => {
    const { container } = render(<PosterCard title="Show" width={180} onPress={() => {}} />);
    expect(container.querySelector(".tech-row")).toBeNull();
  });
});
