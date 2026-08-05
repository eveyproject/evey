import { describe, it, expect } from "vitest";
import { formatSeriesRuntime, hasMediaFile } from "./mediaDetailUtils";

describe("formatSeriesRuntime", () => {
  it("is empty without episodes or runtime", () => {
    expect(formatSeriesRuntime(undefined, 0)).toBe("");
    expect(formatSeriesRuntime("0", 12)).toBe("");
  });

  it("renders hours with the minute remainder, dropping a zero remainder", () => {
    expect(formatSeriesRuntime("5520000", 2)).toBe("1h 32min");
    expect(formatSeriesRuntime("7200000", 3)).toBe("2h");
    expect(formatSeriesRuntime("2700000", 1)).toBe("45min");
  });

  it("switches to days for long totals", () => {
    expect(formatSeriesRuntime(String(26 * 3600000), 20)).toBe("1d 2h");
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
