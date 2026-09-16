import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { StatsGrid } from "./StatsGrid";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { count?: number }) => (o?.count === undefined ? k : `${k}:${o.count}`) }),
}));
vi.mock("@/components/ui/Card", () => ({ Card: ({ children }: any) => React.createElement("div", null, children) }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, ...p }: any) => React.createElement("button", { onClick, disabled, "data-testid": p["data-testid"] }, children),
}));

const zeroBreakdown = {
  missing: 0, damaged: 0, wrongContainer: 0, resyncRequired: 0,
  truncated: 0, seekCoarse: 0, tracksDisagree: 0, delayedAudio: 0, audioShort: 0, trailingJunk: 0,
};

function mount(over: Record<string, unknown> = {}) {
  const ackGroup = { mutate: vi.fn(), isPending: false };
  const props = {
    result: { total: 12 },
    triage: { repairable: 0, informational: 0, acked: 0 },
    breakdown: zeroBreakdown,
    withBackup: 2,
    ackGroup,
    libraryId: undefined,
    busy: false,
    ...over,
  };
  render(<StatsGrid {...(props as any)} />);
  return ackGroup;
}

beforeEach(() => vi.clearAllMocks());

// A count never appears without saying what it is made of: every non-zero
// defect renders its own phrase, an all-zero card says "none". The
// acknowledge-all button exists only when there is something informational to
// acknowledge, scopes itself to the chosen library, and freezes while busy.
describe("StatsGrid", () => {
  it("lists only the non-zero parts and says none for an empty card", () => {
    mount({
      triage: { repairable: 5, informational: 0, acked: 0 },
      breakdown: { ...zeroBreakdown, missing: 3, resyncRequired: 2 },
    });
    expect(screen.getByTestId("seek-index-repairable-count")).toHaveTextContent("5");
    expect(screen.getByTestId("seek-index-missing-count")).toHaveTextContent("admin.seekIndex.partMissing:3");
    expect(screen.getByTestId("seek-index-resync-required-count")).toHaveTextContent("admin.seekIndex.partResyncRequired:2");
    expect(screen.queryByTestId("seek-index-damaged-count")).toBeNull();
    expect(screen.queryByTestId("seek-index-wrong-container-count")).toBeNull();
    // The informational card is empty: a single "none" line, no button.
    expect(screen.getByText("admin.seekIndex.breakdownNone")).toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-ack-informational-btn")).toBeNull();
    expect(screen.queryByTestId("seek-index-acked-count")).toBeNull();
    expect(screen.getByText("12")).toBeInTheDocument();
  });

  it("acknowledges the informational group scoped to the library", () => {
    const ackGroup = mount({
      triage: { repairable: 0, informational: 4, acked: 1 },
      breakdown: { ...zeroBreakdown, seekCoarse: 3, trailingJunk: 1 },
      libraryId: "lib-1",
    });
    expect(screen.getByTestId("seek-index-informational-count")).toHaveTextContent("4");
    expect(screen.getByTestId("seek-index-coarse-count")).toHaveTextContent(":3");
    expect(screen.getByTestId("seek-index-trailing-junk-count")).toHaveTextContent(":1");
    expect(screen.getByTestId("seek-index-acked-count")).toHaveTextContent("admin.seekIndex.partAcked:1");
    const btn = screen.getByTestId("seek-index-ack-informational-btn") as HTMLButtonElement;
    expect(btn).toHaveTextContent("admin.seekIndex.ackGroupBtn:4");
    fireEvent.click(btn);
    expect(ackGroup.mutate).toHaveBeenCalledWith({ data: { libraryId: "lib-1", verdict: "informational" } });
  });

  it("acknowledges across libraries when none is chosen and freezes while busy", () => {
    const ackGroup = mount({ triage: { repairable: 0, informational: 2, acked: 0 }, breakdown: { ...zeroBreakdown, truncated: 2 } });
    fireEvent.click(screen.getByTestId("seek-index-ack-informational-btn"));
    expect(ackGroup.mutate).toHaveBeenCalledWith({ data: { verdict: "informational" } });

    mount({ triage: { repairable: 0, informational: 2, acked: 0 }, breakdown: { ...zeroBreakdown, truncated: 2 }, busy: true });
    const buttons = screen.getAllByTestId("seek-index-ack-informational-btn") as HTMLButtonElement[];
    expect(buttons.at(-1)!.disabled).toBe(true);
  });
});
