import { describe, it, expect } from "vitest";
import { createLru } from "../src/domain/services/textureLru";

describe("createLru", () => {
  it("does not evict while under capacity", () => {
    const lru = createLru<number>(3);
    expect(lru.touch(1)).toBeNull();
    expect(lru.touch(2)).toBeNull();
    expect(lru.size()).toBe(2);
  });

  it("evicts the least-recently-used key once capacity is exceeded", () => {
    const lru = createLru<number>(2);
    lru.touch(1);
    lru.touch(2);
    const evicted = lru.touch(3);
    expect(evicted).toBe(1);
    expect(lru.has(1)).toBe(false);
    expect(lru.has(2)).toBe(true);
    expect(lru.has(3)).toBe(true);
    expect(lru.size()).toBe(2);
  });

  it("re-touching an existing key promotes it to most-recently-used without evicting", () => {
    const lru = createLru<number>(2);
    lru.touch(1);
    lru.touch(2);
    expect(lru.touch(1)).toBeNull(); // 1 is now MRU, 2 is LRU
    const evicted = lru.touch(3);
    expect(evicted).toBe(2);
    expect(lru.has(1)).toBe(true);
  });

  it("never grows past capacity across many touches", () => {
    const lru = createLru<number>(32);
    for (let i = 0; i < 500; i++) lru.touch(i);
    expect(lru.size()).toBe(32);
  });

  it("keys() reports oldest-first order", () => {
    const lru = createLru<number>(3);
    lru.touch(1);
    lru.touch(2);
    lru.touch(3);
    expect(lru.keys()).toEqual([1, 2, 3]);
  });

  it("remove() drops a key without affecting others", () => {
    const lru = createLru<number>(3);
    lru.touch(1);
    lru.touch(2);
    lru.remove(1);
    expect(lru.has(1)).toBe(false);
    expect(lru.has(2)).toBe(true);
    expect(lru.size()).toBe(1);
  });

  it("clamps capacity to at least 1", () => {
    const lru = createLru<number>(0);
    lru.touch(1);
    const evicted = lru.touch(2);
    expect(evicted).toBe(1);
    expect(lru.size()).toBe(1);
  });
});
