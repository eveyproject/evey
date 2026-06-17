import { describe, it, expect } from "vitest";
import { formatTime, nominalQualityTier } from "./playerUtils";

describe("nominalQualityTier", () => {
  it("rounds a widescreen height to the nearest standard tier", () => {
    expect(nominalQualityTier(798)).toBe(720); // 2.40:1 "1080p" rung → 720p (closer)
    expect(nominalQualityTier(532)).toBe(480);
    expect(nominalQualityTier(1040)).toBe(1080);
  });

  it("keeps exact standard heights", () => {
    expect(nominalQualityTier(720)).toBe(720);
    expect(nominalQualityTier(1080)).toBe(1080);
    expect(nominalQualityTier(2160)).toBe(2160);
  });

  it("returns 0 for non-positive heights", () => {
    expect(nominalQualityTier(0)).toBe(0);
    expect(nominalQualityTier(-5)).toBe(0);
  });
});

describe("formatTime", () => {
  it("formats seconds under a minute", () => {
    expect(formatTime(0)).toBe("0:00");
    expect(formatTime(5)).toBe("0:05");
    expect(formatTime(59)).toBe("0:59");
  });

  it("formats minutes", () => {
    expect(formatTime(60)).toBe("1:00");
    expect(formatTime(125)).toBe("2:05");
    expect(formatTime(3599)).toBe("59:59");
  });

  it("formats hours", () => {
    expect(formatTime(3600)).toBe("1:00:00");
    expect(formatTime(3661)).toBe("1:01:01");
    expect(formatTime(7384)).toBe("2:03:04");
  });

  it("truncates fractional seconds", () => {
    expect(formatTime(61.7)).toBe("1:01");
  });
});
