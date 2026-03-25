import { describe, it, expect } from "vitest";
import { formatTime } from "./playerUtils";

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
