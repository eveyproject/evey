import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, beforeEach } from "vitest";
import { useRecentSearches } from "./useRecentSearches";

describe("useRecentSearches", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("starts empty when localStorage is empty", () => {
    const { result } = renderHook(() => useRecentSearches());
    expect(result.current.recents).toEqual([]);
  });

  it("push adds a query to recents", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("batman"); });
    expect(result.current.recents).toContain("batman");
  });

  it("push deduplicates and moves query to front", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("batman"); });
    act(() => { result.current.push("superman"); });
    act(() => { result.current.push("batman"); });
    expect(result.current.recents[0]).toBe("batman");
    expect(result.current.recents.filter((q) => q === "batman")).toHaveLength(1);
  });

  it("push trims whitespace and ignores empty strings", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("   "); });
    expect(result.current.recents).toHaveLength(0);
    act(() => { result.current.push("  action  "); });
    expect(result.current.recents[0]).toBe("action");
  });

  it("remove deletes a specific query", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("batman"); });
    act(() => { result.current.push("superman"); });
    act(() => { result.current.remove("batman"); });
    expect(result.current.recents).not.toContain("batman");
    expect(result.current.recents).toContain("superman");
  });

  it("clear empties all recents", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("batman"); });
    act(() => { result.current.push("superman"); });
    act(() => { result.current.clear(); });
    expect(result.current.recents).toHaveLength(0);
  });

  it("persists to localStorage on push", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => { result.current.push("batman"); });
    const stored = JSON.parse(localStorage.getItem("evey-recent-searches") ?? "[]");
    expect(stored).toContain("batman");
  });

  it("loads from localStorage on mount", () => {
    localStorage.setItem("evey-recent-searches", JSON.stringify(["cached"]));
    const { result } = renderHook(() => useRecentSearches());
    expect(result.current.recents).toContain("cached");
  });

  it("caps at 8 entries", () => {
    const { result } = renderHook(() => useRecentSearches());
    act(() => {
      for (let i = 1; i <= 10; i++) {
        result.current.push(`query${i}`);
      }
    });
    expect(result.current.recents).toHaveLength(8);
  });
});
