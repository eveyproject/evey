import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { V1MediaItem } from "@evey/api";

const searchMock = vi.fn();
const eveyFetchMock = vi.fn();
let mockChildren: Record<string, V1MediaItem[]> = {};

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  libraryServiceSearchMedia: (...args: unknown[]) => searchMock(...args),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  useLibraryServiceListMediaItems: (params: { parentId?: string }) => ({
    data: { items: mockChildren[params.parentId ?? ""] ?? [] },
  }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...rest }: { children: React.ReactNode; [k: string]: unknown }) => (
    <a {...rest}>{children}</a>
  ),
}));

// The 300ms debounce is an input nicety, not behaviour under test.
vi.mock("@/hooks/useDebounce", () => ({ useDebounce: <T,>(v: T) => v }));

import { IntroTestLab, pickSampleEpisode } from "./IntroTestLab";

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const SERIES: V1MediaItem = { id: "sr-1", title: "Some Series", type: "MEDIA_ITEM_TYPE_SERIES" } as V1MediaItem;
const MOVIE: V1MediaItem = { id: "mv-1", title: "Some Movie", type: "MEDIA_ITEM_TYPE_MOVIE" } as V1MediaItem;
const EPISODE_ITEM: V1MediaItem = { id: "ep-x", title: "Stray Episode", type: "MEDIA_ITEM_TYPE_EPISODE" } as V1MediaItem;

function seedSeries() {
  mockChildren = {
    "sr-1": [
      { id: "se-1", title: "Season 1", type: "MEDIA_ITEM_TYPE_SEASON" } as V1MediaItem,
      { id: "se-2", title: "Season 2", type: "MEDIA_ITEM_TYPE_SEASON" } as V1MediaItem,
    ],
    "se-1": [
      { id: "ep-1", title: "One", indexNumber: 1, type: "MEDIA_ITEM_TYPE_EPISODE" },
      { id: "ep-2", title: "Two", indexNumber: 2, type: "MEDIA_ITEM_TYPE_EPISODE" },
      { id: "ep-3", title: "Three", indexNumber: 3, type: "MEDIA_ITEM_TYPE_EPISODE" },
    ] as V1MediaItem[],
  };
}

describe("pickSampleEpisode", () => {
  it("picks the mid-season episode and survives an empty list", () => {
    expect(pickSampleEpisode([])).toBeNull();
    expect(pickSampleEpisode([EPISODE_ITEM])?.id).toBe("ep-x");
    const eps = [{ id: "a" }, { id: "b" }, { id: "c" }] as V1MediaItem[];
    expect(pickSampleEpisode(eps)?.id).toBe("b");
  });
});

describe("IntroTestLab", () => {
  beforeEach(() => {
    searchMock.mockReset();
    eveyFetchMock.mockReset();
    mockChildren = {};
  });

  it("lists only series and movies from the search results", async () => {
    const user = userEvent.setup();
    searchMock.mockResolvedValue({ items: [SERIES, MOVIE, EPISODE_ITEM] });
    render(<IntroTestLab />, { wrapper });
    await user.type(screen.getByTestId("intro-test-search"), "some");
    await waitFor(() => expect(screen.getByText("Some Series")).toBeInTheDocument());
    expect(screen.getByText("Some Movie")).toBeInTheDocument();
    expect(screen.queryByText("Stray Episode")).toBeNull();
  });

  it("dry-runs the mid-season witness episode of a picked series", async () => {
    const user = userEvent.setup();
    searchMock.mockResolvedValue({ items: [SERIES] });
    seedSeries();
    eveyFetchMock.mockResolvedValue([
      { type: "intro", startMs: 80000, endMs: 107000, confidence: 0.8, source: "audio_cluster" },
    ]);
    render(<IntroTestLab />, { wrapper });
    await user.type(screen.getByTestId("intro-test-search"), "some");
    await user.click(await screen.findByText("Some Series"));
    await user.click(screen.getByTestId("intro-test-run"));
    await waitFor(() => expect(screen.getByTestId("intro-test-result")).toBeInTheDocument());
    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/admin/media/ep-2/detect-segments?dry_run=true",
        method: "POST",
      }),
    );
    expect(screen.getByTestId("intro-test-verdict-found")).toBeInTheDocument();
    expect(screen.getByText("1:20 → 1:47")).toBeInTheDocument();
    expect(screen.getByText("audio_cluster")).toBeInTheDocument();
  });

  it("shows the no-candidate verdict on an empty dry-run", async () => {
    const user = userEvent.setup();
    searchMock.mockResolvedValue({ items: [MOVIE] });
    eveyFetchMock.mockResolvedValue([]);
    render(<IntroTestLab />, { wrapper });
    await user.type(screen.getByTestId("intro-test-search"), "some");
    await user.click(await screen.findByText("Some Movie"));
    await user.click(screen.getByTestId("intro-test-run"));
    await waitFor(() => expect(screen.getByTestId("intro-test-verdict-none")).toBeInTheDocument());
  });

  it("applies at the SEASON granularity for a series", async () => {
    const user = userEvent.setup();
    searchMock.mockResolvedValue({ items: [SERIES] });
    seedSeries();
    eveyFetchMock.mockResolvedValue({ status: "started" });
    render(<IntroTestLab />, { wrapper });
    await user.type(screen.getByTestId("intro-test-search"), "some");
    await user.click(await screen.findByText("Some Series"));
    await user.click(screen.getByTestId("intro-test-apply"));
    await waitFor(() =>
      expect(eveyFetchMock).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/media/se-1/detect-segments",
          method: "POST",
        }),
      ),
    );
  });

  it("applies directly on a movie", async () => {
    const user = userEvent.setup();
    searchMock.mockResolvedValue({ items: [MOVIE] });
    eveyFetchMock.mockResolvedValue({ status: "started" });
    render(<IntroTestLab />, { wrapper });
    await user.type(screen.getByTestId("intro-test-search"), "some");
    await user.click(await screen.findByText("Some Movie"));
    await user.click(screen.getByTestId("intro-test-apply"));
    await waitFor(() =>
      expect(eveyFetchMock).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/media/mv-1/detect-segments",
          method: "POST",
        }),
      ),
    );
  });
});
