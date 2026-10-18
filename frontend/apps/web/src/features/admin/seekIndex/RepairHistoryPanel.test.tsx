import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({
  history: { data: undefined as unknown, isLoading: false, isError: false, error: null as unknown, refetch: vi.fn() },
  historyArgs: vi.fn(),
  undo: vi.fn(),
  undoOpts: null as any,
}));
vi.mock("@evey/api", () => ({
  getAdminMediaServiceListMediaRepairsQueryKey: () => ["/api/v1/admin/media/repairs"],
  SEEK_INDEX_SCAN_QUERY_BASE: "/api/v1/admin/media/seek-index",
  useAdminMediaServiceGetMediaRepairHistory: (...a: unknown[]) => {
    api.historyArgs(...a);
    return api.history;
  },
  useAdminMediaServiceUndoMediaRepair: (opts: any) => {
    api.undoOpts = opts;
    return { mutate: api.undo, isPending: false };
  },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.entries(o).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Button", () => ({ Button: ({ children, onClick, ...p }: any) => React.createElement("button", { onClick, "data-testid": p["data-testid"] }, children) }));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children, variant }: any) => React.createElement("span", { "data-variant": variant }, children) }));

import { RepairHistoryPanel } from "./RepairHistoryPanel";

let qc: QueryClient;
function renderWithQC(ui: React.ReactElement) {
  qc = new QueryClient();
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  api.history = { data: undefined, isLoading: false, isError: false, error: null, refetch: vi.fn() };
});

// The journal is the way back, with or without a backup copy. Each
// entry shows what was done, on what evidence (a measurement or the file's
// own claim), how far the sound moved; undo is offered only while an entry
// is reversible and refetches the journal once done.
describe("RepairHistoryPanel", () => {
  it("lists the entries with their evidence and shift, and offers undo on a reversible one", () => {
    api.history.data = {
      entries: [
        { atMs: "1758000000000", op: "audio_retime", outcome: "applied", reversible: true, shiftSec: { "2": 0.083, "3": -0.25 }, evidenceMethod: "track-coherence", evidenceLagSec: 0.083, evidenceConfidence: 0.91 },
        { atMs: "1757000000000", op: "reindex", outcome: "failed", evidenceMethod: "declared-defect", error: "erofs" },
      ],
    };
    const onClose = vi.fn();
    renderWithQC(<RepairHistoryPanel mediaId="mi-1" title="Film" onClose={onClose} />);
    expect(api.historyArgs).toHaveBeenCalledWith("mi-1", { limit: 20 });
    expect(screen.getByText("admin.repairHistory.title(title=Film)")).toBeInTheDocument();
    expect(screen.getAllByTestId("repair-history-entry")).toHaveLength(2);
    expect(screen.getByTestId("repair-history-shift")).toHaveTextContent("shift=2: +0.083s, 3: -0.250s");
    expect(screen.getByText("admin.repairHistory.evidenceMeasured(lag=0.083,confidence=0.91)")).toBeInTheDocument();
    expect(screen.getByText("admin.repairHistory.evidenceDeclared")).toBeInTheDocument();
    expect(screen.getByText("erofs")).toBeInTheDocument();
    expect(screen.getByText("admin.repairHistory.outcome.failed").getAttribute("data-variant")).toBe("error");
    expect(screen.getByText("admin.repairHistory.reversible")).toBeInTheDocument();
    expect(screen.queryByTestId("repair-history-not-reversible")).toBeNull();

    fireEvent.click(screen.getByTestId("repair-history-undo-btn"));
    expect(api.undo).toHaveBeenCalledWith({ mediaId: "mi-1", data: {} });
    api.undoOpts.mutation.onSuccess();
    expect(api.history.refetch).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("repair-history-close"));
    expect(onClose).toHaveBeenCalled();
  });

  // An operator's repair read as the file's own claim here, while the journal
  // already named it; both now read the same label.
  it("names an operator's repair as the operator's call", () => {
    api.history.data = { entries: [{ atMs: "1758000000000", op: "rollback", outcome: "applied", evidenceMethod: "operator-undo" }] };
    renderWithQC(<RepairHistoryPanel mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    expect(screen.getByText("admin.repairJournal.evidenceOperator")).toBeInTheDocument();
    expect(screen.queryByText("admin.repairHistory.evidenceDeclared")).toBeNull();
  });

  it("says when nothing can be undone, and when there is no history at all", () => {
    api.history.data = { entries: [{ atMs: "1", op: "rollback", outcome: "rolled_back", reversible: false }] };
    renderWithQC(<RepairHistoryPanel mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    expect(screen.queryByTestId("repair-history-undo-btn")).toBeNull();
    expect(screen.getByTestId("repair-history-not-reversible")).toBeInTheDocument();
    expect(screen.queryByTestId("repair-history-shift")).toBeNull();

    api.history = { data: { entries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() };
    renderWithQC(<RepairHistoryPanel mediaId="mi-2" title="Other" onClose={vi.fn()} />);
    expect(screen.getByTestId("repair-history-empty")).toBeInTheDocument();
  });

  // GetMediaRepairHistory refuses while the seek index is not configured: a
  // failure to read, not a file never repaired.
  it("says the history failed to load, not that it is empty, and retries", () => {
    api.history = { data: undefined, isLoading: false, isError: true, error: new Error("unavailable"), refetch: vi.fn() };
    renderWithQC(<RepairHistoryPanel mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    expect(screen.queryByTestId("repair-history-empty")).toBeNull();
    expect(screen.getByTestId("fetch-error-state")).toBeInTheDocument();
    fireEvent.click(screen.getByText("common.retry"));
    expect(api.history.refetch).toHaveBeenCalled();
  });

  // An undo rewrites the file: the journal and the seek-index row are
  // refreshed along with this history.
  it("refreshes the journal and the seek index after an undo", () => {
    renderWithQC(<RepairHistoryPanel mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    api.undoOpts.mutation.onSuccess();
    expect(invalidate.mock.calls.map(([f]) => f?.queryKey)).toEqual([
      ["/api/v1/admin/media/repairs"],
      ["/api/v1/admin/media/seek-index"],
    ]);
  });
});
