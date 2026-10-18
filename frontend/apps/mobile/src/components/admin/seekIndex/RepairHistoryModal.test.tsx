import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Modal: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, f?: unknown) => (f && typeof f === "object" ? `${k}(${Object.entries(f as object).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : typeof f === "string" ? f : k),
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", success: "#0a0", warning: "#fa0", error: "#f33" }, radii: { lg: 12, md: 8 } }),
}));
const api = vi.hoisted(() => ({ history: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() }, historyArgs: vi.fn(), undo: vi.fn(), undoOpts: null as any }));
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

import { RepairHistoryModal } from "./RepairHistoryModal";

let qc: QueryClient;
function mount(ui: React.ReactElement) {
  qc = new QueryClient();
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  api.history = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

// Mobile twin of the web history: each entry with its evidence and shift,
// undo only while an entry is reversible, the journal refetched after it.
describe("RepairHistoryModal (mobile)", () => {
  it("lists entries with evidence and shift, undoes a reversible one and refetches", () => {
    api.history.data = {
      entries: [
        { atMs: "1758000000000", op: "audio_retime", outcome: "applied", reversible: true, shiftSec: { "2": 0.083 }, evidenceMethod: "track-coherence", evidenceLagSec: 0.083, evidenceConfidence: 0.91 },
        { atMs: "1757000000000", op: "reindex", outcome: "failed", evidenceMethod: "declared-defect", error: "erofs" },
      ],
    };
    const onClose = vi.fn();
    mount(<RepairHistoryModal mediaId="mi-1" title="Film" onClose={onClose} />);
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    expect(api.historyArgs).toHaveBeenCalledWith("mi-1", { limit: 20 });
    expect(screen.getByText("admin.repairHistory.title(title=Film)")).toBeTruthy();
    expect(screen.getByText("admin.repairHistory.shift(shift=2: +0.083s)")).toBeTruthy();
    expect(screen.getByText("admin.repairHistory.evidenceMeasured(lag=0.083,confidence=0.91)")).toBeTruthy();
    expect(screen.getByText("admin.repairHistory.evidenceDeclared")).toBeTruthy();
    expect(screen.getByText("erofs")).toBeTruthy();
    fireEvent.click(screen.getByText("Undo last repair").closest("button")!);
    expect(api.undo).toHaveBeenCalledWith({ mediaId: "mi-1", data: {} });
    api.undoOpts.mutation.onSuccess();
    expect(api.history.refetch).toHaveBeenCalled();
    // The undo moved the library journal and the seek-index row too.
    expect(invalidate.mock.calls.map(([f]) => f?.queryKey)).toEqual([["/api/v1/admin/media/repairs"], ["/api/v1/admin/media/seek-index"]]);
    fireEvent.click(screen.getByText("common.close").closest("button")!);
    expect(onClose).toHaveBeenCalled();
  });

  it("says when nothing is reversible, and when there is no history", () => {
    api.history.data = { entries: [{ atMs: "1", op: "rollback", outcome: "rolled_back", reversible: false }] };
    mount(<RepairHistoryModal mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    expect(screen.queryByText("Undo last repair")).toBeNull();
    expect(screen.getByText("No reversible repair to undo.")).toBeTruthy();
    api.history = { data: { entries: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    mount(<RepairHistoryModal mediaId="mi-2" title="Other" onClose={vi.fn()} />);
    expect(screen.getByText("No repair history for this file.")).toBeTruthy();
  });

  // ListMediaRepairs and GetMediaRepairHistory refuse while the seek index is
  // not configured: that is a failure to read, not a file never repaired.
  it("says the history failed to load, not that it is empty, and retries", () => {
    api.history = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    mount(<RepairHistoryModal mediaId="mi-1" title="Film" onClose={vi.fn()} />);
    expect(screen.queryByText("No repair history for this file.")).toBeNull();
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry").closest("button")!);
    expect(api.history.refetch).toHaveBeenCalled();
  });
});
