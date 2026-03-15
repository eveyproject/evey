import { renderHook, act } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { useSelection } from "./useSelection";

describe("useSelection", () => {
  it("starts with empty selection", () => {
    const { result } = renderHook(() => useSelection<string>());
    expect(result.current.count).toBe(0);
    expect([...result.current.selected]).toEqual([]);
  });

  it("toggle adds an id not yet selected", () => {
    const { result } = renderHook(() => useSelection<string>());
    act(() => { result.current.toggle("a"); });
    expect(result.current.isSelected("a")).toBe(true);
    expect(result.current.count).toBe(1);
  });

  it("toggle removes an already-selected id", () => {
    const { result } = renderHook(() => useSelection<string>());
    act(() => { result.current.toggle("a"); });
    act(() => { result.current.toggle("a"); });
    expect(result.current.isSelected("a")).toBe(false);
    expect(result.current.count).toBe(0);
  });

  it("selectAll replaces selection with given ids", () => {
    const { result } = renderHook(() => useSelection<string>());
    act(() => { result.current.toggle("x"); });
    act(() => { result.current.selectAll(["a", "b", "c"]); });
    expect(result.current.count).toBe(3);
    expect(result.current.isSelected("a")).toBe(true);
    expect(result.current.isSelected("b")).toBe(true);
    expect(result.current.isSelected("c")).toBe(true);
    expect(result.current.isSelected("x")).toBe(false);
  });

  it("clear empties the selection", () => {
    const { result } = renderHook(() => useSelection<string>());
    act(() => { result.current.selectAll(["a", "b"]); });
    act(() => { result.current.clear(); });
    expect(result.current.count).toBe(0);
    expect(result.current.isSelected("a")).toBe(false);
  });

  it("isSelected returns false for non-selected id", () => {
    const { result } = renderHook(() => useSelection<string>());
    expect(result.current.isSelected("unknown")).toBe(false);
  });

  it("count reflects current selection size", () => {
    const { result } = renderHook(() => useSelection<string>());
    act(() => { result.current.toggle("a"); });
    act(() => { result.current.toggle("b"); });
    expect(result.current.count).toBe(2);
    act(() => { result.current.toggle("a"); });
    expect(result.current.count).toBe(1);
  });
});
