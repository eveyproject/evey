import { describe, it, expect, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useAuthStore } from "@evey/store";
import { useImageToken } from "./imageToken";

function jwt(expOffsetSec: number, tag: string): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + expOffsetSec, tag }));
  return `h.${payload}.s`;
}

beforeEach(() => {
  useAuthStore.getState().logout();
});

describe("useImageToken", () => {
  it("keeps the pinned token across a rotation while it is still valid", () => {
    const first = jwt(600, "a");
    act(() => useAuthStore.getState().setTokens(first));
    const { result } = renderHook(() => useImageToken());
    expect(result.current).toBe(first);

    act(() => useAuthStore.getState().setTokens(jwt(900, "b")));
    expect(result.current).toBe(first);
  });

  it("moves to the live token once the pinned one has expired", () => {
    const stale = jwt(-5, "a");
    act(() => useAuthStore.getState().setTokens(stale));
    const { result } = renderHook(() => useImageToken());
    expect(result.current).toBe(stale);

    const fresh = jwt(600, "b");
    act(() => useAuthStore.getState().setTokens(fresh));
    expect(result.current).toBe(fresh);
  });

  it("drops the token on sign-out", () => {
    act(() => useAuthStore.getState().setTokens(jwt(600, "a")));
    const { result } = renderHook(() => useImageToken());
    act(() => useAuthStore.getState().logout());
    expect(result.current).toBeNull();
  });
});
