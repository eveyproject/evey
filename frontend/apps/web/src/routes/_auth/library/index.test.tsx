import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const librariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() }));

const mediaItemsHook = vi.fn(() => ({ data: { pagination: { totalItems: 0 } } }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, ...props }: { children: React.ReactNode; to?: string; params?: Record<string, string> }) =>
    React.createElement("a", { href: props.to }, children),
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: () => librariesHook(),
  useLibraryServiceListMediaItems: () => mediaItemsHook(),
  useMusicServiceListTracks: () => ({ data: { total: 0 } }),
  usePhotoServiceListPhotos: () => ({ data: { total: 0 } }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getLibraryIndex() {
  const mod = await import("./index.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("LibraryIndex", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    librariesHook.mockReturnValue({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() });
  });

  it("shows the no-libraries empty state (with an action) when the fetch succeeds empty", async () => {
    const LibraryIndex = await getLibraryIndex();
    render(React.createElement(LibraryIndex), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/no libraries configured/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /go to admin/i })).toBeInTheDocument();
  });

  it("shows a distinct error state with a retry action when the fetch fails - never the empty-libraries message", async () => {
    const refetch = vi.fn();
    librariesHook.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: Object.assign(new Error("boom"), { httpStatus: 500 }),
      refetch,
    });

    const LibraryIndex = await getLibraryIndex();
    render(React.createElement(LibraryIndex), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/no libraries configured/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("shows the offline copy when the failure carries no httpStatus", async () => {
    librariesHook.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new TypeError("Failed to fetch"),
      refetch: vi.fn(),
    });

    const LibraryIndex = await getLibraryIndex();
    render(React.createElement(LibraryIndex), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Offline")).toBeInTheDocument());
  });

  it("renders the library grid when libraries load successfully", async () => {
    librariesHook.mockReturnValue({
      data: { libraries: [{ id: "l1", name: "Movies", type: "LIBRARY_TYPE_MOVIES" }] },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    const LibraryIndex = await getLibraryIndex();
    render(React.createElement(LibraryIndex), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByRole("heading", { name: "Movies", level: 2 })).toBeInTheDocument());
  });

  it("reads each card's count from the library itself and sends no per-card item query", async () => {
    librariesHook.mockReturnValue({
      data: {
        libraries: [
          { id: "l1", name: "Movies", type: "LIBRARY_TYPE_MOVIES", itemCount: 42 },
          { id: "l2", name: "Shows", type: "LIBRARY_TYPE_SHOWS" },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    const LibraryIndex = await getLibraryIndex();
    render(React.createElement(LibraryIndex), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("42 items")).toBeInTheDocument());
    // The absent count of the second card shows nothing, never "0 items".
    expect(screen.queryByText(/^0 items?$/)).not.toBeInTheDocument();
    expect(mediaItemsHook).not.toHaveBeenCalled();
  });
});
