import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { SimilarSection } from "./MediaDetailSimilar";
import type { V1MediaItem } from "@evey/api";

const mockSimilarItems = vi.fn();
const mockListMediaItems = vi.fn();

vi.mock("@evey/api", () => ({
  useFeedServiceSimilarItems: (...args: unknown[]) => mockSimilarItems(...args),
  useLibraryServiceListMediaItems: (...args: unknown[]) => mockListMediaItems(...args),
}));

vi.mock("./MediaCard", () => ({
  MediaCard: ({ item, lite }: { item: { id: string; title?: string }; lite?: boolean }) => (
    <div data-testid={`card-${item.id}`}>{item.title}</div>
  ),
}));

function makeItem(overrides: Partial<V1MediaItem> = {}): V1MediaItem {
  return { id: "m-1", title: "Test Movie", genres: ["Action"], type: "movie", ...overrides } as V1MediaItem;
}

describe("SimilarSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListMediaItems.mockReturnValue({ data: undefined });
  });

  it("returns null when no similar or genre items", () => {
    mockSimilarItems.mockReturnValue({ data: { items: [] } });
    mockListMediaItems.mockReturnValue({ data: { items: [] } });
    const { container } = render(<SimilarSection item={makeItem()} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders similar items from feed", () => {
    mockSimilarItems.mockReturnValue({
      data: { items: [{ mediaItemId: "s-1", title: "Similar A" }, { mediaItemId: "s-2", title: "Similar B" }] },
    });
    render(<SimilarSection item={makeItem()} />);
    expect(screen.getByText("Similar Titles")).toBeInTheDocument();
    expect(screen.getByTestId("card-s-1")).toBeInTheDocument();
    expect(screen.getByTestId("card-s-2")).toBeInTheDocument();
  });

  it("falls back to genre items when no feed results", () => {
    mockSimilarItems.mockReturnValue({ data: { items: [] } });
    mockListMediaItems.mockReturnValue({
      data: { items: [{ id: "g-1", title: "Genre Movie" }, { id: "m-1", title: "Self" }] },
    });
    render(<SimilarSection item={makeItem()} />);
    expect(screen.getByTestId("card-g-1")).toBeInTheDocument();
    expect(screen.queryByTestId("card-m-1")).not.toBeInTheDocument();
  });

  it("filters out items without mediaItemId from feed", () => {
    mockSimilarItems.mockReturnValue({
      data: { items: [{ mediaItemId: "s-1", title: "Valid" }, { title: "No ID" }] },
    });
    render(<SimilarSection item={makeItem()} />);
    expect(screen.getByTestId("card-s-1")).toBeInTheDocument();
    expect(screen.queryByText("No ID")).not.toBeInTheDocument();
  });
});
