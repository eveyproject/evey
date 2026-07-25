import { describe, it, expect } from "vitest";
import { audioDelayed, formatSize, baseName } from "./seekIndexHelpers";

describe("audioDelayed", () => {
  it("is false when the delay is at or below the defect floor", () => {
    expect(audioDelayed({ audioDelaySec: 0 })).toBe(false);
    expect(audioDelayed({ audioDelaySec: 0.1 })).toBe(false);
    expect(audioDelayed({})).toBe(false);
  });

  it("is true above the defect floor", () => {
    expect(audioDelayed({ audioDelaySec: 0.11 })).toBe(true);
    expect(audioDelayed({ audioDelaySec: 1.5 })).toBe(true);
  });
});

describe("formatSize", () => {
  it("renders 0 B for falsy input", () => {
    expect(formatSize(0)).toBe("0 B");
  });

  it("picks the right unit and precision", () => {
    expect(formatSize(512)).toBe("512 B");
    expect(formatSize(2048)).toBe("2.0 KB");
    expect(formatSize(1536 * 1024)).toBe("1.5 MB");
    expect(formatSize(3 * 1024 * 1024 * 1024)).toBe("3.0 GB");
  });
});

describe("baseName", () => {
  it("strips the directory portion of a path", () => {
    expect(baseName("/data/media/movies/film.mkv")).toBe("film.mkv");
  });

  it("returns the input unchanged when there is no slash", () => {
    expect(baseName("film.mkv")).toBe("film.mkv");
  });
});
