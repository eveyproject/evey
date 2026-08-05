import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRef } from "react";
import { SearchDropdown } from "./SearchDropdown";

const mockNavigate = vi.fn();
vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

const mockSearchResults = {
  items: [
    { id: "1", title: "Inception", year: 2010, type: "MEDIA_ITEM_TYPE_MOVIE", genres: ["Sci-Fi"] },
    { id: "2", title: "Breaking Bad", year: 2008, type: "MEDIA_ITEM_TYPE_SERIES", genres: ["Drama"] },
  ],
  totalHits: 2,
};

let searchMockReturn: { data: typeof mockSearchResults | undefined; isLoading: boolean } = {
  data: mockSearchResults,
  isLoading: false,
};

let musicMockReturn: { data: { tracks: Array<Record<string, unknown>> } | undefined; isLoading: boolean } = {
  data: { tracks: [] },
  isLoading: false,
};

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  libraryServiceSearchMedia: vi.fn(),
  musicServiceSearchMusic: vi.fn(),
}));

vi.mock("@tanstack/react-query", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    // Route by queryKey so the media and music queries can return distinct data.
    useQuery: (opts: { queryKey?: unknown[] }) => {
      const key = opts?.queryKey?.[0];
      if (typeof key === "string" && key.includes("/music/")) return musicMockReturn;
      return searchMockReturn;
    },
  };
});

function createTestQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function renderDropdown(props: Partial<Parameters<typeof SearchDropdown>[0]> = {}) {
  const inputRef = props.inputRef ?? createRef<HTMLInputElement>();
  const defaultProps = {
    query: "inception",
    visible: true,
    onClose: vi.fn(),
    onNavigateToSearch: vi.fn(),
    inputRef,
    kind: "media" as const,
    onKindChange: vi.fn(),
    ...props,
  };
  const qc = createTestQueryClient();
  return {
    ...render(
      <QueryClientProvider client={qc}>
        <SearchDropdown {...defaultProps} />
      </QueryClientProvider>,
    ),
    onClose: defaultProps.onClose,
    onNavigateToSearch: defaultProps.onNavigateToSearch,
  };
}

describe("SearchDropdown", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    searchMockReturn = { data: mockSearchResults, isLoading: false };
    musicMockReturn = { data: { tracks: [] }, isLoading: false };
  });

  it("renders results with title, year, type badge", () => {
    renderDropdown();
    expect(screen.getByText("Inception")).toBeInTheDocument();
    expect(screen.getByText("2010")).toBeInTheDocument();
    expect(screen.getByText("Film")).toBeInTheDocument();
    expect(screen.getByText("Breaking Bad")).toBeInTheDocument();
    expect(screen.getByText("2008")).toBeInTheDocument();
    expect(screen.getByText("Serie")).toBeInTheDocument();
  });

  it("shows no results message when list is empty", () => {
    searchMockReturn = { data: { items: [], totalHits: 0 }, isLoading: false };
    renderDropdown();
    expect(screen.getByTestId("search-dropdown-empty")).toBeInTheDocument();
  });

  it("navigates on click", async () => {
    const user = userEvent.setup();
    const { onClose } = renderDropdown();
    await user.click(screen.getByText("Inception"));
    expect(onClose).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: "/media/$mediaId", params: { mediaId: "1" } }),
    );
  });

  it("defaults to the media scope and hides music results", () => {
    musicMockReturn = {
      data: { tracks: [{ id: "t1", title: "The Emptiness Machine", artist: "Linkin Park", album: "From Zero", albumId: "x", libraryId: "lib-1" }] },
      isLoading: false,
    };
    renderDropdown(); // default kind = media
    expect(screen.getByText("Inception")).toBeInTheDocument();
    expect(screen.queryByTestId("search-dropdown-music")).not.toBeInTheDocument();
  });

  it("in the music scope, renders tracks and navigates to the album on click", async () => {
    musicMockReturn = {
      data: { tracks: [{ id: "t1", title: "The Emptiness Machine", artist: "Linkin Park", album: "From Zero", albumId: "914750d5b8f8bb25", libraryId: "lib-1" }] },
      isLoading: false,
    };
    const user = userEvent.setup();
    const { onClose } = renderDropdown({ kind: "music" });

    expect(screen.getByTestId("search-dropdown-music")).toBeInTheDocument();
    expect(screen.queryByText("Inception")).not.toBeInTheDocument(); // media hidden
    await user.click(screen.getByText("The Emptiness Machine"));
    expect(onClose).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: "/music",
        search: expect.objectContaining({ tab: "tracks", album: "914750d5b8f8bb25", libraryId: "lib-1" }),
      }),
    );
  });

  it("clicking the Music scope tab calls onKindChange", async () => {
    const user = userEvent.setup();
    const onKindChange = vi.fn();
    renderDropdown({ onKindChange });
    await user.click(screen.getByRole("tab", { name: /Music|Musique/ }));
    expect(onKindChange).toHaveBeenCalledWith("music");
  });

  it("keyboard ArrowDown moves focus to first item", async () => {
    const user = userEvent.setup();
    renderDropdown();
    await user.keyboard("{ArrowDown}");
    const items = screen.getAllByRole("button").filter((b) => b.hasAttribute("data-search-item"));
    expect(document.activeElement).toBe(items[0]);
  });

  it("keyboard ArrowDown then ArrowDown moves to second item", async () => {
    const user = userEvent.setup();
    renderDropdown();
    await user.keyboard("{ArrowDown}{ArrowDown}");
    const items = screen.getAllByRole("button").filter((b) => b.hasAttribute("data-search-item"));
    expect(document.activeElement).toBe(items[1]);
  });

  it("keyboard ArrowUp from second moves back to first", async () => {
    const user = userEvent.setup();
    renderDropdown();
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowUp}");
    const items = screen.getAllByRole("button").filter((b) => b.hasAttribute("data-search-item"));
    expect(document.activeElement).toBe(items[0]);
  });

  it("does not render when visible=false", () => {
    renderDropdown({ visible: false });
    expect(screen.queryByTestId("search-dropdown")).not.toBeInTheDocument();
  });

  it("does not render when query is too short", () => {
    renderDropdown({ query: "a" });
    expect(screen.queryByTestId("search-dropdown")).not.toBeInTheDocument();
  });

  it("shows 'View all' when totalHits > items.length", () => {
    searchMockReturn = {
      data: { items: mockSearchResults.items, totalHits: 20 },
      isLoading: false,
    };
    renderDropdown();
    expect(screen.getByText("View all 20 results")).toBeInTheDocument();
  });

  it("calls onNavigateToSearch on 'View all' click", async () => {
    const user = userEvent.setup();
    searchMockReturn = {
      data: { items: mockSearchResults.items, totalHits: 20 },
      isLoading: false,
    };
    const { onClose, onNavigateToSearch } = renderDropdown();
    await user.click(screen.getByText("View all 20 results"));
    expect(onClose).toHaveBeenCalled();
    expect(onNavigateToSearch).toHaveBeenCalledWith("inception");
  });
});
