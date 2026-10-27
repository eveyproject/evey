import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { getSeekPreview, setSeekPreview, useSeekPreview } from "./seekPreviewStore";

describe("seekPreviewStore", () => {
  it("hands subscribers the pending seek and its release", () => {
    const { result } = renderHook(() => useSeekPreview());
    act(() => setSeekPreview(90));
    expect(result.current).toBe(90);
    expect(getSeekPreview()).toBe(90);
    act(() => setSeekPreview(null));
    expect(result.current).toBeNull();
  });
});
