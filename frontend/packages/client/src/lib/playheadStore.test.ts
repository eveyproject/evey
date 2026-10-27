import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { getPlayhead, setPlayhead, usePlayhead } from "./playheadStore";

describe("playheadStore", () => {
  it("hands subscribers each new position and nothing for a repeat", () => {
    const { result } = renderHook(() => usePlayhead());
    act(() => setPlayhead(12.5));
    expect(result.current).toBe(12.5);
    expect(getPlayhead()).toBe(12.5);
    act(() => setPlayhead(12.5));
    expect(result.current).toBe(12.5);
    act(() => setPlayhead(13));
    expect(result.current).toBe(13);
  });
});
