import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WatchHistory } from "./WatchHistory";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

const unwatch = vi.fn();
const clearAll = vi.fn();

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// Rows a test appends to the fixed fixture below.
const extraItems = vi.hoisted(() => ({ list: [] as Record<string, unknown>[] }));

// importOriginal, not a bare factory: watchHistoryLabel is a PURE formatter,
// not a network hook, so the component must exercise the real one. Stubbing it
// out would let a label regression pass this suite.
const fixture = () => ({
  id: "wp1",
  mediaItemId: "m1",
  positionMs: "3600000",
  durationMs: "7200000",
  percentComplete: 50,
  played: false,
  playCount: 1,
  lastPlayedAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
});

// Cursor -> page. The first page is the fixture plus a test's extra rows.
const pages = vi.hoisted(() => ({ byCursor: {} as Record<string, { items: Record<string, unknown>[]; pagination?: { nextCursor?: string } }> }));
const getHistory = vi.fn(async (params?: Record<string, unknown>) => {
  const cursor = (params?.["pagination.cursor"] as string | undefined) ?? "";
  return pages.byCursor[cursor] ?? { items: cursor ? [] : [fixture(), ...extraItems.list] };
});

// importOriginal, not a bare factory: watchHistoryLabel is a PURE formatter,
// not a network hook, so the component must exercise the real one. Stubbing it
// out would let a label regression pass this suite.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  playbackServiceGetWatchHistory: (params?: Record<string, unknown>) => getHistory(params),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: unwatch, isPending: false }),
  usePlaybackServiceClearWatchHistory: () => ({ mutate: clearAll, isPending: false }),
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["watch-history"],
}));

// The sentinel is driven by hand: jsdom has no IntersectionObserver.
const sentinel = vi.hoisted(() => ({ fetchNext: undefined as undefined | (() => void), hasNext: false }));
vi.mock("@/hooks/useInfiniteSentinel", () => ({
  useInfiniteSentinel: (hasNext: boolean, _fetching: boolean, fetchNext: () => void) => {
    sentinel.hasNext = hasNext;
    sentinel.fetchNext = fetchNext;
    return { current: null };
  },
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("WatchHistory", () => {
  it("renders history items", async () => {
    render(<WatchHistory />, { wrapper: createWrapper() });
    expect(await screen.findByTestId("history-list")).toBeInTheDocument();
    expect(screen.getByTestId("history-item")).toBeInTheDocument();
  });

  it("calls unwatch.mutate on Remove click", async () => {
    unwatch.mockClear();
    render(<WatchHistory />, { wrapper: createWrapper() });
    await userEvent.click(await screen.findByRole("button", { name: "Remove from history" }));
    expect(unwatch).toHaveBeenCalledWith({ mediaItemId: "m1" });
  });

  it("shows Clear history button when showClearButton=true", async () => {
    render(<WatchHistory showClearButton />, { wrapper: createWrapper() });
    expect(await screen.findByTestId("clear-history-btn")).toBeInTheDocument();
  });

  it("does not show Clear history button by default", async () => {
    render(<WatchHistory />, { wrapper: createWrapper() });
    await screen.findByTestId("history-list");
    expect(screen.queryByTestId("clear-history-btn")).not.toBeInTheDocument();
  });

  it("shows a finished title as watched, without a progress bar", async () => {
    extraItems.list = [{ ...fixture(), id: "wp2", mediaItemId: "m2", percentComplete: 97, played: true }];
    render(<WatchHistory />, { wrapper: createWrapper() });
    const rows = await screen.findAllByTestId("history-item");
    expect(within(rows[0]).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "50");
    expect(within(rows[1]).queryByRole("progressbar")).not.toBeInTheDocument();
    expect(rows[1].textContent).toContain("Watched");
    extraItems.list = [];
  });
});

describe("WatchHistory paging", () => {
  // The page asked for the first 50 entries and stopped: a day with more than
  // that hid every older entry, with nothing saying they existed.
  it("loads the next page from the server cursor when the end of the list is reached", async () => {
    const day = (n: number) => new Date(Date.now() - n * 24 * 3600 * 1000).toISOString();
    pages.byCursor = {
      "": { items: [{ ...fixture(), id: "a", mediaItemId: "ma", lastPlayedAt: day(0) }], pagination: { nextCursor: "c1" } },
      c1: { items: [{ ...fixture(), id: "b", mediaItemId: "mb", lastPlayedAt: day(3) }], pagination: {} },
    };
    render(<WatchHistory />, { wrapper: createWrapper() });
    expect(await screen.findAllByTestId("history-item")).toHaveLength(1);
    expect(sentinel.hasNext).toBe(true);
    sentinel.fetchNext?.();
    await waitFor(() => expect(screen.getAllByTestId("history-item")).toHaveLength(2));
    expect(getHistory).toHaveBeenLastCalledWith(expect.objectContaining({ "pagination.cursor": "c1" }));
    expect(screen.getByText("This Week")).toBeInTheDocument();
    expect(sentinel.hasNext).toBe(false);
    pages.byCursor = {};
  });
});

describe("WatchHistory zero duration", () => {
  // The gateway serializes an int64 zero as the string "0", which is truthy:
  // the row printed "1h / " with nothing after the slash.
  it("prints no position / duration line when the duration is the proto zero", async () => {
    extraItems.list = [{
      id: "wp2", mediaItemId: "m2", positionMs: "3600000", durationMs: "0", percentComplete: 0,
      played: false, playCount: 1, lastPlayedAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
    }];
    render(<WatchHistory />, { wrapper: createWrapper() });
    const rows = await screen.findAllByTestId("history-item");
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("1h / 2h");
    expect(rows[1].textContent).not.toContain(" / ");
    extraItems.list = [];
  });
});

describe("WatchHistory empty", () => {
  it("renders the empty state when the history has no entry", async () => {
    pages.byCursor = { "": { items: [] } };
    render(<WatchHistory showClearButton />, { wrapper: createWrapper() });
    expect(await screen.findByTestId("history-empty")).toBeInTheDocument();
    expect(screen.queryByTestId("clear-history-btn")).not.toBeInTheDocument();
    pages.byCursor = {};
  });
});
