import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockLibrariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const mockTracksHook = vi.fn(() => ({ data: { tracks: [], total: 0 } }));
const mockAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const mockArtistsHook = vi.fn(() => ({ data: { artists: [] } }));

function applySelect<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const select = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
}

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockLibrariesHook(), opts),
  useMusicServiceListTracks: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockTracksHook(), opts),
  useMusicServiceGetAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockAlbumsHook(), opts),
  useMusicServiceGetArtists: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockArtistsHook(), opts),
  musicServiceDeleteTrack: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockLibraries = [{ id: "lib1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];

const mockTracks = [
  { ID: "t1", Title: "Blue Sky", Artist: "Band A", Album: "Summer", AlbumArtist: "Band A", Year: 2020, TrackNumber: 1, DiskNumber: 1, Duration: 180, FilePath: "/t/1", MimeType: "audio/flac", Genre: "Rock", CoverArtURL: "" },
  { ID: "t2", Title: "Dark Night", Artist: "Band B", Album: "Winter", AlbumArtist: "Band B", Year: 2021, TrackNumber: 2, DiskNumber: 1, Duration: 240, FilePath: "/t/2", MimeType: "audio/flac", Genre: "Jazz", CoverArtURL: "" },
];

// 51 tracks to trigger pagination (page size = 50)
const manyTracks = Array.from({ length: 51 }, (_, i) => ({
  ID: `t${i}`,
  Title: `Track ${i}`,
  Artist: `Artist ${i}`,
  Album: `Album ${i}`,
  AlbumArtist: `Artist ${i}`,
  Year: 2020,
  TrackNumber: i + 1,
  DiskNumber: 1,
  Duration: 180,
  FilePath: `/t/${i}`,
  MimeType: "audio/flac",
  Genre: "Pop",
  CoverArtURL: "",
}));

function setupMocks(tracks: typeof mockTracks, total: number) {
  mockLibrariesHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
  // convert PascalCase test tracks to camelCase proto
  const protoTracks = tracks.map((t) => ({
    id: t.ID, title: t.Title, artist: t.Artist, album: t.Album, albumArtist: t.AlbumArtist,
    year: t.Year, trackNumber: t.TrackNumber, diskNumber: t.DiskNumber, duration: t.Duration,
    filePath: t.FilePath, mimeType: t.MimeType, genre: t.Genre, coverArtUrl: t.CoverArtURL,
  }));
  mockTracksHook.mockReturnValue({ data: { tracks: protoTracks, total } });
  mockAlbumsHook.mockReturnValue({ data: { albums: [] } });
  mockArtistsHook.mockReturnValue({ data: { artists: [] } });
}

describe("MusicPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows no libraries message when no music library exists", async () => {
    
    mockLibrariesHook.mockReturnValue({ data: { libraries: [] }, isLoading: false });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No music libraries/i)).toBeInTheDocument();
    });
  });

  it("renders Music heading and track list", async () => {
    
    setupMocks(mockTracks, 2);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Music")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText("Blue Sky")).toBeInTheDocument();
      expect(screen.getByText("Dark Night")).toBeInTheDocument();
    });
  });

  it("search input updates value immediately", async () => {
    
    setupMocks(mockTracks, 2);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());

    const input = screen.getByPlaceholderText(/Search tracks/i);
    fireEvent.change(input, { target: { value: "Blue" } });

    expect(input).toHaveValue("Blue");
  });

  it("pagination Previous is disabled on first page", async () => {
    
    setupMocks(manyTracks, 51);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Previous" })).toBeInTheDocument());

    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();
  });

  it("clicking Next navigates to page 2 and Previous becomes enabled", async () => {
    
    setupMocks(manyTracks, 51);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Next" })).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    // After clicking Next, Previous should be enabled (page > 0)
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Previous" })).not.toBeDisabled();
    });
  });

  it("tab navigation switches to Albums tab", async () => {
    
    setupMocks(mockTracks, 2);
    mockAlbumsHook.mockReturnValue({ data: { albums: [{ id: "a1", title: "Summer", artist: "Band A", year: 2020, coverArtUrl: "", trackCount: 10, duration: 3600 }] } });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Albums" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Albums" }));

    await waitFor(() => {
      expect(screen.getByText("Summer")).toBeInTheDocument();
    });
  });
});
