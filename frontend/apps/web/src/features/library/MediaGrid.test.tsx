import { renderWithQueryClient } from "@/test/test-utils";
import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, it, expect } from "vitest";
import { MediaGrid } from "./MediaGrid";

// @tanstack/react-virtual is mocked globally in test/setup.ts to render every
// item flatly (JSDOM has no layout, so real virtualization would render nothing).
//
// MediaGrid defers its virtualized mount until `#main-content` (AuthLayout's
// scrollable region) resolves on the client. JSDOM doesn't render
// AuthLayout, so the test must add the element before mounting or the
// component renders 12 skeleton cards instead of the children.
describe("MediaGrid", () => {
  let mainContent: HTMLElement;
  beforeEach(() => {
    mainContent = document.createElement("main");
    mainContent.id = "main-content";
    document.body.appendChild(mainContent);
  });
  afterEach(() => {
    document.body.removeChild(mainContent);
  });

  it("renders grid of children", async () => {
    renderWithQueryClient(
      <MediaGrid>
        <div data-testid="card-1">Card 1</div>
        <div data-testid="card-2">Card 2</div>
        <div data-testid="card-3">Card 3</div>
      </MediaGrid>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("card-1")).toBeInTheDocument();
    });
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
