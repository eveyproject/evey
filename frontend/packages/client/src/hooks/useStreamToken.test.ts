import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockEveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

import { useStreamToken } from "./useStreamToken";

describe("useStreamToken", () => {
  beforeEach(() => {
    mockEveyFetch.mockReset();
  });

  it("returns null token initially", () => {
    const { result } = renderHook(() => useStreamToken());
    expect(result.current.streamToken).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it("fetches stream token and updates state", async () => {
    mockEveyFetch.mockResolvedValue({ token: "opaque-token-1", mediaId: "m1" });
    const { result } = renderHook(() => useStreamToken());

    let token: string;
    await act(async () => {
      token = await result.current.fetchStreamToken("m1");
    });

    expect(token!).toBe("opaque-token-1");
    expect(result.current.streamToken).toBe("opaque-token-1");
    expect(result.current.isLoading).toBe(false);
    expect(mockEveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/stream-token",
      method: "POST",
    });
  });

  it("sets isLoading false even on error", async () => {
    mockEveyFetch.mockRejectedValue(new Error("401"));
    const { result } = renderHook(() => useStreamToken());

    await act(async () => {
      await result.current.fetchStreamToken("m1").catch(() => {});
    });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.streamToken).toBeNull();
  });
});
