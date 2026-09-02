import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// jsdom lacks IntersectionObserver (unused by LogsExplorer now, but other
// tests in this suite still share the jsdom env, so keep the stub cheap).
(globalThis as Record<string, unknown>).IntersectionObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof IntersectionObserver;

const mockFetch = vi.fn();

// The admin listings the explorer resolves ids against: one account with one
// profile. The ids are deliberately opaque - the point is that the operator
// never has to read them.
const identityState = vi.hoisted(() => ({
  users: [{ id: "usr-ddcb76cd509e05d0aa289796", username: "mcarvalho" }],
  profiles: [{ id: "prf-77aa", userId: "usr-ddcb76cd509e05d0aa289796", name: "Salon" }],
}));

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
  getBaseURL: () => "",
  useUserServiceListUsers: () => ({ data: { users: identityState.users } }),
  useUserServiceListAllProfiles: () => ({ data: { profiles: identityState.profiles } }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel?: (s: Record<string, unknown>) => unknown) => (sel ? sel({}) : {}), {
    getState: () => ({ accessToken: "tok" }),
  }),
}));

// The global test setup (src/test/setup.ts) mocks @tanstack/react-virtual to
// render EVERY row - convenient for components that only need list content,
// but it would hide a virtualization regression in LogsExplorer's own tests
// (a mock that renders everything can never prove windowing). vi.hoisted lets
// the mock factory below and the test bodies share mutable state: tests move
// `virtualMockState.windowStart` to simulate scrolling, then force a
// re-render, and the mocked useVirtualizer recomputes its window from that
// position - a REAL sliding window, not a fixed 0..4 slice a non-virtualized
// component would also satisfy.
const virtualMockState = vi.hoisted(() => ({
  windowStart: 0,
  windowSize: 5,
  measureElement: vi.fn(),
}));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: {
    count: number;
    estimateSize: (i: number) => number;
    getItemKey?: (i: number) => string | number;
  }) => {
    const { windowStart, windowSize, measureElement } = virtualMockState;
    const maxStart = Math.max(0, opts.count - windowSize);
    const start = Math.min(windowStart, maxStart);
    const end = Math.min(opts.count, start + windowSize);
    const items = Array.from({ length: Math.max(0, end - start) }, (_, i) => {
      const idx = start + i;
      // Mirror the real hook: the item key comes from getItemKey when the
      // caller provides one, and only falls back to the index otherwise.
      return {
        index: idx,
        key: opts.getItemKey ? opts.getItemKey(idx) : idx,
        start: idx * opts.estimateSize(idx),
        size: opts.estimateSize(idx),
      };
    });
    return {
      getVirtualItems: () => items,
      getTotalSize: () => opts.count * opts.estimateSize(0),
      measureElement,
    };
  },
}));

import { LogsExplorer } from "./LogsExplorer";

function resp(entries: unknown[], extra: Record<string, unknown> = {}) {
  return { entries, total: entries.length, scanned: entries.length, truncated: false, ...extra };
}

const sampleEntry = {
  time: "2026-07-07T19:22:01Z",
  level: "ERROR",
  msg: "hls: ffmpeg exited with error",
  where: "player",
  source: "ffmpeg",
  origin: "server",
  mediaId: "mi-1",
  raw: '{"msg":"hls: ffmpeg exited with error"}',
};

function renderExplorer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  const view = render(
    <QueryClientProvider client={qc}>
      <LogsExplorer />
    </QueryClientProvider>,
  );
  return {
    ...view,
    qc,
    // Forces LogsExplorer to re-render against the SAME component instance
    // and query client, so it re-runs useVirtualizer (picking up a moved
    // virtualMockState.windowStart) without losing fetched data or state -
    // the test-equivalent of "the user scrolled".
    rerender: () =>
      view.rerender(
        <QueryClientProvider client={qc}>
          <LogsExplorer />
        </QueryClientProvider>,
      ),
  };
}

// The explorer now lands on the live tail (Direct pre-checked); the history
// search only runs once the user toggles Live off. Most tests exercise the
// history path, so they start from there.
async function renderExplorerOnHistory() {
  const view = renderExplorer();
  await userEvent.click(screen.getByRole("button", { name: /live/i }));
  return view;
}

function sseResponse(lines: Record<string, unknown>[]): Response {
  const body = lines.map((l) => `data: ${JSON.stringify(l)}\n\n`).join("");
  return new Response(body, { status: 200 });
}

describe("LogsExplorer", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    mockFetch.mockResolvedValue(resp([sampleEntry]));
    virtualMockState.windowStart = 0;
    virtualMockState.measureElement.mockClear();
  });

  it("starts on the live tail without firing a history search", () => {
    renderExplorer();
    // The SSE tail goes through raw fetch, not eveyFetch - a pre-checked
    // Direct mode means no /admin/logs history query at mount.
    expect(mockFetch).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /live/i })).toBeInTheDocument();
  });

  it("fetches and renders classified log entries once Live is toggled off", async () => {
    await renderExplorerOnHistory();
    expect(await screen.findByText("hls: ffmpeg exited with error")).toBeInTheDocument();
    // level + where render as data-driven badges (ERROR also labels a filter
    // chip, so assert at least one occurrence).
    expect(screen.getAllByText("ERROR").length).toBeGreaterThan(0);
    expect(screen.getByText("player")).toBeInTheDocument();
  });

  it("passes the level filter to the search query when a chip is toggled", async () => {
    await renderExplorerOnHistory();
    await screen.findByText("hls: ffmpeg exited with error");

    // The WARN chip is a plain button labeled WARN.
    await userEvent.click(screen.getByRole("button", { name: "WARN" }));

    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("level=WARN"))).toBe(true);
    });
  });

  it("passes where + media id filters through the query string", async () => {
    await renderExplorerOnHistory();
    await screen.findByText("hls: ffmpeg exited with error");

    // where select (native <select>): choose player
    const selects = screen.getAllByRole("combobox");
    await userEvent.selectOptions(selects[0], "player");

    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("where=player"))).toBe(true);
    });
  });

  it("names the account on the collapsed line and decorates every id it knows", async () => {
    mockFetch.mockResolvedValue(
      resp([{
        ...sampleEntry,
        userId: "usr-ddcb76cd509e05d0aa289796",
        fields: { user_id: "usr-ddcb76cd509e05d0aa289796", profile_id: "prf-77aa", session_id: "ses-1" },
      }]),
    );
    await renderExplorerOnHistory();
    const row = await screen.findByText("hls: ffmpeg exited with error");
    // Collapsed: the name is already there, the id only as a tooltip.
    expect(screen.getByTitle("usr-ddcb76cd509e05d0aa289796")).toHaveTextContent("mcarvalho");
    await userEvent.click(row);
    // Expanded: both the promoted user line and the raw fields carry the name
    // in front of the id; the profile names its owner; unknown ids stay bare.
    expect(screen.getAllByText("mcarvalho (usr-ddcb76cd509e05d0aa289796)").length).toBeGreaterThan(0);
    expect(screen.getByText("Salon (mcarvalho) (prf-77aa)")).toBeInTheDocument();
    expect(screen.getByText("ses-1")).toBeInTheDocument();
  });

  it("accepts a username in the user filter and searches by its id", async () => {
    await renderExplorerOnHistory();
    await screen.findByText("hls: ffmpeg exited with error");
    await userEvent.click(screen.getByRole("button", { name: /advanced/i }));
    await userEvent.type(screen.getByLabelText(/user \(name or id\)/i), "MCarvalho");

    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("user_id=usr-ddcb76cd509e05d0aa289796"))).toBe(true);
    });
  });

  it("expands a row to reveal fields and raw copy", async () => {
    mockFetch.mockResolvedValue(
      resp([{ ...sampleEntry, fields: { variant: 2, media_path: "/media/x.mkv" } }]),
    );
    await renderExplorerOnHistory();
    const row = await screen.findByText("hls: ffmpeg exited with error");
    await userEvent.click(row);
    expect(await screen.findByText("media_path")).toBeInTheDocument();
  });

  it("mounts only the current virtual window in the DOM, and follows it when it moves", async () => {
    const many = Array.from({ length: 200 }, (_, i) => ({ ...sampleEntry, msg: `entry-${i}`, raw: `entry-${i}` }));
    mockFetch.mockResolvedValue(resp(many, { total: 200 }));
    const view = await renderExplorerOnHistory();
    await screen.findByText("entry-0");

    // Window at the top: rows 0-4 are mounted, row 195 (far below) is not.
    expect(screen.getByText("entry-4")).toBeInTheDocument();
    expect(screen.queryByText("entry-195")).not.toBeInTheDocument();

    // Move the window to the tail of the list, as if the operator had
    // scrolled all the way down, and force a re-render.
    virtualMockState.windowStart = 195;
    view.rerender();

    await screen.findByText("entry-199");
    expect(screen.getByText("entry-195")).toBeInTheDocument();
    // Proof this is a SLIDING window, not just a cap: the earlier rows that
    // were mounted a moment ago are gone now that the window moved past them.
    expect(screen.queryByText("entry-0")).not.toBeInTheDocument();
    expect(screen.queryByText("entry-4")).not.toBeInTheDocument();
  });

  it("wires measureElement to every rendered row, for dynamic (expand/collapse) height", async () => {
    await renderExplorerOnHistory();
    await screen.findByText("hls: ffmpeg exited with error");
    expect(virtualMockState.measureElement).toHaveBeenCalled();
  });

  it("only fetches the next page once the virtual window reaches the last row, not before", async () => {
    const page1 = Array.from({ length: 50 }, (_, i) => ({ ...sampleEntry, msg: `row-${i}`, raw: `row-${i}` }));
    mockFetch.mockResolvedValueOnce(resp(page1, { total: 100 }));

    const view = await renderExplorerOnHistory();
    await screen.findByText("row-0");
    expect(mockFetch).toHaveBeenCalledTimes(1);

    // Window sits in the middle of the list (rows 20-24): nowhere near the
    // last row (49). A tautological trigger would fire regardless of where
    // the window is - this must NOT fetch.
    virtualMockState.windowStart = 20;
    view.rerender();
    await screen.findByText("row-24");
    await new Promise((r) => setTimeout(r, 50));
    expect(mockFetch).toHaveBeenCalledTimes(1);

    // Now move the window so its last visible row IS the last entry (49) -
    // this is what "scrolled to the bottom" looks like to the virtualizer.
    virtualMockState.windowStart = 45;
    view.rerender();
    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("offset=50"))).toBe(true);
    });
  });

  it("never triggers fetchNextPage while in live mode, even when the query still has a next page", async () => {
    // A Response body stream can only be read once, and the live tail
    // re-fetches every time `live` flips back on - mockImplementation (not
    // mockResolvedValue) gives every call its own fresh stream.
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async () =>
        sseResponse([{ time: "2026-07-07T19:22:01Z", level: "INFO", msg: "live line", component: "player_client" }]),
      );

    try {
      // The infinite query keeps its last computed hasNextPage even once
      // `enabled` flips back to false (toggling to live doesn't clear react-query's
      // cache), so a page total larger than what was fetched leaves hasNextPage
      // TRUE while live. If that alone were enough to fetch, the only thing
      // standing between it and a call is the `if (live) return;` guard itself -
      // this is what the test below actually exercises. The page has to fill
      // the whole 5-row virtual window (not just 1 row), otherwise the
      // (legitimate) history-mode auto-fetch fires immediately on arrival and
      // masks the thing under test.
      const historyPage = Array.from({ length: 10 }, (_, i) => ({ ...sampleEntry, msg: `hist-${i}`, raw: `hist-${i}` }));
      mockFetch.mockResolvedValueOnce(resp(historyPage, { total: 50 }));
      const view = renderExplorer(); // starts on the live tail (default)
      const liveButton = screen.getByRole("button", { name: /live/i });

      await userEvent.click(liveButton); // -> history: fetches the page above
      await screen.findByText("hist-0");
      const callsAfterHistory = mockFetch.mock.calls.length;

      await userEvent.click(liveButton); // -> back to live
      await screen.findByText("live line");

      // Put the virtual window at the end of the (short) live list too, so
      // the "reached bottom" half of the condition is satisfied either way.
      virtualMockState.windowStart = 999;
      view.rerender();

      await new Promise((r) => setTimeout(r, 50));
      expect(mockFetch.mock.calls.length).toBe(callsAfterHistory);
    } finally {
      fetchSpy.mockRestore();
    }
  });

  it("keeps an expanded live row on its own entry when newer lines arrive", async () => {
    // Live lines are PREPENDED, so every existing row shifts down one index on
    // each tick. With rows keyed by index the open row kept its position and
    // silently adopted the newcomer's content - the reader watched the log
    // they had just expanded turn into another one.
    let push!: (rec: Record<string, unknown>) => void;
    const enc = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        push = (rec) => controller.enqueue(enc.encode(`data: ${JSON.stringify(rec)}\n\n`));
      },
    });
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(body, { status: 200 }));

    try {
      renderExplorer(); // starts on the live tail
      await waitFor(() => expect(fetchSpy).toHaveBeenCalled());

      push({ time: "2026-07-07T19:22:01Z", level: "INFO", msg: "first line", component: "player_client", detail: "detail-first" });
      const firstRow = await screen.findByRole("button", { name: /first line/ });

      await userEvent.click(firstRow);
      expect(firstRow).toHaveAttribute("aria-expanded", "true");
      expect(screen.getByText("detail-first")).toBeInTheDocument();

      push({ time: "2026-07-07T19:22:02Z", level: "INFO", msg: "second line", component: "player_client", detail: "detail-second" });
      await screen.findByText("second line");

      // The panel still belongs to the row the operator opened, and the
      // newcomer arrives collapsed.
      expect(screen.getByRole("button", { name: /first line/ })).toHaveAttribute("aria-expanded", "true");
      expect(screen.getByRole("button", { name: /second line/ })).toHaveAttribute("aria-expanded", "false");
      expect(screen.getByText("detail-first")).toBeInTheDocument();
      expect(screen.queryByText("detail-second")).not.toBeInTheDocument();
    } finally {
      fetchSpy.mockRestore();
    }
  });
});
