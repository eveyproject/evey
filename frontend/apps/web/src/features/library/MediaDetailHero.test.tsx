import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { MediaDetailHero } from "./MediaDetailHero";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
  useRouterState: () => ({ history: { back: vi.fn() } }),
}));

vi.mock("@/components/ui/BackButton", () => ({
  BackButton: () => <button>Back</button>,
}));

vi.mock("@/components/ui/Breadcrumbs", () => ({
  Breadcrumbs: ({ items }: { items: Array<{ label: string }> }) => (
    <nav data-testid="breadcrumbs">{items.map((i) => i.label).join(" > ")}</nav>
  ),
}));

const baseItem = {
  id: "m1",
  title: "Inception",
  year: 2010,
  genres: ["Action", "Sci-Fi"],
  officialRating: "PG-13",
  communityRating: 8.8,
  providerIds: { imdb: "tt1375666", tmdb: "27205" },
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
};

const defaultProps = {
  item: baseItem,
  backdropUrl: "/backdrop.jpg",
  posterUrl: "/poster.jpg",
  duration: "2h 28m",
  breadcrumbItems: [{ label: "Movies", to: "/library/1" }, { label: "Inception" }],
  onEditToggle: vi.fn(),
};

describe("MediaDetailHero", () => {
  it("renders title", () => {
    render(<MediaDetailHero {...defaultProps} />);
    expect(screen.getByTestId("media-title")).toHaveTextContent("Inception");
  });

  it("renders backdrop image", () => {
    render(<MediaDetailHero {...defaultProps} />);
    const img = screen.getByTestId("media-backdrop");
    expect(img).toHaveAttribute("src", "/backdrop.jpg");
  });

  it("renders fallback when no backdrop", () => {
    render(<MediaDetailHero {...defaultProps} backdropUrl={undefined} />);
    expect(screen.queryByTestId("media-backdrop")).not.toBeInTheDocument();
  });

  it("renders year", () => {
    render(<MediaDetailHero {...defaultProps} />);
    expect(screen.getByText("2010")).toBeInTheDocument();
  });

  it("renders duration", () => {
    render(<MediaDetailHero {...defaultProps} />);
    expect(screen.getByText("2h 28m")).toBeInTheDocument();
  });

  it("renders official rating", () => {
    render(<MediaDetailHero {...defaultProps} />);
    expect(screen.getByText("PG-13")).toBeInTheDocument();
  });

  it("renders community rating with stars", () => {
    render(<MediaDetailHero {...defaultProps} />);
    expect(screen.getByText("8.8")).toBeInTheDocument();
  });

  it("renders genres", () => {
    render(<MediaDetailHero {...defaultProps} />);
    const genres = screen.getByTestId("media-genres");
    expect(genres).toHaveTextContent("Action");
    expect(genres).toHaveTextContent("Sci-Fi");
  });

  it("renders IMDb link", () => {
    render(<MediaDetailHero {...defaultProps} />);
    const link = screen.getByText("IMDb");
    expect(link).toHaveAttribute("href", "https://www.imdb.com/title/tt1375666/");
  });

  it("renders TMDB link for movie", () => {
    render(<MediaDetailHero {...defaultProps} />);
    const link = screen.getByText("TMDB");
    expect(link).toHaveAttribute("href", "https://www.themoviedb.org/movie/27205");
  });

  it("renders TMDB link for series", () => {
    const seriesItem = { ...baseItem, type: "MEDIA_ITEM_TYPE_SERIES" as const };
    render(<MediaDetailHero {...defaultProps} item={seriesItem} />);
    const link = screen.getByText("TMDB");
    expect(link).toHaveAttribute("href", "https://www.themoviedb.org/tv/27205");
  });

  it("hides genres when empty", () => {
    const item = { ...baseItem, genres: [] };
    render(<MediaDetailHero {...defaultProps} item={item} />);
    expect(screen.queryByTestId("media-genres")).not.toBeInTheDocument();
  });

  it("hides year when missing", () => {
    const item = { ...baseItem, year: undefined };
    render(<MediaDetailHero {...defaultProps} item={item} />);
    expect(screen.queryByText("2010")).not.toBeInTheDocument();
  });

  it("hides community rating when zero", () => {
    const item = { ...baseItem, communityRating: 0 };
    render(<MediaDetailHero {...defaultProps} item={item} />);
    expect(screen.queryByText("0.0")).not.toBeInTheDocument();
  });

  it("hides IMDb badge when no imdb id", () => {
    const item = { ...baseItem, providerIds: {} };
    render(<MediaDetailHero {...defaultProps} item={item} />);
    expect(screen.queryByText("IMDb")).not.toBeInTheDocument();
  });

  it("calls onEditToggle when edit button clicked", () => {
    const onEditToggle = vi.fn();
    render(<MediaDetailHero {...defaultProps} onEditToggle={onEditToggle} />);
    const editBtn = screen.getByRole("button", { name: /edit/i });
    fireEvent.click(editBtn);
    expect(onEditToggle).toHaveBeenCalledOnce();
  });

  it("renders parent item link", () => {
    const parentItem = { id: "p1", title: "Breaking Bad" };
    render(<MediaDetailHero {...defaultProps} parentItem={parentItem as any} />);
    expect(screen.getByText("Breaking Bad")).toBeInTheDocument();
  });

  it("hides duration when empty", () => {
    render(<MediaDetailHero {...defaultProps} duration="" />);
    expect(screen.queryByText("2h 28m")).not.toBeInTheDocument();
  });
});
