import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ libraries: [] });

    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(PhotosPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No photo libraries/i)).toBeInTheDocument();
    });
  });

  it("renders Photos heading and Albums/All Photos tabs when library exists", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/albums")) return Promise.resolve({ albums: mockAlbums });
      if (url.includes("/api/v1/photos")) return Promise.resolve({ photos: mockPhotos, total: 2 });
      return Promise.resolve({});
    });

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/albums")) return Promise.resolve({ albums: mockAlbums });
      if (url.includes("/api/v1/photos")) return Promise.resolve({ photos: mockPhotos, total: 2 });
      return Promise.resolve({});
    });

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/albums")) return Promise.resolve({ albums: mockAlbums });
      if (url.includes("/api/v1/photos")) return Promise.resolve({ photos: mockPhotos, total: 2 });
      return Promise.resolve({});
    });

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/albums")) return Promise.resolve({ albums: mockAlbums });
      if (url.includes("/api/v1/photos")) return Promise.resolve({ photos: mockPhotos, total: 2 });
      return Promise.resolve({});
    });

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockImplementation(({ url }: { url: string }) => {
      if (url.includes("/api/v1/libraries")) return Promise.resolve({ libraries: mockLibraries });
      if (url.includes("/albums")) return Promise.resolve({ albums: [] });
      if (url.includes("/api/v1/photos")) return Promise.resolve({ photos: [], total: 0 });
      return Promise.resolve({});
    });

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
