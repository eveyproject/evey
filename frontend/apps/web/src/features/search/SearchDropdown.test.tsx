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

vi.mock("@evey/api", () => ({
  libraryServiceSearchMedia: vi.fn(),
}));

vi.mock("@tanstack/react-query", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: () => searchMockReturn,
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
