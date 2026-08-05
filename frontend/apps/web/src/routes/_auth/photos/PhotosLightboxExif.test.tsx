import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const photoLibsHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false }));
const photoAlbumsHook = vi.fn(() => ({ data: { albums: [] } }));
const photoListHook = vi.fn(() => ({ data: { photos: [], total: 0 } }));
const photoDetailHook = vi.fn(() => ({ data: { cameraModel: "" } }));

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
  usePhotoServiceGetPhoto: (_id: string, opts: { query?: { select?: (d: unknown) => unknown } }) => sel(photoDetailHook(), opts),
  photoServiceDeletePhoto: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockLibraries = [{ id: "lib1", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS" }];
const mockAlbums = [{ Name: "Vacation", LibraryID: "lib1", PhotoCount: 5, CoverPhotoID: "p1" }];
const mockPhotos = [
  {
    ID: "p1", LibraryID: "lib1", Title: "Sunset", FilePath: "/p/1", FileSize: 2_400_000,
    MimeType: "image/jpeg", Width: 4032, Height: 3024, Album: "Vacation",
    TakenAt: "2025-06-01T14:30:00Z", CreatedAt: "2025-01-01T00:00:00Z",
  },
];

async function renderPhotosPage() {
  const mod = await import("./index.lazy");
  const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
  render(React.createElement(PhotosPage), { wrapper: createWrapper() });
}

async function openLightboxOnSunset() {
  await renderPhotosPage();
  await waitFor(() => expect(screen.getByRole("button", { name: "All Photos" })).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "All Photos" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Sunset" })).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "Sunset" }));
}

describe("Photos lightbox EXIF strip", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    photoLibsHook.mockReturnValue({ data: { libraries: mockLibraries }, isLoading: false });
    photoAlbumsHook.mockReturnValue({
      data: { albums: mockAlbums.map((a) => ({ name: a.Name, libraryId: a.LibraryID, photoCount: a.PhotoCount, coverPhotoId: a.CoverPhotoID })) },
    });
    photoListHook.mockReturnValue({
      data: {
        photos: mockPhotos.map((p) => ({
          id: p.ID, libraryId: p.LibraryID, title: p.Title, filePath: p.FilePath,
          fileSize: String(p.FileSize), mimeType: p.MimeType, width: p.Width, height: p.Height,
          album: p.Album, takenAt: p.TakenAt, createdAt: p.CreatedAt,
        })),
        total: 1,
      },
    });
    photoDetailHook.mockReturnValue({ data: { cameraModel: "" } });
  });

  it("shows dimensions and file size from the list data without waiting on the detail fetch", async () => {
    await openLightboxOnSunset();
    expect(await screen.findByText("4032 x 3024")).toBeInTheDocument();
    expect(screen.getByText("2.3 MB")).toBeInTheDocument();
  });

  it("shows the camera model once the detail fetch resolves", async () => {
    photoDetailHook.mockReturnValue({ data: { cameraModel: "Canon EOS R5" } });
    await openLightboxOnSunset();
    expect(await screen.findByText("Canon EOS R5")).toBeInTheDocument();
  });

  it("omits the camera model line when none is reported", async () => {
    photoDetailHook.mockReturnValue({ data: { cameraModel: "" } });
    await openLightboxOnSunset();
    await waitFor(() => expect(screen.getByText("4032 x 3024")).toBeInTheDocument());
    expect(screen.queryByText(/Canon|Nikon|iPhone/)).not.toBeInTheDocument();
  });
});
