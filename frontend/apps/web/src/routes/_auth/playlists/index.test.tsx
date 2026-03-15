import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, to }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: to }, children),
}));

vi.mock("@/features/playlist/api", () => ({
  usePlaylistList: vi.fn(() => ({ data: undefined, isLoading: true })),
  useDeletePlaylist: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}));
vi.mock("@/features/playlist/CreatePlaylistModal", () => ({
  CreatePlaylistModal: () => null,
}));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/components/ui/Badge", () => ({
  Badge: ({ children }: { children: React.ReactNode }) => React.createElement("span", null, children),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) =>
    React.createElement("button", { onClick }, children),
}));
vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));
vi.mock("@/components/ui/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("PlaylistsIndex", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows skeleton cards while loading", async () => {
    const mod = await import("./index.lazy");
    const PlaylistsIndex = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0));
  });

  it("shows empty state when no playlists", async () => {
    const api = await import("@/features/playlist/api");
    vi.mocked(api.usePlaylistList).mockReturnValue({ data: [], isLoading: false } as never);
    const mod = await import("./index.lazy");
    const PlaylistsIndex = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/No playlists yet/i)).toBeInTheDocument());
  });

  it("renders playlist cards when data is available", async () => {
    const api = await import("@/features/playlist/api");
    vi.mocked(api.usePlaylistList).mockReturnValue({
      data: [
        { id: "p1", name: "My Favorites", type: "manual", sort_by: "added_at", sort_order: "asc", description: "", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
        { id: "p2", name: "Watch Later", type: "manual", sort_by: "added_at", sort_order: "asc", description: "", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
      ],
      isLoading: false,
    } as never);
    const mod = await import("./index.lazy");
    const PlaylistsIndex = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("My Favorites")).toBeInTheDocument());
    expect(screen.getByText("Watch Later")).toBeInTheDocument();
  });

  it("renders 'New playlist' button", async () => {
    const api = await import("@/features/playlist/api");
    vi.mocked(api.usePlaylistList).mockReturnValue({ data: [], isLoading: false } as never);
    const mod = await import("./index.lazy");
    const PlaylistsIndex = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /new playlist/i })).toBeInTheDocument());
  });
});
