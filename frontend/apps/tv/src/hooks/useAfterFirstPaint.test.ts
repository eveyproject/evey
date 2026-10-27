import { describe, expect, it } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useAfterFirstPaint } from "./useAfterFirstPaint";

describe("useAfterFirstPaint", () => {
  it("is false on the render that first shows a key, true two frames later, and starts over on a new key", async () => {
    const { result, rerender } = renderHook(({ k }: { k: string | undefined }) => useAfterFirstPaint(k), {
      initialProps: { k: undefined as string | undefined },
    });
    expect(result.current).toBe(false);
    rerender({ k: "a" });
    expect(result.current).toBe(false);
    await waitFor(() => expect(result.current).toBe(true));
    act(() => rerender({ k: "b" }));
    expect(result.current).toBe(false);
    await waitFor(() => expect(result.current).toBe(true));
  });
});
