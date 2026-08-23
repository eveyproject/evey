import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockUseSearch = vi.fn(() => ({ q: undefined }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  useNavigate: () => mockNavigate,
  // Route.useSearch is accessed via the module-level Route object - we'll stub below
}));

vi.mock("@evey/api", () => ({
  libraryServiceSearchMedia: vi.fn(() => Promise.resolve({ items: [], totalHits: 0 })),
  musicServiceSearchMusic: vi.fn(() => Promise.resolve({ tracks: [] })),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title, action }: { title: string; action?: { label: string; onClick: () => void } }) =>
    React.createElement(
      "div",
      { "data-testid": "empty-state" },
      title,
      action ? React.createElement("button", { onClick: action.onClick }, action.label) : null,
    ),
}));

vi.mock("@/components/ui/Skeleton", () => ({
  MediaGridSkeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

vi.mock("@/features/library/MediaGrid", () => ({
  MediaGrid: ({ children }: { children: React.ReactNode }) => React.createElement("div", { "data-testid": "media-grid" }, children),
}));

vi.mock("@/features/library/MediaCard", () => ({
  MediaCard: ({ item }: { item: { id: string; title?: string } }) =>
    React.createElement("div", { "data-testid": "media-card" }, item.title),
}));

vi.mock("@/hooks/useRecentSearches", () => ({
  useRecentSearches: vi.fn(() => ({
    recents: [],
    push: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
  })),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getSearchPage() {
  const mod = await import("./search.lazy");
  // Patch Route.useSearch on the imported module's Route export
  (mod.Route as unknown as { useSearch: () => { q?: string } }).useSearch = mockUseSearch;
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("SearchPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseSearch.mockReturnValue({ q: undefined });
    vi.useFakeTimers();
  });

  afterEach(() => { vi.useRealTimers(); });

  it("shows search empty state on initial render with no query", async () => {
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("empty-state")).toBeInTheDocument();
    expect(screen.getByText("Search your media library")).toBeInTheDocument();
  });

  it("renders search input", async () => {
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("search-page-input")).toBeInTheDocument();
  });

  it("shows clear button when input has value", async () => {
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input");
    fireEvent.change(input, { target: { value: "matrix" } });
    expect(screen.getByRole("button", { name: /Clear search/i })).toBeInTheDocument();
  });

  it("clear button resets input value", async () => {
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "matrix" } });
    fireEvent.click(screen.getByRole("button", { name: /Clear search/i }));
    expect(input.value).toBe("");
  });

  it("shows type filter buttons after debounce when query > 1 char", async () => {
    vi.useRealTimers();
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input");
    fireEvent.change(input, { target: { value: "ma" } });
    await waitFor(() => expect(screen.getByTestId("search-type-filters")).toBeInTheDocument(), { timeout: 1000 });
    expect(screen.getByRole("button", { name: /all/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /movies/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /series/i })).toBeInTheDocument();
  });

  it("shows no-results empty state when search returns empty", async () => {
    vi.useRealTimers();
    const { libraryServiceSearchMedia } = await import("@evey/api");
    vi.mocked(libraryServiceSearchMedia).mockResolvedValue({ items: [], totalHits: 0 } as never);
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input");
    fireEvent.change(input, { target: { value: "xyzunknown" } });
    await waitFor(() => expect(screen.getByText(/No results for/i)).toBeInTheDocument(), { timeout: 2000 });
  });

  it("shows a distinct error state with a retry action when the media search fails", async () => {
    vi.useRealTimers();
    const { libraryServiceSearchMedia } = await import("@evey/api");
    vi.mocked(libraryServiceSearchMedia).mockRejectedValue(Object.assign(new Error("boom"), { httpStatus: 500 }));
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input");
    fireEvent.change(input, { target: { value: "xyzunknown" } });
    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument(), { timeout: 2000 });
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("shows a distinct error state with a retry action when the music search fails", async () => {
    vi.useRealTimers();
    const { musicServiceSearchMusic } = await import("@evey/api");
    vi.mocked(musicServiceSearchMusic).mockRejectedValue(Object.assign(new Error("boom"), { httpStatus: 500 }));
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    const input = screen.getByTestId("search-page-input");
    fireEvent.change(input, { target: { value: "xyzunknown" } });
    await waitFor(() => expect(screen.getByTestId("search-kind-toggle")).toBeInTheDocument(), { timeout: 2000 });
    // The music-scope label ("Music") isn't in this test's static translation
    // fixture - select the second toggle button by position instead of text.
    const musicToggle = screen.getByTestId("search-kind-toggle").querySelectorAll("button")[1];
    fireEvent.click(musicToggle);
    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument(), { timeout: 2000 });
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("renders recent searches when available", async () => {
    const { useRecentSearches } = await import("@/hooks/useRecentSearches");
    vi.mocked(useRecentSearches).mockReturnValue({
      recents: ["some movie", "some movie 2"],
      push: vi.fn(),
      remove: vi.fn(),
      clear: vi.fn(),
    });
    const SearchPage = await getSearchPage();
    render(React.createElement(SearchPage), { wrapper: createWrapper() });
    expect(screen.getByText("some movie")).toBeInTheDocument();
    expect(screen.getByText("some movie 2")).toBeInTheDocument();
    expect(screen.getByText("Recent searches")).toBeInTheDocument();
  });
});
