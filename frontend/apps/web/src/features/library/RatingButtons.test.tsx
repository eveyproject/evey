import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockRating = vi.fn(() => ({ data: 0 }));
const mockPut = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useRatingServiceGetMyThumbRating: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => {
    const raw = mockRating();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select({ value: raw.data }) : raw?.data };
  },
  ratingServicePutThumbRating: (id: string, body: unknown) => mockPut(id, body),
  getRatingServiceGetMyThumbRatingQueryKey: (id: string) => ["rating", id],
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("RatingButtons", () => {
  beforeEach(() => { vi.clearAllMocks(); mockRating.mockReturnValue({ data: 0 }); mockPut.mockResolvedValue({ value: 0 }); });

  it("renders like and dislike buttons", async () => {
    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m1" />, { wrapper: createWrapper() });
    expect(screen.getByTitle("Like")).toBeInTheDocument();
    expect(screen.getByTitle("Dislike")).toBeInTheDocument();
  });

  it("shows 'Remove like' title when already liked", async () => {
    mockRating.mockReturnValue({ data: 1 });
    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m2" />, { wrapper: createWrapper() });
    expect(screen.getByTitle("Remove like")).toBeInTheDocument();
  });

  it("shows 'Remove dislike' title when already disliked", async () => {
    mockRating.mockReturnValue({ data: -1 });
    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m3" />, { wrapper: createWrapper() });
    expect(screen.getByTitle("Remove dislike")).toBeInTheDocument();
  });

  it("calls setRating with 1 when like button is clicked", async () => {
    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m4" />, { wrapper: createWrapper() });
    fireEvent.click(screen.getByTitle("Like"));
    await waitFor(() => expect(mockPut).toHaveBeenCalledWith("m4", { value: 1 }));
  });

  it("calls setRating with -1 when dislike button is clicked", async () => {
    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m5" />, { wrapper: createWrapper() });
    fireEvent.click(screen.getByTitle("Dislike"));
    await waitFor(() => expect(mockPut).toHaveBeenCalledWith("m5", { value: -1 }));
  });
});

// The server was taught (2026-08-24) to stop claiming it knows a viewer's
// opinion when the read failed - it returns an error instead of a quiet
// `value: 0`. The screen never learned: `data = 0` on error paints a NEUTRAL
// thumb, which is a statement, and clicking it writes that statement over an
// opinion the viewer was never shown.
//
// Disabled is the honest state here: it says "not now" without inventing a
// third look for "unknown".
describe("RatingButtons when the rating could not be read", () => {
  // The beforeEach above lives inside the first describe, so this block starts
  // with whatever the previous tests left on the mocks.
  beforeEach(() => {
    vi.clearAllMocks();
    mockPut.mockResolvedValue({ value: 0 });
  });

  it("does not let a click overwrite an opinion it never saw", async () => {
    mockRating.mockReturnValue({ data: undefined, isError: true } as never);
    const { RatingButtons } = await import("./RatingButtons");
    render(React.createElement(RatingButtons, { mediaId: "m1" }), { wrapper: createWrapper() });

    const like = screen.getByLabelText("Like") as HTMLButtonElement;
    expect(like.disabled).toBe(true);
    fireEvent.click(like);
    await waitFor(() => expect(mockPut).not.toHaveBeenCalled());
  });

  it("stays usable once the rating is readable again", async () => {
    mockRating.mockReturnValue({ data: 0 });
    const { RatingButtons } = await import("./RatingButtons");
    render(React.createElement(RatingButtons, { mediaId: "m1" }), { wrapper: createWrapper() });

    const like = screen.getByLabelText("Like") as HTMLButtonElement;
    expect(like.disabled).toBe(false);
  });
});
