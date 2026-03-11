import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaCard } from "./MediaCard";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    params,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    params?: Record<string, string>;
    [k: string]: unknown;
  }) => {
    const href = params?.mediaId ? to.replace("$mediaId", params.mediaId) : to;
    return (
      <a href={href} data-testid={rest["data-testid"] as string}>
        {children}
      </a>
    );
  },
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: () => null,
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

const baseItem = {
  id: "42",
  title: "Inception",
  year: 2010,
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
};

describe("MediaCard", () => {
  it("renders title", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const titles = screen.getAllByText("Inception");
    expect(titles.length).toBeGreaterThanOrEqual(1);
  });

  it("renders poster image with correct src", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const img = screen.getByAltText("Inception");
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute("src", "/api/v1/media/42/artwork/poster");
  });

  it("renders custom posterSrc", () => {
    render(<MediaCard item={baseItem} posterSrc="/custom.jpg" />, { wrapper: Wrapper });
    const img = screen.getByAltText("Inception");
    expect(img).toHaveAttribute("src", "/custom.jpg");
  });

  it("links to correct media page", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const link = screen.getByTestId("media-card");
    expect(link).toHaveAttribute("href", "/media/42");
  });

  it("shows year badge", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    expect(screen.getByText("2010")).toBeInTheDocument();
  });

  it("hides year when showYear=false", () => {
    render(<MediaCard item={baseItem} showYear={false} />, { wrapper: Wrapper });
    expect(screen.queryByText("2010")).not.toBeInTheDocument();
  });

  it("shows type label when showType=true", () => {
    render(<MediaCard item={baseItem} showType />, { wrapper: Wrapper });
    expect(screen.getByText("Film")).toBeInTheDocument();
  });

  it("shows progress bar when showProgress and progressPercent", () => {
    const { container } = render(
      <MediaCard item={baseItem} showProgress progressPercent={45} />,
      { wrapper: Wrapper },
    );
    const bar = container.querySelector("[style*='width: 45%']");
    expect(bar).toBeInTheDocument();
  });

  it("renders fallback initial when no poster", () => {
    render(<MediaCard item={{ id: undefined, title: "Arrival" }} />, { wrapper: Wrapper });
    expect(screen.getByText("A")).toBeInTheDocument();
  });
});
