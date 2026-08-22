import { describe, it, expect } from "vitest";
import { queryClient } from "./query-client";

// The retry predicate is what actually protects the remote/server: a status
// a retry cannot fix (401/403/404) must stop immediately, everything else
// still gets the normal 2-retry backoff.
describe("tv query-client retry predicate", () => {
  const retry = queryClient.getDefaultOptions().queries?.retry as (
    failureCount: number,
    error: unknown,
  ) => boolean;

  it("does not retry a 403", () => {
    expect(retry(0, Object.assign(new Error("forbidden"), { httpStatus: 403 }))).toBe(false);
  });

  it("does not retry a 401", () => {
    expect(retry(0, Object.assign(new Error("unauthorized"), { httpStatus: 401 }))).toBe(false);
  });

  it("does not retry a 404", () => {
    expect(retry(0, Object.assign(new Error("not found"), { httpStatus: 404 }))).toBe(false);
  });

  it("does not retry a 410", () => {
    expect(retry(0, Object.assign(new Error("gone"), { httpStatus: 410 }))).toBe(false);
  });

  it("does not retry a 429", () => {
    expect(retry(0, Object.assign(new Error("too many requests"), { httpStatus: 429 }))).toBe(
      false,
    );
  });

  it("retries a 500 up to 2 times", () => {
    const err = Object.assign(new Error("server error"), { httpStatus: 500 });
    expect(retry(0, err)).toBe(true);
    expect(retry(1, err)).toBe(true);
    expect(retry(2, err)).toBe(false);
  });
});
