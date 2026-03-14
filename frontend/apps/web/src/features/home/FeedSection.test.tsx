import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { FeedSection } from "./FeedSection";

describe("FeedSection", () => {
  it("renders title", () => {
    render(<FeedSection title="Recently Added"><div>item</div></FeedSection>);
    expect(screen.getByText("Recently Added")).toBeInTheDocument();
  });

  it("renders children when not loading", () => {
    render(
      <FeedSection title="Section">
        <div data-testid="child">card</div>
      </FeedSection>
    );
    expect(screen.getByTestId("child")).toBeInTheDocument();
  });

  it("renders skeletons when loading", () => {
    const { container } = render(
      <FeedSection title="Section" isLoading>
        <div data-testid="child">card</div>
      </FeedSection>
    );
    expect(screen.queryByTestId("child")).not.toBeInTheDocument();
    // 5 skeleton cards should render
    const skeletons = container.querySelectorAll(".animate-pulse");
    expect(skeletons.length).toBe(5);
  });
});
