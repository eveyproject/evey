import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { usePlaybackSettings } from "./usePlaybackSettings";

const mockHook = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  usePlaybackSettingsServiceGetPlaybackSettings: (opts: { query?: { select?: (d: unknown) => unknown } }) => {
    const raw = mockHook();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
  },
}));

describe("usePlaybackSettings", () => {
  it("returns settings from API", () => {
    mockHook.mockReturnValue({ data: { skipForwardSeconds: 30, skipBackwardSeconds: 15 }, isSuccess: true });
    const { result } = renderHook(() => usePlaybackSettings());
    expect(result.current.data?.skipForwardSeconds).toBe(30);
    expect(result.current.data?.skipBackwardSeconds).toBe(15);
  });

  it("applies defaults for missing fields", () => {
    mockHook.mockReturnValue({ data: {}, isSuccess: true });
    const { result } = renderHook(() => usePlaybackSettings());
    // Default skip was raised to 30s (commits 93bab7098 / e55fd646c).
    expect(result.current.data?.skipForwardSeconds).toBe(30);
    expect(result.current.data?.skipBackwardSeconds).toBe(30);
  });
});
