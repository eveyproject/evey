import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const librariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() }));
const recentHook = vi.fn(() => ({ data: { items: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() }));
const feedConfigHook = vi.fn(() => ({ data: undefined, isError: false }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, ...props }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: props.to }, children),
  useNavigate: () => vi.fn(),
}));

const resumeHook = vi.fn(() => ({ data: { items: [], progress: [] }, isLoading: false }));

vi.mock("@evey/api", () => ({
  useFeedServiceRecommended: () => ({ data: { items: [] }, isLoading: false }),
  useFeedServiceGetFeedConfig: (...args: unknown[]) => feedConfigHook(...(args as [])),
  useFeedServiceRecentlyAdded: () => recentHook(),
  usePlaybackServiceGetResumeItems: () => resumeHook(),
  usePlaybackServiceGetNextUp: () => ({ data: { items: [] }, isLoading: false }),
  useCollectionServiceListCollections: () => ({ data: { collections: [] }, isLoading: false }),
  useActivityServiceGetPopularMedia: () => ({ data: [], isLoading: false }),
  usePlaybackServiceGetWatchHistory: () => ({ data: { items: [] }, isLoading: false }),
  useFeedServiceBecauseYouWatched: () => ({ data: { items: [] }, isLoading: false }),
  useLibraryServiceListLibraries: () => librariesHook(),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] }, isLoading: false }),
  useMusicServiceGetAlbums: () => ({ data: { albums: [] }, isLoading: false }),
  usePhotoServiceListPhotoAlbums: () => ({ data: { albums: [] }, isLoading: false }),
}));

// The resume-row test only checks the WIRING (does the slider hand the card a
// percent at all - the audited bug was a prop never passed), so the card is a
// probe that echoes its props.
vi.mock("../../features/library/MediaCardWithModals", () => ({
  MediaCardWithModals: ({ item, progressPercent, showProgress }: { item: { title?: string }; progressPercent?: number; showProgress?: boolean }) =>
    React.createElement(
      "div",
      { "data-testid": "resume-card", "data-progress": String(progressPercent), "data-show-progress": String(showProgress) },
      item.title,
    ),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "p1" }),
}));

// CustomizePanel mounts unconditionally at the bottom of HomePage (even when
// closed) and pulls in its own set of feed-config hooks - stub it out so this
// test only exercises HomePage's own error/empty/content branching.
vi.mock("@/features/home/CustomizePanel", () => ({
  CustomizePanel: () => null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getHomePage() {
  const mod = await import("./home.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("HomePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    librariesHook.mockReturnValue({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() });
    recentHook.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() });
    feedConfigHook.mockReturnValue({ data: undefined, isError: false });
  });

  it("shows the empty-library state with an action when there is no error and no content", async () => {
    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/your library is empty/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /go to libraries/i })).toBeInTheDocument();
  });

  it("shows a distinct error state with a retry action when the library fetch fails - not the empty-library message", async () => {
    const refetchLibraries = vi.fn();
    librariesHook.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: Object.assign(new Error("boom"), { httpStatus: 500 }), refetch: refetchLibraries });

    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/your library is empty/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetchLibraries).toHaveBeenCalled();
  });

  // The web was the last client on FeedService.ResumePlayback, whose FeedItem
  // carries no duration/percent: the "Continue watching" row could therefore
  // NEVER show a progress bar (audit 2026-08-09). It now rides the same
  // PlaybackService.GetResumeItems as mobile and TV, progress included.
  it("hands each resume card its watch percent", async () => {
    librariesHook.mockReturnValue({
      data: { libraries: [{ id: "lib-1", name: "Some Library", type: "movies" }] },
      isLoading: false, isError: false, error: null, refetch: vi.fn(),
    } as never);
    resumeHook.mockReturnValue({
      data: {
        items: [{ id: "mi-1", title: "Some Movie", type: "MEDIA_ITEM_TYPE_MOVIE" }],
        progress: [{ mediaItemId: "mi-1", percentComplete: 45, positionMs: "60000" }],
      },
      isLoading: false,
    } as never);

    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Some Movie")).toBeInTheDocument());
    const card = screen.getByText("Some Movie").closest("[data-testid='resume-card']") as HTMLElement;
    expect(card.getAttribute("data-progress")).toBe("45");
    expect(card.getAttribute("data-show-progress")).toBe("true");
  });

  it("shows the offline copy when the failure carries no httpStatus (network down)", async () => {
    recentHook.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: new TypeError("Failed to fetch"), refetch: vi.fn() });

    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Offline")).toBeInTheDocument());
  });
});
