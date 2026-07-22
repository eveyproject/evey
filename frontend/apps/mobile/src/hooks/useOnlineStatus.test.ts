import { describe, it, expect, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { onlineManager } from "@tanstack/react-query";
import { useOnlineStatus } from "./useOnlineStatus";

afterEach(() => {
  onlineManager.setOnline(true);
});

// The hook is now a view onto React Query's onlineManager rather than its own
// poller. What the OS state MEANS (captive portal, cold-start throw) is tested
// where that logic lives, in src/lib/online-manager.test.ts; what matters here
// is that the UI and the query layer can no longer disagree.
describe("useOnlineStatus", () => {
  it("reflects the shared online state", () => {
    onlineManager.setOnline(true);
    const { result } = renderHook(() => useOnlineStatus());
    expect(result.current).toBe(true);
  });

  it("re-renders when connectivity changes", () => {
    onlineManager.setOnline(true);
    const { result } = renderHook(() => useOnlineStatus());

    act(() => onlineManager.setOnline(false));
    expect(result.current).toBe(false);

    act(() => onlineManager.setOnline(true));
    expect(result.current).toBe(true);
  });
});
