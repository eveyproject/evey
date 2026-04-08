import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";

vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn((selector: (s: any) => unknown) =>
    selector({ accessToken: null }),
  ),
}));

import { useAuthenticatedImage, useAuthHeaders } from "./useAuthenticatedImage";
import { useAuthStore } from "@evey/store";

const mockUseAuthStore = vi.mocked(useAuthStore);

function setToken(token: string | null) {
  mockUseAuthStore.mockImplementation((selector: (s: any) => unknown) =>
    selector({ accessToken: token }),
  );
}

describe("useAuthenticatedImage", () => {
  it("returns undefined when no token", () => {
    setToken(null);
    const { result } = renderHook(() => useAuthenticatedImage("https://evey.test/img"));
    expect(result.current).toBeUndefined();
  });

  it("returns undefined when no uri", () => {
    setToken("tok123");
    const { result } = renderHook(() => useAuthenticatedImage(undefined));
    expect(result.current).toBeUndefined();
  });

  it("returns uri and Authorization header when both present", () => {
    setToken("tok123");
    const { result } = renderHook(() => useAuthenticatedImage("https://evey.test/img"));
    expect(result.current).toEqual({
      uri: "https://evey.test/img",
      headers: { Authorization: "Bearer tok123" },
    });
  });

  it("does not append token as query param (TV differs from mobile)", () => {
    setToken("abc");
    const { result } = renderHook(() => useAuthenticatedImage("https://evey.test/img"));
    expect(result.current?.uri).toBe("https://evey.test/img");
    expect(result.current?.uri).not.toContain("token=");
  });
});

describe("useAuthHeaders", () => {
  it("returns undefined when no token", () => {
    setToken(null);
    const { result } = renderHook(() => useAuthHeaders());
    expect(result.current).toBeUndefined();
  });

  it("returns Authorization header when token present", () => {
    setToken("mytoken");
    const { result } = renderHook(() => useAuthHeaders());
    expect(result.current).toEqual({ Authorization: "Bearer mytoken" });
  });
});
