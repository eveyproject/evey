import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
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

function makeEveyFetchMock(tracks: typeof mockTracks, total: number) {
  return ({ url }: { url: string }) => {
    if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
    if (url.includes("/tracks")) return Promise.resolve({ tracks, total, limit: 50, offset: 0 });
    if (url.includes("/albums")) return Promise.resolve({ albums: [] });
    if (url.includes("/artists")) return Promise.resolve({ artists: [] });
    return Promise.resolve({});
  };
}

describe("MusicPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows no libraries message when no music library exists", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ libraries: [] });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No music libraries/i)).toBeInTheDocument();
    });
  });

  it("renders Music heading and track list", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(mockTracks, 2));

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(mockTracks, 2));

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());

    const input = screen.getByPlaceholderText(/Search tracks/i);
    fireEvent.change(input, { target: { value: "Blue" } });

    expect(input).toHaveValue("Blue");
  });

  it("pagination Previous is disabled on first page", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(manyTracks, 51));

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "Previous" })).toBeInTheDocument());

    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).not.toBeDisabled();
  });

  it("clicking Next navigates to page 2 and Previous becomes enabled", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(manyTracks, 51));

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/tracks")) return Promise.resolve({ tracks: mockTracks, total: 2, limit: 50, offset: 0 });
      if (url.includes("/albums")) return Promise.resolve({ albums: [{ ID: "a1", Title: "Summer", Artist: "Band A", Year: 2020, CoverArtURL: "", TrackCount: 10, Duration: 3600 }] });
      if (url.includes("/artists")) return Promise.resolve({ artists: [] });
      return Promise.resolve({});
    });

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
