import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ContinueWatchingRow } from "./ContinueWatchingRow";

vi.mock("@evey/api", () => ({
  usePlaybackServiceGetResumeItems: vi.fn(),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
}));

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

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: () => null,
}));

import { usePlaybackServiceGetResumeItems } from "@evey/api";

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("ContinueWatchingRow", () => {
  it("renders media cards with progress", () => {
    vi.mocked(usePlaybackServiceGetResumeItems).mockReturnValue({
      data: {
        items: [
          { id: "1", title: "Movie A", type: "MEDIA_ITEM_TYPE_MOVIE" },
          { id: "2", title: "Movie B", type: "MEDIA_ITEM_TYPE_MOVIE" },
        ],
        progress: [
          { mediaItemId: "1", percentComplete: 50 },
          { mediaItemId: "2", percentComplete: 75 },
        ],
      },
      isLoading: false,
    } as ReturnType<typeof usePlaybackServiceGetResumeItems>);

    render(<ContinueWatchingRow />, { wrapper: Wrapper });
    expect(screen.getByText("Continue Watching")).toBeInTheDocument();
    expect(screen.getAllByText("Movie A").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Movie B").length).toBeGreaterThanOrEqual(1);
  });

  it("hides row when empty", () => {
    vi.mocked(usePlaybackServiceGetResumeItems).mockReturnValue({
      data: { items: [], progress: [] },
      isLoading: false,
    } as ReturnType<typeof usePlaybackServiceGetResumeItems>);

    const { container } = render(<ContinueWatchingRow />, { wrapper: Wrapper });
    expect(container.firstChild).toBeNull();
  });

  it("renders skeleton while loading", () => {
    vi.mocked(usePlaybackServiceGetResumeItems).mockReturnValue({
      data: undefined,
      isLoading: true,
    } as ReturnType<typeof usePlaybackServiceGetResumeItems>);

    render(<ContinueWatchingRow />, { wrapper: Wrapper });
    expect(screen.getByText("Continue Watching")).toBeInTheDocument();
  });
});
