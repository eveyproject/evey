import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Skeleton, MediaCardSkeleton, MediaGridSkeleton, SliderRowSkeleton } from "./Skeleton";

describe("Skeleton", () => {
  it("renders skeleton element with animate-pulse class", () => {
    const { container } = render(<Skeleton />);
    expect(container.firstChild).toHaveClass("animate-pulse");
  });

  it("applies custom className", () => {
    const { container } = render(<Skeleton className="h-8 w-32" />);
    expect(container.firstChild).toHaveClass("h-8", "w-32");
  });
});

describe("MediaCardSkeleton", () => {
  it("renders skeleton element", () => {
    const { container } = render(<MediaCardSkeleton />);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });
});

describe("MediaGridSkeleton", () => {
  it("renders default 12 skeleton cards", () => {
    const { container } = render(<MediaGridSkeleton />);
    const cards = container.querySelectorAll(".animate-pulse");
    expect(cards.length).toBeGreaterThanOrEqual(1);
  });

  it("renders count skeleton cards", () => {
    const { container } = render(<MediaGridSkeleton count={4} />);
    const grid = container.firstChild as HTMLElement;
    expect(grid.children).toHaveLength(4);
  });
});

describe("SliderRowSkeleton", () => {
  it("renders skeleton row", () => {
    const { container } = render(<SliderRowSkeleton />);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });
});
