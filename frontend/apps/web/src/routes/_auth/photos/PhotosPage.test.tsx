import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const photoLibsHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const photoAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const photoListHook = vi.fn(() => ({ data: { photos: [], total: 0 } }));

function sel<T>(raw: { data?: T; [k: string]: unknown }, opts: { query?: { select?: (d: T) => unknown } }) {
  const s = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && s ? s(raw.data) : raw?.data };
}

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useLibraryServiceListLibraries: (opts: { query?: { select?: (d: unknown) => unknown } }) => sel(photoLibsHook(), opts),
  usePhotoServiceListPhotoAlbums: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(photoAlbumsHook(), opts),
  usePhotoServiceListPhotos: (_p: unknown, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(photoListHook(), opts),
  usePhotoServiceGetPhoto: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => sel({ data: { cameraModel: "" } }, opts),
  photoServiceDeletePhoto: vi.fn(),
}));

// Stub router so createLazyFileRoute doesn't fail
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockLibraries = [{ id: "lib1", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }];
const mockPhotos = [
  { ID: "p1", LibraryID: "lib1", Title: "Sunset", FilePath: "/p/1", FileSize: 100, MimeType: "image/jpeg", Width: 1920, Height: 1080, Album: "Vacation", CreatedAt: "2025-01-01T00:00:00Z" },
  { ID: "p2", LibraryID: "lib1", Title: "Mountain", FilePath: "/p/2", FileSize: 200, MimeType: "image/jpeg", Width: 1920, Height: 1080, Album: "Hiking", CreatedAt: "2025-01-02T00:00:00Z" },
];
const mockAlbums = [
  { Name: "Vacation", LibraryID: "lib1", PhotoCount: 5, CoverPhotoID: "p1" },
];

describe("PhotosPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders empty state when no photo libraries", async () => {
    photoLibsHook.mockReturnValue({ data: { libraries: [] }, isLoading: false });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No photo libraries/i)).toBeInTheDocument();
    });
  });

  it("renders Photos heading and Albums/All Photos tabs when library exists", async () => {
    
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({ data: { albums: mockAlbums.map((a) => ({ name: a.Name, libraryId: a.LibraryID, photoCount: a.PhotoCount, coverPhotoId: a.CoverPhotoID })) } });
    photoListHook.mockReturnValue({ data: { photos: mockPhotos.map((p) => ({ id: p.ID, libraryId: p.LibraryID, title: p.Title, filePath: p.FilePath, fileSize: String(p.FileSize), mimeType: p.MimeType, width: p.Width, height: p.Height, album: p.Album, createdAt: p.CreatedAt })), total: 2 } });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Photos")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Albums" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument();
  });

  it("clicking All Photos shows photo grid", async () => {
    
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({ data: { albums: mockAlbums.map((a) => ({ name: a.Name, libraryId: a.LibraryID, photoCount: a.PhotoCount, coverPhotoId: a.CoverPhotoID })) } });
    photoListHook.mockReturnValue({ data: { photos: mockPhotos.map((p) => ({ id: p.ID, libraryId: p.LibraryID, title: p.Title, filePath: p.FilePath, fileSize: String(p.FileSize), mimeType: p.MimeType, width: p.Width, height: p.Height, album: p.Album, createdAt: p.CreatedAt })), total: 2 } });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "All Photos" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Sunset" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Mountain" })).toBeInTheDocument();
    });
  });

  it("clicking a photo opens lightbox overlay", async () => {
    
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({ data: { albums: mockAlbums.map((a) => ({ name: a.Name, libraryId: a.LibraryID, photoCount: a.PhotoCount, coverPhotoId: a.CoverPhotoID })) } });
    photoListHook.mockReturnValue({ data: { photos: mockPhotos.map((p) => ({ id: p.ID, libraryId: p.LibraryID, title: p.Title, filePath: p.FilePath, fileSize: String(p.FileSize), mimeType: p.MimeType, width: p.Width, height: p.Height, album: p.Album, createdAt: p.CreatedAt })), total: 2 } });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "All Photos" }));

    await waitFor(() => expect(screen.getByRole("button", { name: "Sunset" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Sunset" }));

    // Lightbox shows close button and photo title
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
    });
    expect(screen.getAllByText("Sunset").length).toBeGreaterThan(0);
  });

  it("lightbox close button dismisses overlay", async () => {
    
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({ data: { albums: mockAlbums.map((a) => ({ name: a.Name, libraryId: a.LibraryID, photoCount: a.PhotoCount, coverPhotoId: a.CoverPhotoID })) } });
    photoListHook.mockReturnValue({ data: { photos: mockPhotos.map((p) => ({ id: p.ID, libraryId: p.LibraryID, title: p.Title, filePath: p.FilePath, fileSize: String(p.FileSize), mimeType: p.MimeType, width: p.Width, height: p.Height, album: p.Album, createdAt: p.CreatedAt })), total: 2 } });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "All Photos" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Sunset" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Sunset" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Close" })).not.toBeInTheDocument();
    });
  });

  it("shows empty photo grid message when photos array is empty", async () => {
    
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({ data: { albums: [] } });
    photoListHook.mockReturnValue({ data: { photos: [], total: 0 } });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "All Photos" }));

    await waitFor(() => {
      expect(screen.getByText("No photos found in this library.")).toBeInTheDocument();
    });
  });
});
