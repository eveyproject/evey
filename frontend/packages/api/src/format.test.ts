import { describe, it, expect } from "vitest";
import { formatBytes, formatTime, formatTimeMs, formatDurationMs, formatDurationSec, formatWatchTotalMs, formatWatchTotalSec } from "./format";

describe("formatBytes", () => {
  it("renders 0 B for empty or invalid input", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(undefined)).toBe("0 B");
    expect(formatBytes(null)).toBe("0 B");
    expect(formatBytes("0")).toBe("0 B");
    expect(formatBytes("not-a-number")).toBe("0 B");
    expect(formatBytes(-1024)).toBe("0 B");
  });

  it("keeps bytes as integers", () => {
    expect(formatBytes(1)).toBe("1 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1023)).toBe("1023 B");
  });

  it("uses binary units with one decimal above bytes", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1024 * 1.5)).toBe("1.5 KB");
    expect(formatBytes(1536 * 1024)).toBe("1.5 MB");
    expect(formatBytes(3 * 1024 ** 3)).toBe("3.0 GB");
    expect(formatBytes(2 * 1024 ** 4)).toBe("2.0 TB");
  });

  it("accepts the server's int64-as-string sizes", () => {
    expect(formatBytes("2048")).toBe("2.0 KB");
  });

  it("clamps to the last unit instead of walking off the table", () => {
    expect(formatBytes(5 * 1024 ** 5)).toBe("5.0 PB");
    expect(formatBytes(1024 ** 6)).toBe("1024.0 PB");
  });
});

describe("formatTime", () => {
  it("renders minutes and seconds under one hour", () => {
    expect(formatTime(0)).toBe("0:00");
    expect(formatTime(5)).toBe("0:05");
    expect(formatTime(125)).toBe("2:05");
    expect(formatTime(3599)).toBe("59:59");
  });

  it("adds the hour part from one hour up", () => {
    expect(formatTime(3600)).toBe("1:00:00");
    expect(formatTime(7384)).toBe("2:03:04");
  });

  it("floors fractional seconds", () => {
    expect(formatTime(61.7)).toBe("1:01");
  });

  it("clamps negative and non-finite input to 0:00", () => {
    expect(formatTime(-5)).toBe("0:00");
    expect(formatTime(NaN)).toBe("0:00");
    expect(formatTime(Infinity)).toBe("0:00");
  });
});

describe("formatTimeMs", () => {
  it("is formatTime over milliseconds", () => {
    expect(formatTimeMs(0)).toBe("0:00");
    expect(formatTimeMs(125_500)).toBe("2:05");
    expect(formatTimeMs(3_600_000)).toBe("1:00:00");
  });
});

describe("formatDurationMs", () => {
  it("is empty for missing, unparseable or zero input", () => {
    expect(formatDurationMs(undefined)).toBe("");
    expect(formatDurationMs(null)).toBe("");
    expect(formatDurationMs("")).toBe("");
    expect(formatDurationMs("abc")).toBe("");
    expect(formatDurationMs("0")).toBe("");
    expect(formatDurationMs("-1")).toBe("");
  });

  it("renders minutes under one hour", () => {
    expect(formatDurationMs("2700000")).toBe("45min");
    expect(formatDurationMs("61999")).toBe("1min");
  });

  it("renders hours with the minute remainder, dropping a zero remainder", () => {
    expect(formatDurationMs("5520000")).toBe("1h 32min");
    expect(formatDurationMs("7200000")).toBe("2h");
    expect(formatDurationMs(7200000)).toBe("2h");
  });
});

describe("formatDurationSec", () => {
  it("is empty for missing or zero input", () => {
    expect(formatDurationSec(undefined)).toBe("");
    expect(formatDurationSec("0")).toBe("");
  });

  it("renders the same human form from seconds", () => {
    expect(formatDurationSec(2700)).toBe("45min");
    expect(formatDurationSec("5520")).toBe("1h 32min");
    expect(formatDurationSec(7200)).toBe("2h");
  });
});

// A watch-time total: zero is an answer, and seconds watched are not "0min"
// (an admin read "Total Watch Time: 0min" on titles that had been played).
describe("formatWatchTotal", () => {
  it("says 0min for nothing, < 1min for seconds, minutes and hours beyond", () => {
    expect(formatWatchTotalMs(0)).toBe("0min");
    expect(formatWatchTotalMs(undefined)).toBe("0min");
    expect(formatWatchTotalMs("abc")).toBe("0min");
    expect(formatWatchTotalMs(42_000)).toBe("< 1min");
    expect(formatWatchTotalMs("90000")).toBe("1min");
    expect(formatWatchTotalMs(5_520_000)).toBe("1h 32min");
    expect(formatWatchTotalSec(42)).toBe("< 1min");
    expect(formatWatchTotalSec("3600")).toBe("1h");
    expect(formatWatchTotalSec(0)).toBe("0min");
  });
});
