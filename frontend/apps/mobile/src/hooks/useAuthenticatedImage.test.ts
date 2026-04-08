import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";

let mockToken: string | null = "test-token";

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { accessToken: string | null }) => unknown) =>
    selector({ accessToken: mockToken }),
}));

import { useAuthenticatedImage, useAuthHeaders } from "./useAuthenticatedImage";

describe("useAuthenticatedImage", () => {
  it("returns undefined when uri is undefined", () => {
    mockToken = "test-token";
    const { result } = renderHook(() => useAuthenticatedImage(undefined));
    expect(result.current).toBeUndefined();
  });

  it("returns undefined when token is null", () => {
    mockToken = null;
    const { result } = renderHook(() => useAuthenticatedImage("http://example.com/img.jpg"));
    expect(result.current).toBeUndefined();
  });

  it("appends token with ? when uri has no query string", () => {
    mockToken = "abc123";
    const { result } = renderHook(() => useAuthenticatedImage("http://example.com/img.jpg"));
    expect(result.current?.uri).toBe("http://example.com/img.jpg?token=abc123");
  });

  it("appends token with & when uri already has query params", () => {
    mockToken = "abc123";
    const { result } = renderHook(() => useAuthenticatedImage("http://example.com/img.jpg?quality=high"));
    expect(result.current?.uri).toBe("http://example.com/img.jpg?quality=high&token=abc123");
  });

  it("sets Authorization header with Bearer token", () => {
    mockToken = "abc123";
    const { result } = renderHook(() => useAuthenticatedImage("http://example.com/img.jpg"));
    expect(result.current?.headers).toEqual({ Authorization: "Bearer abc123" });
  });

  it("returns both uri and headers together", () => {
    mockToken = "mytoken";
    const { result } = renderHook(() => useAuthenticatedImage("http://host/path"));
    expect(result.current).toEqual({
      uri: "http://host/path?token=mytoken",
      headers: { Authorization: "Bearer mytoken" },
    });
  });
});

describe("useAuthHeaders", () => {
  it("returns undefined when token is null", () => {
    mockToken = null;
    const { result } = renderHook(() => useAuthHeaders());
    expect(result.current).toBeUndefined();
  });

  it("returns Authorization header when token is present", () => {
    mockToken = "mytoken";
    const { result } = renderHook(() => useAuthHeaders());
    expect(result.current).toEqual({ Authorization: "Bearer mytoken" });
  });
});
