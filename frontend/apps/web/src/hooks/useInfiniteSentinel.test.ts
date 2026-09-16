import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useInfiniteSentinel } from "./useInfiniteSentinel";

// The sentinel asks for the next page when it scrolls into view - and only
// when there is one and none is in flight - and does nothing at all where
// IntersectionObserver does not exist instead of throwing.
describe("useInfiniteSentinel", () => {
  afterEach(() => vi.unstubAllGlobals());

  function stubObserver() {
    let callback: ((entries: { isIntersecting: boolean }[]) => void) | undefined;
    const observe = vi.fn();
    const disconnect = vi.fn();
    class FakeObserver {
      constructor(cb: (entries: { isIntersecting: boolean }[]) => void) { callback = cb; }
      observe = observe;
      disconnect = disconnect;
    }
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    return { fire: (visible: boolean) => callback?.([{ isIntersecting: visible }]), observe, disconnect };
  }

  it("fetches when visible with a next page and nothing in flight", () => {
    const { fire, observe } = stubObserver();
    const fetchNext = vi.fn();
    const el = document.createElement("div");
    // Attach the element the way a ref does, then re-run the effect.
    const rerendered = renderHook(({ hasNext, fetching }) => {
      const ref = useInfiniteSentinel(hasNext, fetching, fetchNext);
      (ref as { current: HTMLDivElement | null }).current = el;
      return ref;
    }, { initialProps: { hasNext: true, fetching: false } });
    rerendered.rerender({ hasNext: true, fetching: false });
    expect(observe).toHaveBeenCalled();

    fire(false);
    expect(fetchNext).not.toHaveBeenCalled();
    fire(true);
    expect(fetchNext).toHaveBeenCalledTimes(1);

    rerendered.rerender({ hasNext: true, fetching: true });
    fire(true);
    expect(fetchNext).toHaveBeenCalledTimes(1);
    rerendered.rerender({ hasNext: false, fetching: false });
    fire(true);
    expect(fetchNext).toHaveBeenCalledTimes(1);
  });

  it("disconnects on unmount and survives a missing IntersectionObserver", () => {
    const { disconnect } = stubObserver();
    const el = document.createElement("div");
    const hook = renderHook(() => {
      const ref = useInfiniteSentinel(true, false, vi.fn());
      (ref as { current: HTMLDivElement | null }).current = el;
      return ref;
    });
    hook.rerender();
    hook.unmount();
    expect(disconnect).toHaveBeenCalled();

    vi.stubGlobal("IntersectionObserver", undefined);
    expect(() => {
      const h = renderHook(() => {
        const ref = useInfiniteSentinel(true, false, vi.fn());
        (ref as { current: HTMLDivElement | null }).current = el;
        return ref;
      });
      h.rerender();
    }).not.toThrow();
  });
});
