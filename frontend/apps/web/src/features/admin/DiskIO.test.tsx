import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { DisksStatCard, DiskIOList, busiestDisk, formatBytesPerSecond } from "./DiskIO";

const quiet = { label: "QEMU HARDDISK drive-scsi1", device: "sda", rotational: true, awaitMs: 7.5, utilPercent: 100, readBps: "52428800", writeBps: "0", saturated: false };
const slow = { label: "QEMU HARDDISK drive-scsi2", device: "sdc", rotational: true, awaitMs: 38.2, utilPercent: 61, readBps: "73400320", writeBps: "1048576", saturated: true };

describe("disk I/O", () => {
  // The load test of 2026-10-15 broke with its disks at 56-82 % busy and
  // every request waiting 27-46 ms: the wait is the signal, not the share.
  it("picks the disk that waited the most, not the busiest one", () => {
    expect(busiestDisk([quiet, slow])?.device).toBe("sdc");
    expect(busiestDisk([])).toBeUndefined();
  });

  it("formats byte rates the way iostat reads", () => {
    expect(formatBytesPerSecond("73400320")).toBe("73.4 MB/s");
    expect(formatBytesPerSecond(2048)).toBe("2 kB/s");
    expect(formatBytesPerSecond(undefined)).toBe("0 B/s");
  });

  it("renders the dashboard tile on the worst disk and says so when it is saturated", () => {
    render(<DisksStatCard disks={[quiet, slow]} />);
    const card = screen.getByTestId("disks-stat-card");
    expect(card).toHaveTextContent("38.2 ms");
    expect(card).toHaveTextContent("Saturated · QEMU HARDDISK drive-scsi2");
    expect(card).toHaveTextContent("Disks");
  });

  it("names the disk and its busy share when it keeps up", () => {
    render(<DisksStatCard disks={[quiet]} />);
    expect(screen.getByTestId("disks-stat-card")).toHaveTextContent("QEMU HARDDISK drive-scsi1 · busy 100%");
  });

  it("renders nothing when the machine exposes no disk, rather than a zero", () => {
    const { container } = render(<DisksStatCard disks={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("lists every disk with the wait first and the reference lines the verdict comes from", () => {
    render(<DiskIOList disks={[quiet, slow]} idleAwaitMs={7.5} saturationAwaitMs={20} iowaitPercent={63.4} />);
    const list = screen.getByTestId("disk-io-list");
    expect(list).toHaveTextContent("At rest 7.5 ms · saturated above 20 ms");
    expect(list).toHaveTextContent("CPU waiting on disks: 63.4%");
    const rows = screen.getAllByTestId("disk-io-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("7.5 ms");
    expect(rows[0]).toHaveTextContent("busy 100%");
    expect(rows[0]).toHaveTextContent("read 52.4 MB/s · write 0 B/s");
    expect(rows[0]).not.toHaveTextContent("Saturated");
    expect(rows[1]).toHaveTextContent("38.2 ms");
    expect(rows[1]).toHaveTextContent("Saturated");
  });
});
