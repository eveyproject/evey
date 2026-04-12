import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockStar = vi.fn(() => ({ data: 0 }));
const mockAgg = vi.fn(() => ({ data: undefined }));
const mockPost = vi.fn();
const mockDelete = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useRatingServiceGetStarRating: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => {
    const raw = mockStar();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select({ value: raw.data }) : raw?.data };
  },
  useRatingServiceGetRatingAggregate: () => mockAgg(),
  ratingServicePostStarRating: (id: string, body: unknown) => mockPost(id, body),
  ratingServiceDeleteStarRating: (id: string) => mockDelete(id),
  getRatingServiceGetStarRatingQueryKey: (id: string) => ["star-rating", id],
  getRatingServiceGetRatingAggregateQueryKey: (id: string) => ["star-rating-agg", id],
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("StarRating", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStar.mockReturnValue({ data: 0 });
    mockAgg.mockReturnValue({ data: undefined });
    mockPost.mockResolvedValue({ value: 0 });
  });

  it("renders 5 star buttons and 'Your rating' label", async () => {
    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m1" />, { wrapper: createWrapper() });
    expect(screen.getByText("Your rating:")).toBeInTheDocument();
    for (let i = 1; i <= 5; i++) {
      expect(screen.getByRole("button", { name: `Rate ${i} star${i > 1 ? "s" : ""}` })).toBeInTheDocument();
    }
  });

  it("shows current rating as x/5 when rating > 0", async () => {
    mockStar.mockReturnValue({ data: 3 });
    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m2" />, { wrapper: createWrapper() });
    expect(screen.getByText("3/5")).toBeInTheDocument();
  });

  it("calls POST rating when a star is clicked", async () => {
    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m3" />, { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: "Rate 4 stars" }));
    await waitFor(() => expect(mockPost).toHaveBeenCalledWith("m3", { value: 4 }));
  });

  it("shows community rating when aggregate data has count > 0", async () => {
    mockAgg.mockReturnValue({ data: { average: 4.2, count: 10 } });
    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m4" />, { wrapper: createWrapper() });
    expect(screen.getByText(/Community:.*4\.2\/5.*10 rating/)).toBeInTheDocument();
  });

  it("does not show community rating when count is 0", async () => {
    mockAgg.mockReturnValue({ data: { average: 0, count: 0 } });
    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m5" />, { wrapper: createWrapper() });
    expect(screen.queryByText(/Community:/)).not.toBeInTheDocument();
  });
});

describe("StarRatingBadge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStar.mockReturnValue({ data: 0 });
  });

  it("renders nothing when rating is 0", async () => {
    const { StarRatingBadge } = await import("./StarRating");
    const { container } = render(<StarRatingBadge mediaId="b1" />, { wrapper: createWrapper() });
    expect(container.firstChild).toBeNull();
  });

  it("renders badge with rating when rating > 0", async () => {
    mockStar.mockReturnValue({ data: 5 });
    const { StarRatingBadge } = await import("./StarRating");
    render(<StarRatingBadge mediaId="b2" />, { wrapper: createWrapper() });
    expect(screen.getByTitle("Your rating: 5/5")).toBeInTheDocument();
  });
});
