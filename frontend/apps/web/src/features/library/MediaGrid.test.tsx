import React from "react";
import { renderWithQueryClient } from "@/test/test-utils";
import { screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { MediaGrid } from "./MediaGrid";

vi.mock("react-virtuoso", () => ({
  VirtuosoGrid: ({ totalCount, itemContent, components }: {
    totalCount: number;
    itemContent: (index: number) => React.ReactNode;
    components: { List: React.ComponentType<React.HTMLAttributes<HTMLDivElement>>; Item: React.ComponentType<React.HTMLAttributes<HTMLDivElement> & { children?: React.ReactNode }> };
  }) => {
    const List = components.List;
    const Item = components.Item;
    return (
      <List>
        {Array.from({ length: totalCount }, (_, i) => (
          <Item key={i}>{itemContent(i)}</Item>
        ))}
      </List>
    );
  },
}));

describe("MediaGrid", () => {
  it("renders grid of children", () => {
    renderWithQueryClient(
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
    const { container } = renderWithQueryClient(<MediaGrid>{null}</MediaGrid>);
    expect(container.querySelector("[data-testid='card-1']")).toBeNull();
  });

  it("renders skeleton cards when isLoading", () => {
    const { container } = renderWithQueryClient(<MediaGrid isLoading>children ignored</MediaGrid>);
    const grid = container.firstChild as HTMLElement;
    expect(grid.children).toHaveLength(12);
    expect(container.querySelector(".animate-pulse")).toBeInTheDocument();
  });
});
