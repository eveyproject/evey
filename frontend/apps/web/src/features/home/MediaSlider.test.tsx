import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { MediaSlider } from "./MediaSlider";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

describe("MediaSlider", () => {
  it("renders title", () => {
    render(<MediaSlider title="New Movies"><div>item</div></MediaSlider>);
    expect(screen.getByText("New Movies")).toBeInTheDocument();
  });

  it("renders children when not loading", () => {
    render(
      <MediaSlider title="Movies">
        <div data-testid="card">card</div>
      </MediaSlider>
    );
    expect(screen.getByTestId("card")).toBeInTheDocument();
  });

  it("renders skeletons when loading", () => {
    const { container } = render(
      <MediaSlider title="Movies" isLoading skeletonCount={3}>
        <div data-testid="card">card</div>
      </MediaSlider>
    );
    expect(screen.queryByTestId("card")).not.toBeInTheDocument();
    // Each skeleton card has a flex-shrink-0 root div
    const skeletons = container.querySelectorAll(".flex-shrink-0");
    expect(skeletons.length).toBe(3);
  });

  it("renders See all link when seeAllHref provided", () => {
    render(
      <MediaSlider title="Movies" seeAllHref="/library">
        <div>item</div>
      </MediaSlider>
    );
    const link = screen.getByText("See all →");
    expect(link).toBeInTheDocument();
    expect(link.closest("a")).toHaveAttribute("href", "/library");
  });

  it("does not render See all when no seeAllHref", () => {
    render(<MediaSlider title="Movies"><div>item</div></MediaSlider>);
    expect(screen.queryByText("See all →")).not.toBeInTheDocument();
  });

  it("has data-testid media-slider", () => {
    render(<MediaSlider title="Movies"><div>item</div></MediaSlider>);
    expect(screen.getByTestId("media-slider")).toBeInTheDocument();
  });
});
