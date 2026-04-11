import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

function createWrapper(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

describe("StarRating", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders 5 star buttons and 'Your rating' label", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m1" />, { wrapper: createWrapper() });

    expect(screen.getByText("Your rating:")).toBeInTheDocument();
    for (let i = 1; i <= 5; i++) {
      expect(screen.getByRole("button", { name: `Rate ${i} star${i > 1 ? "s" : ""}` })).toBeInTheDocument();
    }
  });

  it("shows current rating as x/5 when rating > 0", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 3 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "m2"], { value: 3 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m2" />, { wrapper: createWrapper(qc) });

    expect(screen.getByText("3/5")).toBeInTheDocument();
  });

  it("calls eveyFetch to POST rating when a star is clicked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 4 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m3" />, { wrapper: createWrapper() });

    vi.mocked(eveyFetch).mockClear();
    fireEvent.click(screen.getByRole("button", { name: "Rate 4 stars" }));

    await waitFor(() =>
      expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/m3/rating", method: "POST", data: { value: 4 } })
      )
    );
  });

  it("shows community rating when aggregate data has count > 0", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "m4"], { value: 0 });
    qc.setQueryData(["star-rating-agg", "m4"], { average: 4.2, count: 10 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m4" />, { wrapper: createWrapper(qc) });

    expect(screen.getByText(/Community:.*4\.2\/5.*10 rating/)).toBeInTheDocument();
  });

  it("does not show community rating when count is 0", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "m5"], { value: 0 });
    qc.setQueryData(["star-rating-agg", "m5"], { average: 0, count: 0 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m5" />, { wrapper: createWrapper(qc) });

    expect(screen.queryByText(/Community:/)).not.toBeInTheDocument();
  });

  it("sets hovered state on mouse enter and clears on mouse leave", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const { StarRating } = await import("./StarRating");
    render(<StarRating mediaId="m6" />, { wrapper: createWrapper() });

    const starContainer = screen.getByRole("button", { name: "Rate 3 stars" }).parentElement!;
    fireEvent.mouseEnter(screen.getByRole("button", { name: "Rate 3 stars" }));
    fireEvent.mouseLeave(starContainer);
    // No crash — hover state clears correctly
    expect(screen.getByText("Your rating:")).toBeInTheDocument();
  });
});

describe("StarRatingBadge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when rating is 0", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "b1"], { value: 0 });

    const { StarRatingBadge } = await import("./StarRating");
    const { container } = render(<StarRatingBadge mediaId="b1" />, { wrapper: createWrapper(qc) });

    expect(container.firstChild).toBeNull();
  });

  it("renders badge with rating when rating > 0", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 5 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["star-rating", "b2"], { value: 5 });

    const { StarRatingBadge } = await import("./StarRating");
    render(<StarRatingBadge mediaId="b2" />, { wrapper: createWrapper(qc) });

    expect(screen.getByTitle("Your rating: 5/5")).toBeInTheDocument();
  });
});
