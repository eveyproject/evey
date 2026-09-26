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

function applySelect<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const select = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
}

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => applySelect(mockLibrariesHook(), opts),
  musicServiceListTracks: (p?: unknown, signal?: unknown) => mockTracksFetch(p, signal),
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

// @tanstack/react-virtual is mocked globally in test/setup.ts to render every
// item flatly (JSDOM has no layout, so real virtualization would render
// nothing). Covers both Albums (via MediaGrid) and Artists (via ArtistsTab).
// Mirrors features/library/MediaGrid.test.tsx.

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
  mockTracksFetch.mockResolvedValue({ tracks: protoTracks, total, nextCursor: undefined });
  mockAlbumsHook.mockReturnValue({ data: { albums: [] } });
  mockArtistsHook.mockReturnValue({ data: { artists: [] } });
}

describe("MusicPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // MediaGrid defers its virtualized mount until #main-content (AuthLayout's
    // scroll region) exists; JSDOM doesn't render AuthLayout, so add it or the
    // Albums tab shows skeletons instead of the album cards.
    if (!document.getElementById("main-content")) {
      const main = document.createElement("main");
      main.id = "main-content";
      document.body.appendChild(main);
    }
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

  it("shows a distinct error state with a retry action when the libraries fetch fails - never 'No music libraries'", async () => {
    const refetch = vi.fn();
    mockLibrariesHook.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("boom"), { httpStatus: 500 }),
      refetch,
    });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/No music libraries/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("renders Music heading and track list", async () => {
    
    setupMocks(mockTracks, 2);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Music")).toBeInTheDocument();
    });
    // Default tab is now Albums; switch to Tracks for these assertions.
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
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

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
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

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Previous" })).toBeInTheDocument());

    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();
  });

  it("clicking Next navigates to page 2 and Previous becomes enabled", async () => {
    
    setupMocks(manyTracks, 51);

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Next" })).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    // After clicking Next, Previous should be enabled (page > 0)
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Previous" })).not.toBeDisabled();
    });
  });

  // Regression guard. The tab used to ask for a flat limit=200 with no cursor,
  // so track 201 of a library was unreachable: the pager reported "1 / 4 - 200
  // tracks" and Next went dead at the end of that slice. Next must now ask the
  // server for the following page.
  it("asks the server for the next page when Next runs past the loaded tracks", async () => {
    mockLibrariesHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    mockAlbumsHook.mockReturnValue({ data: { albums: [] } });
    mockArtistsHook.mockReturnValue({ data: { artists: [] } });
    mockTracksFetch.mockReset();
    // A full first page hands out a cursor; the second one doesn't, which is
    // how the server says "that was the last".
    mockTracksFetch
      .mockResolvedValueOnce({
        tracks: [{ id: "t1", title: "Some Track A", artist: "Band A", album: "Some Album", albumArtist: "Band A", year: 2020, trackNumber: 1, diskNumber: 1, duration: 180, filePath: "/t/1", mimeType: "audio/flac", genre: "Rock", coverArtUrl: "" }],
        total: 2,
        nextCursor: "cur-1",
      })
      .mockResolvedValueOnce({
        tracks: [{ id: "t2", title: "Some Track B", artist: "Band B", album: "Some Album", albumArtist: "Band B", year: 2021, trackNumber: 2, diskNumber: 1, duration: 200, filePath: "/t/2", mimeType: "audio/flac", genre: "Rock", coverArtUrl: "" }],
        total: 0,
        nextCursor: undefined,
      });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Tracks" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Tracks" }));

    // One loaded track is far short of a client page, so the pager only exists
    // at all because the server said there is more.
    await waitFor(() => expect(screen.getByRole("button", { name: "Next" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => expect(mockTracksFetch).toHaveBeenCalledTimes(2));
    const secondCall = mockTracksFetch.mock.calls[1][0] as { cursor?: string; offset?: number };
    expect(secondCall.cursor).toBe("cur-1");
    expect(secondCall.offset).toBeUndefined();

    // The second page's tracks are appended, not swapped in.
    await waitFor(() => expect(screen.getByText("Some Track B")).toBeInTheDocument());
    expect(screen.getByText("Some Track A")).toBeInTheDocument();
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

  // Régression : MediaGrid pose `justify-items-start` parce qu'il est fait pour
  // MediaCard, dont la largeur est FIXE (w-40 sm:w-48). Un enfant sans largeur
  // se dimensionne alors sur son CONTENU, donc sur la longueur du titre - et comme
  // la pochette est en aspect-square, un titre long donnait une vignette géante et
  // un titre court une vignette minuscule, qui se replaçait n'importe où dans la
  // grille. Les cartes d'album doivent déclarer la même largeur que MediaCard.
  it("gives every album card a fixed width so the title length cannot drive the cover size", async () => {
    setupMocks(mockTracks, 2);
    mockAlbumsHook.mockReturnValue({ data: { albums: [
      { id: "a1", title: "Even in the Quietest Moments and Then Some More Words", artist: "Skyrider", year: 1977, coverArtUrl: "", trackCount: 8, duration: 2400 },
      { id: "a2", title: "From Nowhere", artist: "Neon Harbor", year: 2024, coverArtUrl: "", trackCount: 11, duration: 1900 },
    ] } });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Albums" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Albums" }));

    await waitFor(() => expect(screen.getByText("From Nowhere")).toBeInTheDocument());

    const cards = document.querySelectorAll(".evey-album-card");
    expect(cards).toHaveLength(2);
    // Même largeur déclarée pour les deux, indépendamment du titre.
    const widths = [...cards].map((c) =>
      [...c.classList].filter((cl) => /^(w-|sm:w-)/.test(cl)).sort().join(" "),
    );
    expect(widths[0]).not.toBe("");
    expect(widths[0]).toBe(widths[1]);
  });

  it("filters tracks by album id and shows the album name (not the id) in the chip", async () => {
    setupMocks(mockTracks, 2);
    // Album id is opaque ("a1"); the display name is "Summer".
    mockAlbumsHook.mockReturnValue({ data: { albums: [{ id: "a1", title: "Summer", artist: "Band A", year: 2020, coverArtUrl: "", trackCount: 10, duration: 3600 }] } });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Albums" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Albums" }));

    const card = await screen.findByRole("button", { name: /Summer/ });
    fireEvent.click(card);

    // The album filter chip must show the human label, never the opaque id.
    await waitFor(() => {
      const chip = screen.getByTitle("Clear");
      expect(chip.textContent).toContain("Summer");
      expect(chip.textContent).not.toContain("a1");
    });
  });
});
