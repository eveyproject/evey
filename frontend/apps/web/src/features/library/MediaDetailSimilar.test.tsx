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
  MediaCard: ({ item }: { item: { id: string; title?: string }; lite?: boolean }) => (
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

  // No same-genre fallback: a title sharing one catch-all genre is not
  // "similar", and a row of them lied (2026-10-01).
  it("shows no row when the server has no similar title, even with same-genre titles around", () => {
    mockSimilarItems.mockReturnValue({ data: { items: [] } });
    mockListMediaItems.mockReturnValue({
      data: { items: [{ id: "g-1", title: "Genre Movie" }] },
    });
    const { container } = render(<SimilarSection item={makeItem()} />);
    expect(container.innerHTML).toBe("");
    expect(mockListMediaItems).not.toHaveBeenCalled();
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
