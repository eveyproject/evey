import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RecommendedRow } from "./RecommendedRow";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/hooks/useWatchedMap", () => ({
  useWatchedMap: () => new Map(),
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
  useFeedServiceRecommended: () => ({
    data: {
      items: [
        { mediaItemId: "r1", title: "Rec Movie", type: "MEDIA_ITEM_TYPE_MOVIE" },
      ],
    },
    isLoading: false,
  }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (s: (state: { accessToken: string | null }) => unknown) => s({ accessToken: null }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("RecommendedRow", () => {
  it("renders Recommended title", () => {
    render(<RecommendedRow />, { wrapper: createWrapper() });
    expect(screen.getByText(/recommended for you/i)).toBeInTheDocument();
  });

  it("renders recommended items", () => {
    render(<RecommendedRow />, { wrapper: createWrapper() });
    expect(screen.getAllByText("Rec Movie").length).toBeGreaterThan(0);
  });
});
