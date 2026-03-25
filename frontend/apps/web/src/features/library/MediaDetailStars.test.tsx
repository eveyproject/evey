import React from "react";
import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";

vi.mock("lucide-react", () => ({
  Star: (props: Record<string, unknown>) =>
    React.createElement("svg", { "data-testid": "star", "data-fill": props.fill }),
}));

async function getStars() {
  const mod = await import("./MediaDetailStars");
  return mod.Stars;
}

describe("Stars", () => {
  it("renders 5 star icons", async () => {
    const Stars = await getStars();
    const { container } = render(React.createElement(Stars, { rating: 6 }));
    expect(container.querySelectorAll("[data-testid='star']")).toHaveLength(5);
  });

  it("fills correct number of stars for rating 8 (4 out of 5)", async () => {
    const Stars = await getStars();
    const { container } = render(React.createElement(Stars, { rating: 8 }));
    const stars = container.querySelectorAll("[data-testid='star']");
    const filled = Array.from(stars).filter((s) => s.getAttribute("data-fill") === "var(--accent)");
    expect(filled).toHaveLength(4);
  });

  it("fills 0 stars for rating 0", async () => {
    const Stars = await getStars();
    const { container } = render(React.createElement(Stars, { rating: 0 }));
    const stars = container.querySelectorAll("[data-testid='star']");
    const filled = Array.from(stars).filter((s) => s.getAttribute("data-fill") === "var(--accent)");
    expect(filled).toHaveLength(0);
  });

  it("fills all 5 stars for rating 10", async () => {
    const Stars = await getStars();
    const { container } = render(React.createElement(Stars, { rating: 10 }));
    const stars = container.querySelectorAll("[data-testid='star']");
    const filled = Array.from(stars).filter((s) => s.getAttribute("data-fill") === "var(--accent)");
    expect(filled).toHaveLength(5);
  });
});
