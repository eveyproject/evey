import { describe, it, expect } from "vitest";
import { formatWait, parseRateLimit } from "./rateLimitMessage";

const t = (_k: string, fb: string, p?: Record<string, number>) =>
  fb.replace("{{count}}", String(p?.count ?? ""));

describe("parseRateLimit", () => {
  it("ignores anything that is not a 429", () => {
    expect(parseRateLimit(401, "60", { retryAfterSeconds: 60 })).toBeNull();
    expect(parseRateLimit(200, null, null)).toBeNull();
  });

  it("reads the Retry-After header first", () => {
    expect(parseRateLimit(429, "42", { retryAfterSeconds: 999 })).toEqual({ retryAfterSeconds: 42 });
  });

  it("falls back to the body when a proxy strips the header", () => {
    expect(parseRateLimit(429, null, { retryAfterSeconds: 17 })).toEqual({ retryAfterSeconds: 17 });
  });

  it("never reports an unknown wait", () => {
    expect(parseRateLimit(429, "garbage", {})).toEqual({ retryAfterSeconds: 60 });
    expect(parseRateLimit(429, null, null)).toEqual({ retryAfterSeconds: 60 });
  });
});

describe("formatWait", () => {
  it("uses seconds under a minute, rounding up", () => {
    expect(formatWait(3, t)).toBe("3 s");
    expect(formatWait(0.2, t)).toBe("1 s");
  });

  it("uses whole minutes above, rounding up so nobody returns too early", () => {
    expect(formatWait(60, t)).toBe("1 min");
    expect(formatWait(61, t)).toBe("2 min");
    expect(formatWait(600, t)).toBe("10 min");
  });
});
