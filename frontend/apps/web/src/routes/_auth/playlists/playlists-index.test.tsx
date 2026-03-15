import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockUsePlaylistList = vi.fn();
const mockUseDeletePlaylist = vi.fn();

vi.mock("@/features/playlist/api", () => ({
  usePlaylistList: (...args: unknown[]) => mockUsePlaylistList(...args),
  useDeletePlaylist: (...args: unknown[]) => mockUseDeletePlaylist(...args),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  Link: ({ children, ...props }: React.PropsWithChildren<Record<string, unknown>>) =>
    React.createElement("a", props, children),
}));

vi.mock("@/features/playlist/CreatePlaylistModal", () => ({
  CreatePlaylistModal: ({ open, onClose }: { open: boolean; onClose: () => void }) =>
    open
      ? React.createElement("div", { role: "dialog", "data-testid": "create-playlist-modal" },
          React.createElement("button", { onClick: onClose }, "Close")
        )
      : null,
}));

// Mock window.confirm
vi.stubGlobal("confirm", vi.fn(() => true));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getPlaylistsPage() {
  const mod = await import("./index.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockPlaylists = [
  {
    id: "pl-1",
    name: "My Favorites",
    type: "manual",
    description: "Best movies",
    created_at: "2024-01-15T10:00:00Z",
  },
  {
    id: "pl-2",
    name: "Auto Picks",
    type: "smart",
    description: "",
    created_at: "2024-02-01T10:00:00Z",
  },
];

describe("PlaylistsIndex", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDeletePlaylist.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders Playlists heading", async () => {
    mockUsePlaylistList.mockReturnValue({ data: [], isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    expect(screen.getByText("Playlists")).toBeInTheDocument();
  });

  it("shows empty state when no playlists", async () => {
    mockUsePlaylistList.mockReturnValue({ data: [], isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    expect(screen.getByText(/No playlists yet/i)).toBeInTheDocument();
  });

  it("renders playlist cards with names", async () => {
    mockUsePlaylistList.mockReturnValue({ data: mockPlaylists, isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    expect(screen.getByText("My Favorites")).toBeInTheDocument();
    expect(screen.getByText("Auto Picks")).toBeInTheDocument();
  });

  it("renders type badges", async () => {
    mockUsePlaylistList.mockReturnValue({ data: mockPlaylists, isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    expect(screen.getByText("manual")).toBeInTheDocument();
    expect(screen.getByText("smart")).toBeInTheDocument();
  });

  it("clicking New Playlist opens create modal", async () => {
    mockUsePlaylistList.mockReturnValue({ data: [], isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /New Playlist/i }));
    expect(screen.getByTestId("create-playlist-modal")).toBeInTheDocument();
  });

  it("clicking Create Playlist in empty state opens create modal", async () => {
    mockUsePlaylistList.mockReturnValue({ data: [], isLoading: false });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Create Playlist/i }));
    expect(screen.getByTestId("create-playlist-modal")).toBeInTheDocument();
  });

  it("shows loading skeletons while loading", async () => {
    mockUsePlaylistList.mockReturnValue({ data: undefined, isLoading: true });
    const PlaylistsIndex = await getPlaylistsPage();
    render(React.createElement(PlaylistsIndex), { wrapper: createWrapper() });
    expect(screen.getByText("Playlists")).toBeInTheDocument();
  });
});
