import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { MediaGrid } from "./MediaGrid";

describe("MediaGrid", () => {
  it("renders grid of children", () => {
    render(
      <MediaGrid>
        <div data-testid="card-1">Card 1</div>
        <div data-testid="card-2">Card 2</div>
        <div data-testid="card-3">Card 3</div>
      </MediaGrid>,
    );
    expect(screen.getByTestId("card-1")).toBeInTheDocument();
    expect(screen.getByTestId("card-2")).toBeInTheDocument();
    expect(screen.getByTestId("card-3")).toBeInTheDocument();
  });

  it("shows EmptyState when no items", () => {
    const { container } = render(<MediaGrid>{null}</MediaGrid>);
    const grid = container.firstChild as HTMLElement;
    expect(grid).toBeInTheDocument();
    expect(grid.children).toHaveLength(0);
  });

  it("renders skeleton cards when isLoading", () => {
    const { container } = render(<MediaGrid isLoading>children ignored</MediaGrid>);
    const grid = container.firstChild as HTMLElement;
    expect(grid.children).toHaveLength(12);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });
});
