import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockLibrariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
// Tracks are keyset-paginated through useInfiniteQuery, so a test drives the
// FETCHER and lets react-query assemble the pages.
const mockTracksFetch = vi.fn(async (_p?: unknown, _signal?: unknown) => ({ tracks: [] as Record<string, unknown>[], total: 0, nextCursor: undefined as string | undefined }));
const mockAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const mockArtistsHook = vi.fn(() => ({ data: { artists: [] } }));
const mockTracksParams = vi.fn();

function applySelect<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const select = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
}

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockLibrariesHook(), opts),
  musicServiceListTracks: (p?: unknown, signal?: unknown) => { mockTracksParams(p); return mockTracksFetch(p, signal); },
  useMusicServiceGetAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockAlbumsHook(), opts),
  useMusicServiceGetArtists: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockArtistsHook(), opts),
  musicServiceDeleteTrack: vi.fn(),
  usePlaylistServiceListPlaylists: () => ({ data: { playlists: [] }, isLoading: false }),
  usePlaylistServiceCreatePlaylist: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  usePlaylistServiceAddPlaylistItem: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  usePlaylistServiceGetPlaylistItems: () => ({ data: { items: [] } }),
  usePlaylistServiceRemovePlaylistItem: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  usePlaylistServiceDeletePlaylist: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  getPlaylistServiceListPlaylistsQueryKey: () => ["playlists"],
  getPlaylistServiceGetPlaylistItemsQueryKey: () => ["playlist-items"],
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => ({
    ...def,
    useSearch: () => ({}),
  }),
}));

const mockUseAuthStore = vi.fn((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: false }));

const mockMusicStore = {
  playing: null as unknown,
  queue: [] as unknown[],
  shuffle: false,
  externallyPaused: false,
  play: vi.fn(),
  setQueue: vi.fn(),
  toggleShuffle: vi.fn(),
  pauseExternal: vi.fn(),
  resumeExternal: vi.fn(),
  next: vi.fn(),
  prev: vi.fn(),
  close: vi.fn(),
};

const authStoreMock = Object.assign(
  (s: (state: { isAdmin: boolean; accessToken: string | null }) => unknown) => mockUseAuthStore(s),
  { getState: () => ({ isAdmin: false, accessToken: null as string | null }) },
);
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: authStoreMock,
  useMusicPlayerStore: <T,>(selector?: (s: typeof mockMusicStore) => T): T | typeof mockMusicStore =>
    selector ? selector(mockMusicStore) : mockMusicStore,
}));

const mockLibraries = [{ id: "lib1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];
const protoTracks = [{ id: "t1", title: "Blue Sky", artist: "Band A", album: "Summer", albumArtist: "Band A", year: 2020, trackNumber: 1, diskNumber: 1, duration: 180, filePath: "/t/1", mimeType: "audio/flac", genre: "Rock", coverArtUrl: "" }];

function setup(tracks = protoTracks, total = 1) {
  mockLibrariesHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
  mockTracksFetch.mockResolvedValue({ tracks, total, nextCursor: undefined });
  mockAlbumsHook.mockReturnValue({ data: { albums: [] } });
  mockArtistsHook.mockReturnValue({ data: { artists: [] } });
}

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("MusicPage search", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuthStore.mockImplementation((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: false }));
  });

  it("passes search param to tracks hook after typing in search input", async () => {
    setup();
    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/Search tracks/i), { target: { value: "Blue" } });

    await waitFor(() => {
      const paramCalls = mockTracksParams.mock.calls.map((c) => c[0] as { search?: string });
      expect(paramCalls.some((p) => p.search === "Blue")).toBe(true);
    });
  });

  it("shows 'No tracks match your search.' when tracks is empty and search is non-empty", async () => {
    setup([], 0);
    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/Search tracks/i), { target: { value: "xyz" } });

    await waitFor(() => expect(screen.getByText("No tracks match your search.")).toBeInTheDocument());
  });

  it("delete button was removed - never rendered regardless of role", async () => {
    mockUseAuthStore.mockImplementation((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: true }));
    setup();
    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
    await waitFor(() => expect(screen.getByText("Blue Sky")).toBeInTheDocument());
    expect(screen.queryByTitle("Delete track")).not.toBeInTheDocument();
  });
});
