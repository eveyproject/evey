import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockListLibraries = vi.fn();
const mockDeleteLibrary = vi.fn();
const mockScanLibrary = vi.fn();
const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: (...args: unknown[]) => mockListLibraries(...args),
  useLibraryServiceDeleteLibrary: (...args: unknown[]) => mockDeleteLibrary(...args),
  useLibraryServiceScanLibrary: (...args: unknown[]) => mockScanLibrary(...args),
  libraryServiceListMediaItems: vi.fn().mockResolvedValue({ pagination: { totalItems: 0 } }),
  eveyFetch: vi.fn().mockResolvedValue({ stats: [], schedule: null }),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@/features/admin/LibraryForm", () => ({
  LibraryForm: ({ onSuccess }: { onSuccess: () => void }) =>
    React.createElement("div", { "data-testid": "library-form" },
      React.createElement("button", { onClick: onSuccess }, "Submit")
    ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getLibrariesPage() {
  const mod = await import("./libraries.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockLibrary = {
  id: "lib-1",
  name: "My Movies",
  type: "LIBRARY_TYPE_MOVIES",
  paths: ["/media/movies"],
};

describe("LibrariesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseQuery.mockImplementation((opts: { queryKey: unknown[] }) => {
      const key = opts?.queryKey?.[0];
      if (key === "/api/v1/admin/library-stats") return { data: [], isLoading: false };
      if (key === "/api/v1/admin/library-stats" || String(key).includes("schedule")) return { data: null, isLoading: false };
      // media items query
      return { data: { pagination: { totalItems: 5 } }, isLoading: false };
    });
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mockDeleteLibrary.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mockScanLibrary.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders page heading", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getByText("Libraries")).toBeInTheDocument();
  });

  it("shows empty message when no libraries", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getByText(/No libraries/i)).toBeInTheDocument();
  });

  it("renders library row with name and type", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [mockLibrary] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getByText("My Movies")).toBeInTheDocument();
    expect(screen.getByText("Movies")).toBeInTheDocument();
  });

  it("clicking Add Library opens modal", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Add Library/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByTestId("library-form")).toBeInTheDocument();
  });

  it("clicking Delete shows confirmation modal", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [mockLibrary] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Delete$/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  // No cron schedule does not mean no scan: the server walks every library on
  // its own periodic pass. The column said "Off", which read as never scanned.
  it("shows an unscheduled library as scanned automatically, not off", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [mockLibrary] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getByText("Automatic")).toBeInTheDocument();
    expect(screen.queryByText("Off")).not.toBeInTheDocument();
  });

  it("shows scan and schedule buttons for library row", async () => {
    mockListLibraries.mockReturnValue({ data: { libraries: [mockLibrary] }, isLoading: false });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /^Scan$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Schedule/i })).toBeInTheDocument();
  });

  it("shows provider match buttons only for movie and show libraries", async () => {
    mockListLibraries.mockReturnValue({
      data: {
        libraries: [
          mockLibrary,
          { id: "lib-2", name: "My Shows", type: "LIBRARY_TYPE_SHOWS", paths: ["/media/shows"] },
        ],
      },
      isLoading: false,
    });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.getAllByTestId("refresh-library-metadata-btn")).toHaveLength(2);
    expect(screen.getAllByTestId("match-orphans-btn")).toHaveLength(2);
  });

  it("hides provider match buttons for music and photo libraries", async () => {
    mockListLibraries.mockReturnValue({
      data: {
        libraries: [
          { id: "lib-3", name: "My Music", type: "LIBRARY_TYPE_MUSIC", paths: ["/media/music"] },
          { id: "lib-4", name: "My Photos", type: "LIBRARY_TYPE_PHOTOS", paths: ["/media/photos"] },
        ],
      },
      isLoading: false,
    });
    const LibrariesPage = await getLibrariesPage();
    render(React.createElement(LibrariesPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("refresh-library-metadata-btn")).not.toBeInTheDocument();
    expect(screen.queryByTestId("match-orphans-btn")).not.toBeInTheDocument();
    // The rest of the row still works for these types.
    expect(screen.getAllByRole("button", { name: /^Scan$/i })).toHaveLength(2);
  });
});
