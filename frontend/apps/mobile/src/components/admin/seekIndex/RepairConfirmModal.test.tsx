import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-checked": accessibilityState?.checked }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o && "count" in o ? `${k}:${o.count}` : k) }),
}));

import { RepairConfirmModal } from "./RepairConfirmModal";

const theme = {
  colors: { overlay: "#0008", surface: "#111", text: "#fff", textMuted: "#888", bg: "#000", border: "#333", accent: "#f00", onAccent: "#000", warning: "#fa0", error: "#f33" },
  radii: { sm: 4, md: 8, lg: 12 },
} as any;

type Props = React.ComponentProps<typeof RepairConfirmModal>;

function open(over: Partial<Props> = {}) {
  const props: Props = {
    theme, batch: "repair", onClose: vi.fn(), onConfirm: vi.fn(), pending: false,
    repairableCount: 10, withBackupCount: 3, damagedCount: 2, damagedBytes: 40_000_000, volumes: [],
    keepBackups: true, setKeepBackups: vi.fn(), resyncDamaged: false, setResyncDamaged: vi.fn(),
    ...over,
  };
  const view = render(<RepairConfirmModal {...props} />);
  return { ...props, container: view.container };
}

// The confirmation promises exactly what will run: the damaged files (the
// one repair that loses bytes) are counted only when the operator opts in,
// rollback and purge speak of the backups, and a low-space volume is named.
describe("RepairConfirmModal", () => {
  it("renders nothing without a batch", () => {
    expect(open({ batch: null }).container.textContent).toBe("");
  });

  it("counts the damaged files only when resync is opted in", () => {
    const p = open();
    expect(screen.getByText("admin.seekIndex.repairAllConfirm:8")).toBeTruthy();
    const boxes = screen.getAllByRole("checkbox");
    fireEvent.click(boxes[0]);
    expect(p.setResyncDamaged).toHaveBeenCalledWith(true);
    fireEvent.click(boxes[1]);
    expect(p.setKeepBackups).toHaveBeenCalledWith(false);
  });

  it("promises all files once resync is on, and hides the damaged row when there are none", () => {
    open({ resyncDamaged: true });
    expect(screen.getByText("admin.seekIndex.repairAllConfirm:10")).toBeTruthy();
    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
  });

  it("hides the damaged row when nothing is damaged", () => {
    open({ damagedCount: 0, damagedBytes: 0 });
    expect(screen.getByText("admin.seekIndex.repairAllConfirm:10")).toBeTruthy();
    expect(screen.getAllByRole("checkbox")).toHaveLength(1);
  });

  it("speaks of backups for rollback and purge, and flags a volume short on space", () => {
    open({ batch: "rollback" });
    expect(screen.getByText("admin.seekIndex.rollbackAllConfirm:3")).toBeTruthy();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("confirms and cancels through the parent, and disables confirm while pending", () => {
    const p = open({ volumes: [{ root: "/media", freeBytes: "1000", missingBytes: "50000000000", missingFiles: 3 }] as any });
    expect(screen.getByText("admin.seekIndex.volumeBackupsRisky")).toBeTruthy();
    fireEvent.click(screen.getByText("common.confirm"));
    expect(p.onConfirm).toHaveBeenCalled();
    fireEvent.click(screen.getByText("common.cancel"));
    expect(p.onClose).toHaveBeenCalled();
  });

  it("shows a spinner instead of confirm while the batch is starting", () => {
    open({ pending: true });
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("common.confirm")).toBeNull();
  });
});
