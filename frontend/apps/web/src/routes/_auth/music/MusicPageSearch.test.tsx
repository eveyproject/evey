import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockLibrariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const mockTracksHook = vi.fn(() => ({ data: { tracks: [], total: 0 } }));
const mockAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const mockArtistsHook = vi.fn(() => ({ data: { artists: [] } }));
const mockTracksParams = vi.fn();

function applySelect<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const select = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
}

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockLibrariesHook(), opts),
  useMusicServiceListTracks: (p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => { mockTracksParams(p); return applySelect(mockTracksHook(), opts); },
  useMusicServiceGetAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockAlbumsHook(), opts),
  useMusicServiceGetArtists: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockArtistsHook(), opts),
  musicServiceDeleteTrack: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

const mockUseAuthStore = vi.fn((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: false }));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (s: (state: { isAdmin: boolean }) => unknown) => mockUseAuthStore(s),
}));

const mockLibraries = [{ id: "lib1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];
const protoTracks = [{ id: "t1", title: "Blue Sky", artist: "Band A", album: "Summer", albumArtist: "Band A", year: 2020, trackNumber: 1, diskNumber: 1, duration: 180, filePath: "/t/1", mimeType: "audio/flac", genre: "Rock", coverArtUrl: "" }];

function setup(tracks = protoTracks, total = 1) {
  mockLibrariesHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
  mockTracksHook.mockReturnValue({ data: { tracks, total } });
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

    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());
    fireEvent.change(screen.getByPlaceholderText(/Search tracks/i), { target: { value: "xyz" } });

    await waitFor(() => expect(screen.getByText("No tracks match your search.")).toBeInTheDocument());
  });

  it("delete button was removed — never rendered regardless of role", async () => {
    mockUseAuthStore.mockImplementation((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: true }));
    setup();
    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Blue Sky")).toBeInTheDocument());
    expect(screen.queryByTitle("Delete track")).not.toBeInTheDocument();
  });
});
