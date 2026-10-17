import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k} ${JSON.stringify(o)}` : k) }) }));

import { DiskIOList, busiestDisk, formatBytesPerSecond } from "./DiskIO";

const theme = { colors: { surface: "#111", text: "#fff", textMuted: "#888", border: "#333", danger: "#d00" }, radii: { md: 8 } } as any;
const quiet = { label: "QEMU HARDDISK drive-scsi1", device: "sda", rotational: true, awaitMs: 7.5, utilPercent: 100, readBps: "52428800", writeBps: "0", saturated: false };
const slow = { label: "QEMU HARDDISK drive-scsi2", device: "sdc", rotational: false, awaitMs: 38.2, utilPercent: 61, readBps: "73400320", writeBps: "1048576", saturated: true };

describe("disk I/O (mobile)", () => {
  it("picks the disk that waited the most, not the busiest, and formats rates", () => {
    expect(busiestDisk([quiet, slow])?.device).toBe("sdc");
    expect(busiestDisk(undefined)).toBeUndefined();
    expect(formatBytesPerSecond("73400320")).toBe("73.4 MB/s");
    expect(formatBytesPerSecond(-1)).toBe("-");
  });

  it("lists every disk with its wait, flags the saturated one and shows the reference waits", () => {
    render(<DiskIOList disks={[quiet, slow]} idleAwaitMs={7.5} saturationAwaitMs={20} iowaitPercent={63.4} theme={theme} />);
    expect(screen.getAllByTestId("disk-io-row")).toHaveLength(2);
    expect(screen.getByText("7.5 ms")).toBeTruthy();
    expect(screen.getByText("38.2 ms")).toBeTruthy();
    expect(screen.getByText('admin.systemHealth.diskRestLine {"idle":"7.5","line":"20"}')).toBeTruthy();
    expect(screen.getByText("admin.systemHealth.diskSaturated")).toBeTruthy();
    expect(screen.getByText(/diskSolidState/)).toBeTruthy();
  });
});
