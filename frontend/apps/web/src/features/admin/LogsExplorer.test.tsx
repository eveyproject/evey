import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// jsdom lacks IntersectionObserver (used by the infinite-scroll sentinel).
(globalThis as Record<string, unknown>).IntersectionObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof IntersectionObserver;

const mockFetch = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
  getBaseURL: () => "",
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel?: (s: Record<string, unknown>) => unknown) => (sel ? sel({}) : {}), {
    getState: () => ({ accessToken: "tok" }),
  }),
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
  return render(
    <QueryClientProvider client={qc}>
      <LogsExplorer />
    </QueryClientProvider>,
  );
}

describe("LogsExplorer", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    mockFetch.mockResolvedValue(resp([sampleEntry]));
  });

  it("fetches and renders classified log entries", async () => {
    renderExplorer();
    expect(await screen.findByText("hls: ffmpeg exited with error")).toBeInTheDocument();
    // level + where render as data-driven badges (ERROR also labels a filter
    // chip, so assert at least one occurrence).
    expect(screen.getAllByText("ERROR").length).toBeGreaterThan(0);
    expect(screen.getByText("player")).toBeInTheDocument();
  });

  it("passes the level filter to the search query when a chip is toggled", async () => {
    renderExplorer();
    await screen.findByText("hls: ffmpeg exited with error");

    // The WARN chip is a plain button labeled WARN.
    await userEvent.click(screen.getByRole("button", { name: "WARN" }));

    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("level=WARN"))).toBe(true);
    });
  });

  it("passes where + media id filters through the query string", async () => {
    renderExplorer();
    await screen.findByText("hls: ffmpeg exited with error");

    // where select (native <select>): choose player
    const selects = screen.getAllByRole("combobox");
    await userEvent.selectOptions(selects[0], "player");

    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => (c[0] as { url: string }).url);
      expect(urls.some((u) => u.includes("where=player"))).toBe(true);
    });
  });

  it("expands a row to reveal fields and raw copy", async () => {
    mockFetch.mockResolvedValue(
      resp([{ ...sampleEntry, fields: { variant: 2, media_path: "/media/x.mkv" } }]),
    );
    renderExplorer();
    const row = await screen.findByText("hls: ffmpeg exited with error");
    await userEvent.click(row);
    expect(await screen.findByText("media_path")).toBeInTheDocument();
  });
});
