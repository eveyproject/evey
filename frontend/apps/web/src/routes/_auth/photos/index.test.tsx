import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const libsHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const albumsHook = vi.fn(() => ({ data: { albums: [] }, isLoading: false }));
// The page pages photos through useInfiniteQuery, so a test drives the
// FETCHER and lets react-query assemble the pages.
const photosListFetch = vi.fn(async (_params?: unknown, _signal?: unknown) => ({ photos: [] as Record<string, unknown>[], total: 0, nextCursor: undefined as string | undefined }));

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const s = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && s ? s(raw.data) : raw?.data };
}

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

// Spreading importOriginal is required, not cosmetic: the page also imports
// the shared hasMoreFromCursor from this package, and a mock that lists only
// hooks would hand it back undefined.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => sel(libsHook(), opts),
  usePhotoServiceListPhotoAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(albumsHook(), opts),
  photoServiceListPhotos: (params?: unknown, signal?: unknown) => photosListFetch(params, signal),
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
  TriangleAlert: () => React.createElement("span"),
}));
vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title, action }: { title: string; action?: { label: string; onClick: () => void } }) =>
    React.createElement(
      "div",
      null,
      title,
      action ? React.createElement("button", { onClick: action.onClick }, action.label) : null,
    ),
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
    photosListFetch.mockResolvedValue({ photos: [], total: 0, nextCursor: undefined });
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

  it("shows a distinct error state with a retry action when the libraries fetch fails - never 'no photo libraries'", async () => {
    const refetch = vi.fn();
    libsHook.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("boom"), { httpStatus: 500 }),
      refetch,
    });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/no photo libraries/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("shows a distinct error state for the album grid when the albums fetch fails - never 'no albums'", async () => {
    libsHook.mockReturnValue({ data: { libraries: [{ id: "lib1", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }] }, isLoading: false });
    albumsHook.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("boom"), { httpStatus: 500 }),
      refetch: vi.fn(),
    });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/no albums/i)).not.toBeInTheDocument();
  });
});
