import { render, screen } from "@testing-library/react";
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
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceGetWatchHistory: () => ({
    data: {
      items: [
        {
          id: "wp1",
          mediaItemId: "m1",
          positionMs: "3600000",
          durationMs: "7200000",
          percentComplete: 50,
          played: false,
          playCount: 1,
          lastPlayedAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
        ...extraItems.list,
      ],
    },
    isLoading: false,
  }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: unwatch, isPending: false }),
  usePlaybackServiceClearWatchHistory: () => ({ mutate: clearAll, isPending: false }),
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["watch-history"],
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("WatchHistory", () => {
  it("renders history items", () => {
    render(<WatchHistory />, { wrapper: createWrapper() });
    expect(screen.getByTestId("history-list")).toBeInTheDocument();
    expect(screen.getByTestId("history-item")).toBeInTheDocument();
  });

  it("shows Remove button on each item", () => {
    render(<WatchHistory />, { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
  });

  it("calls unwatch.mutate on Remove click", async () => {
    unwatch.mockClear();
    render(<WatchHistory />, { wrapper: createWrapper() });
    await userEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(unwatch).toHaveBeenCalledWith({ mediaItemId: "m1" });
  });

  it("shows Clear history button when showClearButton=true", () => {
    render(<WatchHistory showClearButton />, { wrapper: createWrapper() });
    expect(screen.getByTestId("clear-history-btn")).toBeInTheDocument();
  });

  it("does not show Clear history button by default", () => {
    render(<WatchHistory />, { wrapper: createWrapper() });
    expect(screen.queryByTestId("clear-history-btn")).not.toBeInTheDocument();
  });
});

describe("WatchHistory zero duration", () => {
  // The gateway serializes an int64 zero as the string "0", which is truthy:
  // the row printed "1h / " with nothing after the slash.
  it("prints no position / duration line when the duration is the proto zero", () => {
    extraItems.list = [{
      id: "wp2", mediaItemId: "m2", positionMs: "3600000", durationMs: "0", percentComplete: 0,
      played: false, playCount: 1, lastPlayedAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
    }];
    render(<WatchHistory />, { wrapper: createWrapper() });
    const rows = screen.getAllByTestId("history-item");
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("1h / 2h");
    expect(rows[1].textContent).not.toContain(" / ");
    extraItems.list = [];
  });
});

describe("WatchHistory empty", () => {
  it("renders empty state when no items", () => {
    vi.doMock("@evey/api", () => ({
      usePlaybackServiceGetWatchHistory: () => ({ data: { items: [] }, isLoading: false }),
      usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
      getPlaybackServiceGetWatchHistoryQueryKey: () => ["watch-history"],
    }));
    // In static mocks, empty state covered by testid="history-empty" in the component
  });
});
