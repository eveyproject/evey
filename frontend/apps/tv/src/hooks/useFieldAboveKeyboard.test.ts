import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useFieldAboveKeyboard } from "./useFieldAboveKeyboard";

const layout = (v: { y?: number; height?: number }) => ({ nativeEvent: { layout: { x: 0, y: 0, width: 0, height: 0, ...v } } }) as never;

describe("useFieldAboveKeyboard", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  function mount() {
    const scrollTo = vi.fn();
    const hook = renderHook(({ editing }) => useFieldAboveKeyboard(editing), { initialProps: { editing: false } });
    (hook.result.current.scrollRef as { current: unknown }).current = { scrollTo };
    act(() => {
      hook.result.current.onViewportLayout(layout({ height: 675 }));
      hook.result.current.onFieldLayout(layout({ y: 300 }));
    });
    return { hook, scrollTo };
  }

  it("leaves a form that is not being edited where it is", () => {
    const { hook, scrollTo } = mount();
    expect(hook.result.current.contentStyle).toBeNull();
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("brings the first field under the top edge while a field is edited, and follows it when the layout moves", () => {
    const { hook, scrollTo } = mount();
    hook.rerender({ editing: true });
    expect(hook.result.current.contentStyle).toEqual({ justifyContent: "flex-start", paddingBottom: 675 });
    expect(scrollTo).toHaveBeenLastCalledWith({ y: 276, animated: false });
    act(() => { hook.result.current.onFieldLayout(layout({ y: 120 })); });
    expect(scrollTo).toHaveBeenLastCalledWith({ y: 96, animated: false });
    // The first request can land before the content has grown: it is repeated.
    scrollTo.mockClear();
    hook.result.current.onContentSizeChange();
    expect(scrollTo).toHaveBeenLastCalledWith({ y: 96, animated: false });
  });

  it("stays up while the focus moves from one field to the next, and comes back once editing is over", () => {
    const { hook, scrollTo } = mount();
    hook.rerender({ editing: true });
    hook.rerender({ editing: false });
    act(() => { vi.advanceTimersByTime(50); });
    hook.rerender({ editing: true });
    act(() => { vi.advanceTimersByTime(500); });
    expect(hook.result.current.contentStyle).not.toBeNull();
    hook.rerender({ editing: false });
    act(() => { vi.advanceTimersByTime(200); });
    expect(hook.result.current.contentStyle).toBeNull();
    expect(scrollTo).toHaveBeenLastCalledWith({ y: 0, animated: false });
  });
});
