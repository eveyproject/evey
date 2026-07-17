import { describe, it, expect } from "vitest";
import { formatDuration, formatFileSize, hasMediaFile } from "./mediaDetailUtils";

describe("formatDuration", () => {
  it("returns empty string for undefined", () => {
    expect(formatDuration(undefined)).toBe("");
  });

  it("returns empty string for empty string", () => {
    expect(formatDuration("")).toBe("");
  });

  it("returns empty string for non-numeric", () => {
    expect(formatDuration("abc")).toBe("");
  });

  it("formats minutes only when < 1 hour", () => {
    expect(formatDuration("2700000")).toBe("45m");
  });

  it("formats hours and minutes", () => {
    expect(formatDuration("7200000")).toBe("2h 0m");
  });

  it("formats hours and minutes combined", () => {
    expect(formatDuration("5520000")).toBe("1h 32m");
  });

  it("returns empty for 0 (series carry durationMs=0)", () => {
    expect(formatDuration("0")).toBe("");
  });

  it("returns empty for negative", () => {
    expect(formatDuration("-1")).toBe("");
  });

  it("floors partial seconds", () => {
    expect(formatDuration("61999")).toBe("1m");
  });
});

describe("formatFileSize", () => {
  it("returns empty string for undefined", () => {
    expect(formatFileSize(undefined)).toBe("");
  });

  it("returns empty string for empty string", () => {
    expect(formatFileSize("")).toBe("");
  });

  it("returns empty string for non-numeric", () => {
    expect(formatFileSize("xyz")).toBe("");
  });

  it("formats KB", () => {
    expect(formatFileSize("512000")).toBe("500 KB");
  });

  it("formats MB", () => {
    expect(formatFileSize("104857600")).toBe("100 MB");
  });

  it("formats GB with one decimal", () => {
    expect(formatFileSize("2147483648")).toBe("2.0 GB");
  });

  it("formats large GB values", () => {
    expect(formatFileSize("5368709120")).toBe("5.0 GB");
  });
});

describe("hasMediaFile", () => {
  it("is true for a non-admin item: fileSize present, filePath redacted", () => {
    expect(hasMediaFile({ fileSize: "1073741824", filePath: undefined })).toBe(true);
  });

  it("is true for an admin item: filePath present", () => {
    expect(hasMediaFile({ filePath: "/movies/x.mkv" })).toBe(true);
  });

  it('is false for a container: proto zero size "0" and no filePath', () => {
    expect(hasMediaFile({ fileSize: "0", filePath: undefined })).toBe(false);
  });

  it("is false when nothing is present", () => {
    expect(hasMediaFile({})).toBe(false);
  });
});
