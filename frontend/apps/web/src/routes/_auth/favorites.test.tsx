import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, ...props }: { children: React.ReactNode; to?: string; params?: Record<string, string> }) =>
    React.createElement("a", { href: props.to }, children),
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  usePlaybackServiceMarkAsUnwatched: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: vi.fn((sel: (s: { profileId: string }) => string) => sel({ profileId: "p1" })),
}));

vi.mock("@/hooks/useFavorite", () => ({
  useFavoritesList: vi.fn(() => ({ data: undefined, isLoading: true })),
}));

vi.mock("@/features/library/MediaGrid", () => ({
  MediaGrid: ({ children }: { children: React.ReactNode }) => React.createElement("div", { "data-testid": "media-grid" }, children),
}));

vi.mock("@/features/library/MediaCard", () => ({
  MediaCard: ({ item }: { item: { id: string; title?: string } }) =>
    React.createElement("div", { "data-testid": "media-card" }, item.title ?? item.id),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title, action }: { title: string; action?: { label: string; onClick: () => void } }) =>
    React.createElement(
      "div",
      null,
      title,
      action ? React.createElement("button", { onClick: action.onClick }, action.label) : null,
    ),
}));

vi.mock("@/components/ui/Skeleton", () => ({
  MediaGridSkeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

vi.mock("@/components/BulkActionBar", () => ({
  BulkActionBar: () => null,
}));

vi.mock("@/hooks/useSelection", () => ({
  useSelection: vi.fn(() => ({
    selected: new Set<string>(),
    toggle: vi.fn(),
    selectAll: vi.fn(),
    clear: vi.fn(),
    isSelected: vi.fn(() => false),
    count: 0,
  })),
}));

vi.mock("@/features/playlist/api", () => ({
  usePlaylistList: vi.fn(() => ({ data: [], isLoading: false })),
  useAddToPlaylist: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

vi.mock("@/features/collection/AddToCollectionModal", () => ({
  AddToCollectionModal: () => null,
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? React.createElement("div", { role: "dialog" }, children) : null,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockFavorites = [
  { mediaItemId: "m1", title: "Nova Voyage", type: "MEDIA_ITEM_TYPE_MOVIE", year: 1999, communityRating: 8.7 },
  { mediaItemId: "m2", title: "Ridge Falls", type: "MEDIA_ITEM_TYPE_SERIES", year: 2008, communityRating: 9.5 },
];

describe("FavoritesPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows skeleton while loading", async () => {
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("skeleton")).toBeInTheDocument();
  });

  it("shows empty state when no favorites", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: [], isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    expect(screen.getByText("No favorites yet")).toBeInTheDocument();
  });

  it("shows a distinct error state with a retry action when the fetch fails - never 'No favorites yet'", async () => {
    const refetch = vi.fn();
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("boom"), { httpStatus: 500 }),
      refetch,
    } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    expect(screen.queryByText("No favorites yet")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("renders favorite items in grid", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: mockFavorites as never, isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("media-grid")).toBeInTheDocument();
    expect(screen.getAllByTestId("media-card")).toHaveLength(2);
  });

  it("shows item count when favorites exist", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: mockFavorites as never, isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    expect(screen.getByText("2 items")).toBeInTheDocument();
  });

  it("filters by type", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: mockFavorites as never, isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    const typeSelect = screen.getAllByRole("combobox")[0];
    fireEvent.change(typeSelect, { target: { value: "MEDIA_ITEM_TYPE_MOVIE" } });
    await waitFor(() => expect(screen.getByText("1 item")).toBeInTheDocument());
  });

  it("filters by search query", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: mockFavorites as never, isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    const searchInput = screen.getByRole("searchbox", { name: /search/i });
    fireEvent.change(searchInput, { target: { value: "nova" } });
    await waitFor(() => expect(screen.getByText("1 item")).toBeInTheDocument());
  });

  it("shows no-match message when filter yields zero results", async () => {
    const { useFavoritesList } = await import("@/hooks/useFavorite");
    vi.mocked(useFavoritesList).mockReturnValue({ data: mockFavorites as never, isLoading: false } as never);
    const mod = await import("./favorites.lazy");
    const FavoritesPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(FavoritesPage), { wrapper: createWrapper() });
    const searchInput = screen.getByRole("searchbox", { name: /search/i });
    fireEvent.change(searchInput, { target: { value: "xyznotfound" } });
    await waitFor(() => expect(screen.getByText(/No items match/i)).toBeInTheDocument());
  });
});
