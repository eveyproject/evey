import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

const journal = vi.hoisted(() => ({ hook: vi.fn(), state: { data: undefined as unknown, isLoading: false, isError: false, error: null as unknown, refetch: vi.fn() } }));
vi.mock("@evey/api", () => ({
  useAdminMediaServiceListMediaRepairs: (...args: unknown[]) => {
    journal.hook(...args);
    return journal.state;
  },
  baseName: (p: string) => p.split("/").pop() ?? p,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.entries(o).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Card", () => ({ Card: ({ children, ...p }: any) => React.createElement("div", p, children) }));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children, variant }: any) => React.createElement("span", { "data-variant": variant }, children) }));
vi.mock("@/components/ui/Button", () => ({ Button: ({ children, onClick, ...p }: any) => React.createElement("button", { onClick, "data-testid": p["data-testid"] }, children) }));

import { RepairJournalPanel } from "./RepairJournalPanel";

beforeEach(() => {
  journal.hook.mockReset();
  journal.state = { data: undefined, isLoading: false, isError: false, error: null, refetch: vi.fn() };
});

// The journal answers "what did the repairs touch": it fetches only once
// opened, with a window pinned to the moment the filter changed and the
// batch kind as a server filter, except "operator" which is a client-side
// view of the entries no batch made; each entry says what it rested on.
describe("RepairJournalPanel", () => {
  it("fetches only when opened, defaulting to the last 7 days", () => {
    render(<RepairJournalPanel />);
    expect(journal.hook).toHaveBeenLastCalledWith(expect.anything(), { query: { enabled: false } });
    fireEvent.click(screen.getByTestId("repair-journal-toggle"));
    const [params, opts] = journal.hook.mock.calls.at(-1)!;
    expect(opts).toEqual({ query: { enabled: true } });
    expect(params.limit).toBe(200);
    expect(params.job).toBeUndefined();
    const since = Number(params.sinceMs);
    expect(Date.now() - since).toBeGreaterThanOrEqual(7 * 86_400_000 - 1000);
    expect(Date.now() - since).toBeLessThan(7 * 86_400_000 + 60_000);
    expect(screen.getByTestId("repair-journal-empty")).toBeInTheDocument();
  });

  it("the batch kind is a server filter, 'operator' a client-side one, 'all' drops the window", () => {
    journal.state = {
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      data: {
        entries: [
          { atMs: "1758000000000", path: "/media/a.mkv", op: "reindex", outcome: "applied", job: "reindex", evidenceMethod: "track-coherence", evidenceLagSec: 0.083, evidenceConfidence: 0.91 },
          { atMs: "1758000001000", path: "/media/b.mkv", op: "audio_retime", outcome: "failed", evidenceMethod: "operator", error: "erofs" },
          { atMs: "1758000002000", path: "/media/c.mkv", op: "container_fix", outcome: "rolled_back", job: "fix_container" },
        ],
      },
    };
    render(<RepairJournalPanel />);
    fireEvent.click(screen.getByTestId("repair-journal-toggle"));
    expect(screen.getAllByTestId("repair-journal-entry")).toHaveLength(3);
    expect(screen.getByTestId("repair-journal-count")).toHaveTextContent("count=3");
    expect(screen.getByText("a.mkv")).toBeInTheDocument();
    expect(screen.getByText("admin.repairHistory.evidenceMeasured(lag=0.083,confidence=0.91)")).toBeInTheDocument();
    expect(screen.getByText("admin.repairJournal.evidenceOperator")).toBeInTheDocument();
    expect(screen.getByText("admin.repairHistory.evidenceDeclared")).toBeInTheDocument();
    expect(screen.getByText("erofs")).toBeInTheDocument();
    expect(screen.getByText("admin.repairHistory.outcome.failed").getAttribute("data-variant")).toBe("error");
    expect(screen.getByText("admin.repairHistory.outcome.rolled_back").getAttribute("data-variant")).toBe("warning");

    fireEvent.change(screen.getByTestId("repair-journal-job"), { target: { value: "operator" } });
    expect(journal.hook.mock.calls.at(-1)![0].job).toBeUndefined();
    expect(screen.getAllByTestId("repair-journal-entry")).toHaveLength(1);
    expect(screen.getByText("b.mkv")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("repair-journal-job"), { target: { value: "reindex" } });
    expect(journal.hook.mock.calls.at(-1)![0].job).toBe("reindex");

    fireEvent.change(screen.getByTestId("repair-journal-window"), { target: { value: "all" } });
    expect(journal.hook.mock.calls.at(-1)![0].sinceMs).toBeUndefined();
  });

  // ListMediaRepairs refuses while the seek index is not configured: a failure
  // to read, not an empty window.
  it("says the journal failed to load, not that the window is empty, and retries", () => {
    journal.state = { data: undefined, isLoading: false, isError: true, error: new Error("unavailable"), refetch: vi.fn() };
    render(<RepairJournalPanel />);
    fireEvent.click(screen.getByTestId("repair-journal-toggle"));
    expect(screen.queryByTestId("repair-journal-empty")).toBeNull();
    expect(screen.queryByTestId("repair-journal-count")).toBeNull();
    expect(screen.getByTestId("fetch-error-state")).toBeInTheDocument();
    fireEvent.click(screen.getByText("common.retry"));
    expect(journal.state.refetch).toHaveBeenCalled();
  });
});
