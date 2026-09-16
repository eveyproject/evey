import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, f?: unknown) => {
      if (f && typeof f === "object") return `${k}(${Object.entries(f as object).map(([a, b]) => `${a}=${String(b)}`).join(",")})`;
      return typeof f === "string" ? f : k;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", surface: "#111", success: "#0a0", warning: "#fa0", error: "#f33" }, radii: { sm: 4, md: 8 } }),
}));
const journal = vi.hoisted(() => ({ hook: vi.fn(), state: { data: undefined as unknown, isLoading: false } }));
vi.mock("@evey/api", () => ({
  useAdminMediaServiceListMediaRepairs: (...a: unknown[]) => {
    journal.hook(...a);
    return journal.state;
  },
  baseName: (p: string) => p.split("/").pop() ?? p,
}));

import { RepairJournalPanel } from "./RepairJournalPanel";

beforeEach(() => {
  journal.hook.mockReset();
  journal.state = { data: undefined, isLoading: false };
});

// Mobile twin of the web journal: fetch only once shown, the window pinned
// when chosen, the batch kind server-side and "operator" client-side.
describe("RepairJournalPanel (mobile)", () => {
  it("fetches only once shown, with the 7-day window by default", () => {
    render(<RepairJournalPanel />);
    expect(journal.hook).toHaveBeenLastCalledWith(expect.anything(), { query: { enabled: false } });
    fireEvent.click(screen.getByText("Show"));
    const [params, opts] = journal.hook.mock.calls.at(-1)!;
    expect(opts).toEqual({ query: { enabled: true } });
    expect(params.limit).toBe(200);
    expect(Date.now() - Number(params.sinceMs)).toBeGreaterThanOrEqual(7 * 86_400_000 - 1000);
    expect(screen.getByText("No repairs in this window.")).toBeTruthy();
  });

  it("filters the batch kind server-side and the operator view client-side", () => {
    journal.state = {
      isLoading: false,
      data: {
        entries: [
          { atMs: "1758000000000", path: "/m/a.mkv", op: "reindex", outcome: "applied", job: "reindex", evidenceMethod: "track-coherence", evidenceLagSec: 0.083, evidenceConfidence: 0.91 },
          { atMs: "1758000001000", path: "/m/b.mkv", op: "audio_retime", outcome: "failed", evidenceMethod: "operator", error: "erofs" },
        ],
      },
    };
    render(<RepairJournalPanel />);
    fireEvent.click(screen.getByText("Show"));
    expect(screen.getByText("admin.repairJournal.count(count=2)")).toBeTruthy();
    expect(screen.getByText("a.mkv")).toBeTruthy();
    expect(screen.getByText("admin.repairHistory.evidenceMeasured(lag=0.083,confidence=0.91)")).toBeTruthy();
    expect(screen.getByText("erofs")).toBeTruthy();

    fireEvent.click(screen.getByText("admin.repairJournal.jobOperator"));
    expect(journal.hook.mock.calls.at(-1)![0].job).toBeUndefined();
    expect(screen.getByText("admin.repairJournal.count(count=1)")).toBeTruthy();
    expect(screen.queryByText("a.mkv")).toBeNull();

    fireEvent.click(screen.getByText("admin.seekIndex.jobReindex"));
    expect(journal.hook.mock.calls.at(-1)![0].job).toBe("reindex");
    fireEvent.click(screen.getByText("admin.repairJournal.windowAll"));
    expect(journal.hook.mock.calls.at(-1)![0].sinceMs).toBeUndefined();
  });
});
