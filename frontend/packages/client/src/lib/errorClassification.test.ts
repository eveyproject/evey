import { describe, it, expect } from "vitest";
import { classifyError } from "./errorClassification";

describe("classifyError", () => {
  it("classifies a 404 as not-found", () => {
    expect(classifyError(Object.assign(new Error("nope"), { httpStatus: 404 }))).toBe("not-found");
  });

  it("classifies a 500 as server", () => {
    expect(classifyError(Object.assign(new Error("boom"), { httpStatus: 500 }))).toBe("server");
  });

  it("classifies a 503 as server", () => {
    expect(classifyError(Object.assign(new Error("unavailable"), { httpStatus: 503 }))).toBe("server");
  });

  it("classifies other 4xx (e.g. 403) as unknown, not offline or not-found", () => {
    expect(classifyError(Object.assign(new Error("forbidden"), { httpStatus: 403 }))).toBe("unknown");
  });

  it("classifies an error with no httpStatus as offline (fetch never reached the server)", () => {
    expect(classifyError(new TypeError("Failed to fetch"))).toBe("offline");
  });

  it("classifies non-error values as offline", () => {
    expect(classifyError(undefined)).toBe("offline");
    expect(classifyError(null)).toBe("offline");
    expect(classifyError("boom")).toBe("offline");
  });
});
