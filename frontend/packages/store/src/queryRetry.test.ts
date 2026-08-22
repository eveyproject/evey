import { describe, it, expect } from "vitest";
import { shouldRetryQuery } from "./queryRetry";

const err = (httpStatus: number) => Object.assign(new Error("boom"), { httpStatus });

describe("shouldRetryQuery", () => {
  it.each([401, 403, 404, 410, 429])("never retries a %i", (status) => {
    expect(shouldRetryQuery(0, err(status))).toBe(false);
  });

  it("retries transient failures up to twice", () => {
    expect(shouldRetryQuery(0, err(500))).toBe(true);
    expect(shouldRetryQuery(1, err(503))).toBe(true);
    expect(shouldRetryQuery(2, err(500))).toBe(false);
  });

  it("retries a network error with no status", () => {
    expect(shouldRetryQuery(0, new Error("network down"))).toBe(true);
  });
});
