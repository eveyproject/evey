import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaDetail } from "./MediaDetail";

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
    const href = params
      ? Object.entries(params).reduce(
          (acc, [k, v]) => acc.replace(`$${k}`, v),
          to,
        )
      : to;
    return <a href={href} {...rest}>{children}</a>;
  },
  useRouterState: () => ({ history: { back: vi.fn() } }),
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  useFeedServiceSimilarItems: () => ({ data: undefined }),
  useLibraryServiceListMediaItems: () => ({ data: undefined }),
  useLibraryServiceGetMediaItem: () => ({ data: undefined }),
  useLibraryServiceGetSeriesAggregate: () => ({ data: undefined }),
  useUserServiceGetCurrentUser: () => ({ data: undefined }),
  useUserServiceListProfiles: () => ({ data: undefined }),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] } }),
  useMetadataServiceSetMetadataOverride: () => ({ mutateAsync: vi.fn(), isError: false }),
  useMetadataServiceRefreshMetadata: () => ({ mutateAsync: vi.fn() }),
  useMetadataServiceGetArtwork: () => ({ data: undefined }),
  useMetadataServiceListMetadataOverrides: () => ({ data: undefined }),
  useMetadataServiceRemoveMetadataOverride: () => ({ mutateAsync: vi.fn() }),
  getMetadataServiceListMetadataOverridesQueryKey: () => ["overrides"],
  eveyFetch: vi.fn(),
  useParentalServiceGetParentalSettings: () => ({ data: undefined }),
  useRatingServiceGetMyThumbRating: () => ({ data: 0 }),
  useRatingServiceGetRatingAggregate: () => ({ data: undefined }),
  ratingServicePutThumbRating: vi.fn(),
  getRatingServiceGetMyThumbRatingQueryKey: (id: string) => ["rating", id],
  getRatingServiceGetStarRatingQueryKey: (id: string) => ["star-rating", id],
  getRatingServiceGetRatingAggregateQueryKey: (id: string) => ["star-rating-agg", id],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel: (s: { accessToken: string; profileId: string | null; isAdmin: boolean }) => unknown) =>
    sel({ accessToken: "tok", profileId: null, isAdmin: false }),
}));

vi.mock("@/components/ui/BackButton", () => ({
  BackButton: () => <button>Back</button>,
}));

vi.mock("@/components/ui/Breadcrumbs", () => ({
  Breadcrumbs: () => null,
}));

vi.mock("../playback/DownloadButton", () => ({
  DownloadButton: () => <button>Download</button>,
}));

vi.mock("./FavoriteButton", () => ({
  FavoriteButton: () => <button>Favorite</button>,
}));

vi.mock("../playlist/AddToPlaylistModal", () => ({
  AddToPlaylistModal: () => null,
}));

vi.mock("./MediaDetailSagas", () => ({
  SagaSection: () => null,
}));

vi.mock("./StreamList", () => ({
  StreamList: () => null,
}));

vi.mock("./MediaCard", () => ({
  MediaCard: ({ item }: { item: { title?: string } }) => <div>{item.title}</div>,
}));

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

const baseItem = {
  id: "media-1",
  title: "Inception",
  overview: "A thief who steals secrets through dreams.",
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
  year: 2010,
  genres: ["Action", "Sci-Fi"],
  people: [
    { id: "p1", name: "Leonardo DiCaprio", type: "PERSON_TYPE_ACTOR" as const, role: "Cobb" },
    { id: "p2", name: "Christopher Nolan", type: "PERSON_TYPE_DIRECTOR" as const },
  ],
};

describe("MediaDetail", () => {
  it("renders title and overview", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    expect(screen.getByTestId("media-title")).toHaveTextContent("Inception");
    expect(screen.getByTestId("media-overview")).toHaveTextContent(
      "A thief who steals secrets through dreams.",
    );
  });

  it("shows play button for movie", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-button")).toBeInTheDocument();
  });

  it("renders genres", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    const genres = screen.getByTestId("media-genres");
    expect(genres).toHaveTextContent("Action");
    expect(genres).toHaveTextContent("Sci-Fi");
  });

  it("shows cast section when cast data present", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    const castSection = screen.getByTestId("cast-section");
    expect(castSection).toHaveTextContent("Leonardo DiCaprio");
  });

  it("does not show play button for series", () => {
    const seriesItem = {
      ...baseItem,
      type: "MEDIA_ITEM_TYPE_SERIES" as const,
    };
    render(<MediaDetail item={seriesItem} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("play-button")).not.toBeInTheDocument();
  });

  it("shows director name", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    const matches = screen.getAllByText("Christopher Nolan");
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("shows year in backdrop area", () => {
    render(<MediaDetail item={baseItem} />, { wrapper: Wrapper });
    expect(screen.getByText("2010")).toBeInTheDocument();
  });
});
