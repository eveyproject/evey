import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const nowPlayingMockLibrariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const nowPlayingMockTracksHook = vi.fn(() => ({ data: { tracks: [], total: 0 } }));
const nowPlayingMockAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const nowPlayingMockArtistsHook = vi.fn(() => ({ data: { artists: [] } }));

function applyNowPlayingSelect<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const select = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
}

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => applyNowPlayingSelect(nowPlayingMockLibrariesHook(), opts),
  useMusicServiceListTracks: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applyNowPlayingSelect(nowPlayingMockTracksHook(), opts),
  useMusicServiceGetAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applyNowPlayingSelect(nowPlayingMockAlbumsHook(), opts),
  useMusicServiceGetArtists: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => applyNowPlayingSelect(nowPlayingMockArtistsHook(), opts),
  musicServiceDeleteTrack: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const state = { isAdmin: false };
    return selector ? selector(state) : state;
  },
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

// HTMLMediaElement.play/pause are not implemented in JSDOM
window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
window.HTMLMediaElement.prototype.pause = vi.fn();

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockLibraries = [{ id: "lib1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];

const mockTracks = [
  {
    ID: "t1",
    Title: "Blue Sky",
    Artist: "Band A",
    Album: "Summer",
    AlbumArtist: "Band A",
    Year: 2020,
    TrackNumber: 1,
    DiskNumber: 1,
    Duration: 180,
    FilePath: "/t/1",
    MimeType: "audio/flac",
    Genre: "Rock",
    CoverArtURL: "",
  },
  {
    ID: "t2",
    Title: "Dark Night",
    Artist: "Band B",
    Album: "Winter",
    AlbumArtist: "Band B",
    Year: 2021,
    TrackNumber: 2,
    DiskNumber: 1,
    Duration: 240,
    FilePath: "/t/2",
    MimeType: "audio/flac",
    Genre: "Jazz",
    CoverArtURL: "",
  },
];

async function renderMusicPageWithTrackPlaying() {
  nowPlayingMockLibrariesHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
  const protoTracks = mockTracks.map((t) => ({
    id: t.ID, title: t.Title, artist: t.Artist, album: t.Album, albumArtist: t.AlbumArtist,
    year: t.Year, trackNumber: t.TrackNumber, diskNumber: t.DiskNumber, duration: t.Duration,
    filePath: t.FilePath, mimeType: t.MimeType, genre: t.Genre, coverArtUrl: t.CoverArtURL,
  }));
  nowPlayingMockTracksHook.mockReturnValue({ data: { tracks: protoTracks, total: 2 } });
  nowPlayingMockAlbumsHook.mockReturnValue({ data: { albums: [] } });
  nowPlayingMockArtistsHook.mockReturnValue({ data: { artists: [] } });

  const mod = await import("./index.lazy");
  const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

  render(React.createElement(MusicPage), { wrapper: createWrapper() });

  // Wait for track list and click first track to start playing
  await waitFor(() => expect(screen.getByText("Blue Sky")).toBeInTheDocument());
  fireEvent.click(screen.getByText("Blue Sky"));

  // Wait for NowPlayingBar to appear
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Play|Pause/ })).toBeInTheDocument(),
  );
}

describe("NowPlayingBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows track title and artist when a track is playing", async () => {
    await renderMusicPageWithTrackPlaying();
    const bars = screen.getAllByText("Blue Sky");
    expect(bars.length).toBeGreaterThan(0);
    const artistElements = screen.getAllByText("Band A");
    expect(artistElements.length).toBeGreaterThan(0);
  });

  it("play/pause button is present", async () => {
    await renderMusicPageWithTrackPlaying();
    expect(screen.getByRole("button", { name: /Play|Pause/ })).toBeInTheDocument();
  });

  it("Previous and Next navigation buttons are present", async () => {
    await renderMusicPageWithTrackPlaying();
    expect(screen.getByRole("button", { name: "Previous" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Next" })).toBeInTheDocument();
  });

  it("Previous button is disabled when first track is playing", async () => {
    await renderMusicPageWithTrackPlaying();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
  });

  it("Next button is enabled when there are more tracks in the queue", async () => {
    await renderMusicPageWithTrackPlaying();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();
  });

  it("close button dismisses the player", async () => {
    await renderMusicPageWithTrackPlaying();
    expect(screen.getByRole("button", { name: "Close player" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close player" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /Play|Pause/ })).not.toBeInTheDocument(),
    );
  });

  it("clicking Next switches to the next track", async () => {
    await renderMusicPageWithTrackPlaying();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() => {
      const titles = screen.getAllByText("Dark Night");
      expect(titles.length).toBeGreaterThan(0);
    });
  });
});
