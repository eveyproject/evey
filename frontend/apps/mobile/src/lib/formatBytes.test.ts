import { describe, it, expect } from "vitest";
import { formatBytes } from "./formatBytes";

describe("formatBytes", () => {
  it("renders 0 B for zero and nullish input", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(undefined)).toBe("0 B");
    expect(formatBytes(null)).toBe("0 B");
  });

  it("stays in bytes below 1 KB", () => {
    expect(formatBytes(1)).toBe("1 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1023)).toBe("1023 B");
  });

  it("crosses the exact 1024 (KB) boundary", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1024 * 1.5)).toBe("1.5 KB");
  });

  it("crosses the exact 1048576 (MB) boundary", () => {
    expect(formatBytes(1024 * 1024)).toBe("1.0 MB");
    expect(formatBytes(1024 * 1024 * 2.5)).toBe("2.5 MB");
  });

  it("handles GB and TB", () => {
    expect(formatBytes(3 * 1024 * 1024 * 1024)).toBe("3.0 GB");
    expect(formatBytes(2 * 1024 ** 4)).toBe("2.0 TB");
  });

  it("clamps negative values to 0 B instead of crashing", () => {
    expect(formatBytes(-1)).toBe("0 B");
    expect(formatBytes(-1024)).toBe("0 B");
  });

  it("handles very large values (PB range) without overflowing the unit table", () => {
    expect(formatBytes(5 * 1024 ** 5)).toBe("5.0 PB");
    expect(formatBytes(1024 ** 6)).toBe("1024.0 PB");
  });

  it("parses string inputs (generated API int64 fields serialize as strings)", () => {
    expect(formatBytes("2048")).toBe("2.0 KB");
    expect(formatBytes("0")).toBe("0 B");
  });
});
