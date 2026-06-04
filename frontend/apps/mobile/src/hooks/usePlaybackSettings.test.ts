import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { usePlaybackSettings } from "./usePlaybackSettings";

const mockHook = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  usePlaybackSettingsServiceGetPlaybackSettings: (opts: {
    query?: { select?: (d: unknown) => unknown };
  }) => {
    const raw = mockHook();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
  },
}));

describe("usePlaybackSettings", () => {
  it("returns skip seconds from the API", () => {
    mockHook.mockReturnValue({
      data: { skipForwardSeconds: 10, skipBackwardSeconds: 5 },
      isSuccess: true,
    });
    const { result } = renderHook(() => usePlaybackSettings());
    expect(result.current.data?.skipForwardSeconds).toBe(10);
    expect(result.current.data?.skipBackwardSeconds).toBe(5);
  });

  it("applies default 30s for skipForwardSeconds when field is absent", () => {
    mockHook.mockReturnValue({ data: { skipBackwardSeconds: 15 }, isSuccess: true });
    const { result } = renderHook(() => usePlaybackSettings());
    expect(result.current.data?.skipForwardSeconds).toBe(30);
    expect(result.current.data?.skipBackwardSeconds).toBe(15);
  });

  it("applies default 30s for skipBackwardSeconds when field is absent", () => {
    mockHook.mockReturnValue({ data: { skipForwardSeconds: 20 }, isSuccess: true });
    const { result } = renderHook(() => usePlaybackSettings());
    expect(result.current.data?.skipForwardSeconds).toBe(20);
    expect(result.current.data?.skipBackwardSeconds).toBe(30);
  });

  it("applies all defaults when API returns an empty object", () => {
    mockHook.mockReturnValue({ data: {}, isSuccess: true });
    const { result } = renderHook(() => usePlaybackSettings());
    expect(result.current.data?.skipForwardSeconds).toBe(30);
    expect(result.current.data?.skipBackwardSeconds).toBe(30);
  });
});
