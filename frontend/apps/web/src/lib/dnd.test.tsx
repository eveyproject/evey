import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, fireEvent, act } from "@testing-library/react";
import { useDraggable, useDropZone, moveTargetIndex } from "./dnd";

type Data = { kind: "item"; id: string };
const isData = (d: unknown): d is Data => (d as Data)?.kind === "item";

function Source({ id }: { id: string }) {
  const drag = useDraggable(`src:${id}`, (): Data => ({ kind: "item", id }), `label ${id}`);
  return (
    <button type="button" data-testid={`src-${id}`} {...drag.props} data-dragging={drag.dragging ? "1" : "0"}>
      {id}
    </button>
  );
}

function Zone({ onDrop }: { onDrop: (d: Data, index: number | null) => void }) {
  const zone = useDropZone<Data>("zone", isData, onDrop);
  return (
    <ul ref={zone.ref} {...zone.props} data-testid="zone" data-over={zone.over ? "1" : "0"}>
      <li data-drop-index={0} data-testid="row-0">a</li>
      <li data-drop-index={1} data-testid="row-1">b</li>
    </ul>
  );
}

// jsdom lays nothing out: each row is given a 20px band, and the hit test is
// pointed at whatever the test says is under the pointer.
let under: Element | null = null;
beforeEach(() => {
  under = null;
  document.elementFromPoint = vi.fn(() => under);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const i = Number(this.getAttribute("data-drop-index") ?? 0);
    return { top: i * 20, bottom: i * 20 + 20, height: 20, left: 0, right: 100, width: 100, x: 0, y: i * 20, toJSON: () => ({}) } as DOMRect;
  });
});
afterEach(async () => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  // A finished drag swallows the click that follows it until the next task.
  await new Promise((r) => setTimeout(r, 0));
});

function pointer(type: string, x: number, y: number, pointerType = "mouse") {
  const ev = new Event(type, { bubbles: true }) as PointerEvent;
  Object.assign(ev, { clientX: x, clientY: y, pointerId: 1, pointerType, button: 0 });
  window.dispatchEvent(ev);
}

describe("dnd", () => {
  it("a mouse drag drops on the row under the pointer, before or after it by its half", () => {
    const onDrop = vi.fn();
    const { getByTestId } = render(<><Source id="x" /><Zone onDrop={onDrop} /></>);
    fireEvent.pointerDown(getByTestId("src-x"), { clientX: 0, clientY: 0, pointerId: 1, pointerType: "mouse", button: 0 });
    under = getByTestId("row-1");
    act(() => pointer("pointermove", 10, 25));
    expect(getByTestId("src-x").getAttribute("data-dragging")).toBe("1");
    expect(getByTestId("zone").getAttribute("data-over")).toBe("1");
    act(() => pointer("pointerup", 10, 35));
    expect(onDrop).toHaveBeenCalledWith({ kind: "item", id: "x" }, 2);
    expect(getByTestId("src-x").getAttribute("data-dragging")).toBe("0");
  });

  it("a click without movement stays a click and drops nothing", () => {
    const onDrop = vi.fn();
    const onClick = vi.fn();
    const { getByTestId } = render(<><Source id="x" /><Zone onDrop={onDrop} /></>);
    getByTestId("src-x").addEventListener("click", onClick);
    fireEvent.pointerDown(getByTestId("src-x"), { clientX: 0, clientY: 0, pointerId: 1, pointerType: "mouse", button: 0 });
    act(() => pointer("pointerup", 1, 1));
    fireEvent.click(getByTestId("src-x"));
    expect(onDrop).not.toHaveBeenCalled();
    expect(onClick).toHaveBeenCalled();
  });

  it("a drop outside any zone does nothing", () => {
    const onDrop = vi.fn();
    const { getByTestId } = render(<><Source id="x" /><Zone onDrop={onDrop} /></>);
    fireEvent.pointerDown(getByTestId("src-x"), { clientX: 0, clientY: 0, pointerId: 1, pointerType: "mouse", button: 0 });
    act(() => pointer("pointermove", 50, 50));
    under = document.body;
    act(() => pointer("pointerup", 50, 50));
    expect(onDrop).not.toHaveBeenCalled();
  });

  it("a finger that moves before the long press scrolls and never drags", () => {
    vi.useFakeTimers();
    const onDrop = vi.fn();
    const { getByTestId } = render(<><Source id="x" /><Zone onDrop={onDrop} /></>);
    fireEvent.pointerDown(getByTestId("src-x"), { clientX: 0, clientY: 0, pointerId: 1, pointerType: "touch", button: 0 });
    act(() => pointer("pointermove", 0, 30, "touch"));
    act(() => { vi.advanceTimersByTime(500); });
    expect(getByTestId("src-x").getAttribute("data-dragging")).toBe("0");
    under = getByTestId("row-0");
    act(() => pointer("pointerup", 0, 5, "touch"));
    expect(onDrop).not.toHaveBeenCalled();
  });

  it("a long press starts a touch drag", () => {
    vi.useFakeTimers();
    const onDrop = vi.fn();
    const { getByTestId } = render(<><Source id="x" /><Zone onDrop={onDrop} /></>);
    fireEvent.pointerDown(getByTestId("src-x"), { clientX: 0, clientY: 0, pointerId: 1, pointerType: "touch", button: 0 });
    act(() => { vi.advanceTimersByTime(350); });
    expect(getByTestId("src-x").getAttribute("data-dragging")).toBe("1");
    under = getByTestId("row-0");
    act(() => pointer("pointerup", 0, 5, "touch"));
    expect(onDrop).toHaveBeenCalledWith({ kind: "item", id: "x" }, 0);
  });

  it("moveTargetIndex turns an insertion point into the moved entry's new index", () => {
    expect(moveTargetIndex(0, 3)).toBe(2);
    expect(moveTargetIndex(3, 0)).toBe(0);
    expect(moveTargetIndex(1, 1)).toBe(1);
    expect(moveTargetIndex(1, 2)).toBe(1);
  });
});
