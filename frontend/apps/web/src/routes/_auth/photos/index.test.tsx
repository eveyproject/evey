import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const libsHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const albumsHook = vi.fn(() => ({ data: { albums: [] }, isLoading: false }));
const photosListHook = vi.fn(() => ({ data: { photos: [], total: 0 }, isLoading: false }));

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const s = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && s ? s(raw.data) : raw?.data };
}

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => sel(libsHook(), opts),
  usePhotoServiceListPhotoAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(albumsHook(), opts),
  usePhotoServiceListPhotos: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(photosListHook(), opts),
  usePhotoServiceGetPhoto: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => sel({ data: { cameraModel: "" } }, opts),
  photoServiceDeletePhoto: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: vi.fn(() => false),
}));
vi.mock("lucide-react", () => ({
  Images: () => React.createElement("span", null, "icon"),
  ChevronLeft: () => React.createElement("span"),
  ChevronRight: () => React.createElement("span"),
  X: () => React.createElement("span"),
  Trash2: () => React.createElement("span"),
}));
vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("PhotosPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    libsHook.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    albumsHook.mockReturnValue({ data: { albums: [] }, isLoading: false });
    photosListHook.mockReturnValue({ data: { photos: [], total: 0 }, isLoading: false });
  });

  it("shows loading skeleton when libraries loading", async () => {
    libsHook.mockReturnValue({ data: undefined, isLoading: true });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    expect(document.querySelector(".animate-pulse")).toBeTruthy();
  });

  it("shows empty state when no libraries", async () => {
    libsHook.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/no photo libraries/i)).toBeInTheDocument());
  });

  it("shows albums view by default when library selected", async () => {
    libsHook.mockReturnValue({ data: { libraries: [{ id: "lib1", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }] }, isLoading: false });
    albumsHook.mockReturnValue({ data: { albums: [{ name: "Vacation", libraryId: "lib1", photoCount: 5, coverPhotoId: "" }] }, isLoading: false });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("Vacation")).toBeInTheDocument());
  });
});
