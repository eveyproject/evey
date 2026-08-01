import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { LibraryListView } from "./LibraryListView";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params }: { children: React.ReactNode; to: string; params?: Record<string, string> }) => (
    <a href={`${to}/${params?.mediaId ?? ""}`}>{children}</a>
  ),
}));

// The default global mock (src/test/setup.ts) renders EVERY row - fine for
// components that only need list content, but it can never prove windowing
// (a non-virtualized component would pass those tests too). This local mock
// gives the test a real sliding window it can move, mirroring
// LogsExplorer.test.tsx / the mock's own precedent in this repo.
const virtualMockState = vi.hoisted(() => ({
  windowStart: 0,
  windowSize: 5,
}));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number; estimateSize: (i: number) => number }) => {
    const { windowStart, windowSize } = virtualMockState;
    const maxStart = Math.max(0, opts.count - windowSize);
    const start = Math.min(windowStart, maxStart);
    const end = Math.min(opts.count, start + windowSize);
    const items = Array.from({ length: Math.max(0, end - start) }, (_, i) => {
      const idx = start + i;
      return { index: idx, key: idx, start: idx * opts.estimateSize(idx), size: opts.estimateSize(idx) };
    });
    return {
      getVirtualItems: () => items,
      getTotalSize: () => opts.count * opts.estimateSize(0),
      measureElement: () => {},
      options: { scrollMargin: 0 },
    };
  },
}));

function makeItem(i: number): V1MediaItem {
  return {
    id: `mi-${i}`,
    title: `Item ${i}`,
    type: "MEDIA_ITEM_TYPE_MOVIE",
  } as V1MediaItem;
}

function renderList(items: V1MediaItem[], extraProps: Partial<React.ComponentProps<typeof LibraryListView>> = {}) {
  return render(<LibraryListView items={items} {...extraProps} />);
}

describe("LibraryListView", () => {
  beforeEach(() => {
    virtualMockState.windowStart = 0;
  });

  it("mounts only the current virtual window in the DOM, and follows it when it moves", () => {
    const items = Array.from({ length: 200 }, (_, i) => makeItem(i));
    const { rerender } = renderList(items);

    // Window at the top: rows 0-4 are mounted, row 195 (far below) is not.
    expect(screen.getByText("Item 0")).toBeInTheDocument();
    expect(screen.getByText("Item 4")).toBeInTheDocument();
    expect(screen.queryByText("Item 195")).not.toBeInTheDocument();

    // Move the window to the tail of the list, as if the user had scrolled
    // all the way down, and force a re-render.
    virtualMockState.windowStart = 195;
    rerender(<LibraryListView items={items} />);

    expect(screen.getByText("Item 199")).toBeInTheDocument();
    expect(screen.getByText("Item 195")).toBeInTheDocument();
    // Proof this is a SLIDING window, not just a cap: the earlier rows that
    // were mounted a moment ago are gone now that the window moved past them.
    expect(screen.queryByText("Item 0")).not.toBeInTheDocument();
    expect(screen.queryByText("Item 4")).not.toBeInTheDocument();
  });

  it("calls onLoadMore once the virtual window reaches the last row, and not before", () => {
    const items = Array.from({ length: 50 }, (_, i) => makeItem(i));
    const onLoadMore = vi.fn();
    const { rerender } = renderList(items, { hasNextPage: true, isFetchingNextPage: false, onLoadMore });

    // Window sits at the top (rows 0-4): nowhere near the last row (49). A
    // tautological trigger would fire regardless of window position - this
    // must NOT call onLoadMore.
    expect(onLoadMore).not.toHaveBeenCalled();

    // Move the window so its last visible row IS the last item (49) - this is
    // what "scrolled to the bottom" looks like to the virtualizer.
    virtualMockState.windowStart = 45;
    rerender(<LibraryListView items={items} hasNextPage isFetchingNextPage={false} onLoadMore={onLoadMore} />);

    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("does not call onLoadMore at the end of the list when hasNextPage is false", () => {
    const items = Array.from({ length: 10 }, (_, i) => makeItem(i));
    const onLoadMore = vi.fn();
    virtualMockState.windowStart = 8; // window reaches the last row already
    renderList(items, { hasNextPage: false, isFetchingNextPage: false, onLoadMore });

    expect(onLoadMore).not.toHaveBeenCalled();
  });

  it("does not call onLoadMore at the end of the list while a fetch is already in flight", () => {
    const items = Array.from({ length: 10 }, (_, i) => makeItem(i));
    const onLoadMore = vi.fn();
    virtualMockState.windowStart = 8;
    renderList(items, { hasNextPage: true, isFetchingNextPage: true, onLoadMore });

    expect(onLoadMore).not.toHaveBeenCalled();
  });
});
