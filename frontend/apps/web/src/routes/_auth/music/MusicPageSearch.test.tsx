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

const mockUseAuthStore = vi.fn((s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: false }));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (s: (state: { isAdmin: boolean }) => unknown) => mockUseAuthStore(s),
}));

const mockLibraries = [{ id: "lib1", name: "My Music", type: "LIBRARY_TYPE_MUSIC" }];

const mockTracks = [
  { ID: "t1", Title: "Blue Sky", Artist: "Band A", Album: "Summer", AlbumArtist: "Band A", Year: 2020, TrackNumber: 1, DiskNumber: 1, Duration: 180, FilePath: "/t/1", MimeType: "audio/flac", Genre: "Rock", CoverArtURL: "" },
];

function makeEveyFetchMock(tracks: typeof mockTracks, total: number) {
  return ({ url }: { url: string }) => {
    if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
    if (url.includes("/tracks")) return Promise.resolve({ tracks, total, limit: 200, offset: 0 });
    if (url.includes("/albums")) return Promise.resolve({ albums: [] });
    if (url.includes("/artists")) return Promise.resolve({ artists: [] });
    return Promise.resolve({});
  };
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

  it("calls tracks API with search param after typing in search input", async () => {
    const { eveyFetch } = await import("@evey/api");
    const mockFetch = vi.mocked(eveyFetch);
    mockFetch.mockImplementation(makeEveyFetchMock(mockTracks, 1));

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());

    const input = screen.getByPlaceholderText(/Search tracks/i);
    fireEvent.change(input, { target: { value: "Blue" } });

    await waitFor(() => {
      const calls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      const searchCall = calls.find((url) => url.includes("search=Blue"));
      expect(searchCall).toBeDefined();
    });
  });

  it("shows 'No tracks match your search.' when tracks is empty and search is non-empty", async () => {
    const { eveyFetch } = await import("@evey/api");
    const mockFetch = vi.mocked(eveyFetch);
    mockFetch.mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/tracks")) return Promise.resolve({ tracks: [], total: 0, limit: 200, offset: 0 });
      if (url.includes("/albums")) return Promise.resolve({ albums: [] });
      if (url.includes("/artists")) return Promise.resolve({ artists: [] });
      return Promise.resolve({});
    });

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByPlaceholderText(/Search tracks/i)).toBeInTheDocument());

    const input = screen.getByPlaceholderText(/Search tracks/i);
    fireEvent.change(input, { target: { value: "xyz" } });

    await waitFor(() => {
      expect(screen.getByText("No tracks match your search.")).toBeInTheDocument();
    });
  });

  it("delete button (Trash2) is not visible when isAdmin is false", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(mockTracks, 1));

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Blue Sky")).toBeInTheDocument());

    expect(screen.queryByTitle("Delete track")).not.toBeInTheDocument();
  });

  it("delete button (Trash2) is visible when isAdmin is true", async () => {
    mockUseAuthStore.mockImplementation(
      (s: (state: { isAdmin: boolean }) => unknown) => s({ isAdmin: true })
    );

    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(makeEveyFetchMock(mockTracks, 1));

    const mod = await import("./index.lazy");
    const MusicPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(MusicPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Blue Sky")).toBeInTheDocument());

    expect(screen.getByTitle("Delete track")).toBeInTheDocument();
  });
});
