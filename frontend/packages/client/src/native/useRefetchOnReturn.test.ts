import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

type AppStateListener = (state: string) => void;
let appStateListener: AppStateListener | null = null;
const removeSub = vi.fn();
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_event: string, cb: AppStateListener) => {
      appStateListener = cb;
      return { remove: removeSub };
    },
  },
}));

// A stand-in for the navigator: focus() runs the effect, blur() its cleanup.
let focusEffect: (() => void | (() => void)) | null = null;
let blurCleanup: (() => void) | void = undefined;
vi.mock("expo-router", () => ({
  useFocusEffect: (cb: () => void | (() => void)) => {
    focusEffect = cb;
  },
}));
const focus = () => act(() => { blurCleanup = focusEffect?.(); });
const blur = () => act(() => { blurCleanup?.(); blurCleanup = undefined; });

import { useRefetchOnReturn } from "./useRefetchOnReturn";

beforeEach(() => {
  appStateListener = null;
  focusEffect = null;
  blurCleanup = undefined;
  removeSub.mockClear();
});

describe("useRefetchOnReturn", () => {
  it("does not refetch on the first focus: the mount already fetched", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchOnReturn(refetch));
    focus();
    expect(refetch).not.toHaveBeenCalled();
  });

  // The 27-09 case: back from the player onto a page still mounted underneath.
  it("refetches when the screen is focused again", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchOnReturn(refetch));
    focus();
    blur();
    focus();
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("refetches when the app comes back to the foreground on this screen", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchOnReturn(refetch));
    focus();
    act(() => appStateListener?.("background"));
    act(() => appStateListener?.("active"));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  // A page buried under the player must not refetch behind it on every wake.
  it("does not refetch on foreground while another screen covers it", () => {
    const refetch = vi.fn();
    renderHook(() => useRefetchOnReturn(refetch));
    focus();
    blur();
    act(() => appStateListener?.("active"));
    expect(refetch).not.toHaveBeenCalled();
  });

  it("calls the latest refetch and drops its subscription on unmount", () => {
    const first = vi.fn();
    const second = vi.fn();
    const { rerender, unmount } = renderHook(({ r }) => useRefetchOnReturn(r), { initialProps: { r: first } });
    focus();
    rerender({ r: second });
    act(() => appStateListener?.("active"));
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    unmount();
    expect(removeSub).toHaveBeenCalled();
  });
});
