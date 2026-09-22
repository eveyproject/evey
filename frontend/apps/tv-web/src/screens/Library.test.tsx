import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const infinite = vi.fn();
const searchMedia = vi.fn();
const genres = vi.fn();
const library = vi.fn();
const fetchNextPage = vi.fn();

vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: ({ onFocus }: any = {}) => ({
    ref: { current: null },
    focused: false,
    focusSelf: () => onFocus?.({ node: { scrollIntoView: () => {} } }),
  }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: vi.fn(), replace: vi.fn() }));
const authState = { profileId: "prf-1", accessToken: "t" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useInfiniteQuery: (opts: any) => infinite(opts),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetLibrary: () => library(),
  useLibraryServiceListGenres: () => genres(),
  useLibraryServiceSearchMedia: (...a: unknown[]) => searchMedia(...a),
  libraryServiceListMediaItems: vi.fn(),
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useDebounce: (v: string) => v,
  useWatchProgressMaps: () => ({ watchedMap: new Map(), percentMap: new Map() }),
}));

import { LibraryScreen, shouldFetchNextPage, PREFETCH_FROM_END, PAGE_SIZE } from "./Library";

beforeEach(() => {
  library.mockReturnValue({ data: { id: "lib-1", name: "Films", type: "LIBRARY_TYPE_MOVIES" } });
  genres.mockReturnValue({ data: { genres: ["Action", "Drame"] } });
  searchMedia.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() });
  infinite.mockReturnValue({
    data: { pages: [{ items: [{ id: "a", title: "Alpha" }], pagination: { totalItems: 36 } }] },
    isLoading: false,
    isError: false,
    hasNextPage: true,
    isFetchingNextPage: false,
    fetchNextPage,
    refetch: vi.fn(),
  });
});

describe("the library's paging rule", () => {
  it("asks for the next page only from the last rows, once, and only when one remains", () => {
    expect(shouldFetchNextPage(0, PAGE_SIZE, true, false)).toBe(false);
    expect(shouldFetchNextPage(PAGE_SIZE - PREFETCH_FROM_END, PAGE_SIZE, true, false)).toBe(true);
    expect(shouldFetchNextPage(PAGE_SIZE - 1, PAGE_SIZE, true, true)).toBe(false);
    expect(shouldFetchNextPage(PAGE_SIZE - 1, PAGE_SIZE, false, false)).toBe(false);
  });
});

describe("the library screen", () => {
  it("opens a shows library on its series, not on a mix of seasons and episodes", () => {
    library.mockReturnValue({ data: { id: "lib-2", name: "Séries", type: "LIBRARY_TYPE_SHOWS" } });
    render(<LibraryScreen libraryId="lib-2" />);
    const active = Array.from(document.querySelectorAll(".chips .btn.primary")).map((b) => b.textContent);
    expect(active).toContain("media.library.series");
  });

  it("counts what the server says it holds", () => {
    render(<LibraryScreen libraryId="lib-1" />);
    expect(screen.getByText("media.library.resultCount")).toBeTruthy();
  });

  it("turns the grid into one ranked page while something is typed", () => {
    render(<LibraryScreen libraryId="lib-1" />);
    fireEvent.change(screen.getByPlaceholderText("media.search.searchLibrary"), { target: { value: "alpha" } });
    expect(searchMedia).toHaveBeenLastCalledWith(
      expect.objectContaining({ libraryId: "lib-1", query: "alpha" }),
      expect.objectContaining({ query: expect.objectContaining({ enabled: true }) }),
    );
  });

  it("shows the filtered wording rather than the empty-library one when a facet is on", () => {
    infinite.mockReturnValue({
      data: { pages: [{ items: [], pagination: { totalItems: 0 } }] },
      isLoading: false, isError: false, hasNextPage: false, isFetchingNextPage: false, fetchNextPage, refetch: vi.fn(),
    });
    render(<LibraryScreen libraryId="lib-1" />);
    expect(screen.getByText("media.library.noItemsEmptyUser")).toBeTruthy();
    fireEvent.click(screen.getByText("Action"));
    expect(screen.getByText("media.library.noItemsFiltered")).toBeTruthy();
  });
});
