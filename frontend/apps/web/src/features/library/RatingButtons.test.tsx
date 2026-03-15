import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("RatingButtons", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders like and dislike buttons", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m1" />, { wrapper: createWrapper() });

    expect(screen.getByTitle("Like")).toBeInTheDocument();
    expect(screen.getByTitle("Dislike")).toBeInTheDocument();
  });

  it("shows 'Remove like' title when already liked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 1 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["rating", "m2"], { value: 1 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m2" />, { wrapper });

    expect(screen.getByTitle("Remove like")).toBeInTheDocument();
    expect(screen.getByTitle("Dislike")).toBeInTheDocument();
  });

  it("shows 'Remove dislike' title when already disliked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: -1 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["rating", "m3"], { value: -1 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m3" />, { wrapper });

    expect(screen.getByTitle("Like")).toBeInTheDocument();
    expect(screen.getByTitle("Remove dislike")).toBeInTheDocument();
  });

  it("calls setRating with 1 when like button is clicked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 1 });

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m4" />, { wrapper: createWrapper() });

    vi.mocked(eveyFetch).mockClear();
    fireEvent.click(screen.getByTitle("Like"));
    await waitFor(() =>
      expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/m4/rate", method: "PUT", data: { value: 1 } }),
      ),
    );
  });

  it("calls setRating with -1 when dislike button is clicked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: -1 });

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m5" />, { wrapper: createWrapper() });

    vi.mocked(eveyFetch).mockClear();
    fireEvent.click(screen.getByTitle("Dislike"));
    await waitFor(() =>
      expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/m5/rate", method: "PUT", data: { value: -1 } }),
      ),
    );
  });

  it("removes like when clicking like button while already liked", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ value: 0 });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["rating", "m6"], { value: 1 });
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children);

    const { RatingButtons } = await import("./RatingButtons");
    render(<RatingButtons mediaId="m6" />, { wrapper });

    vi.mocked(eveyFetch).mockClear();
    fireEvent.click(screen.getByTitle("Remove like"));
    await waitFor(() =>
      expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/m6/rate", method: "PUT", data: { value: 0 } }),
      ),
    );
  });
});
