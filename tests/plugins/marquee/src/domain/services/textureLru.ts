// Bounded texture budget: at most `capacity` poster textures are ever
// resident (WebGL textures are the expensive resource here, not the JS
// objects). `touch` marks a key as most-recently-used and reports the key
// that must be evicted (if any) so the render adapter can delete the
// corresponding GL texture. Pure bookkeeping - no GL, no image decoding.

export interface Lru<K> {
  // Marks `key` as most-recently-used. Returns the evicted key (least
  // recently used) if adding it pushed the cache past capacity, else null.
  touch(key: K): K | null;
  has(key: K): boolean;
  size(): number;
  // Oldest first, most-recently-used last.
  keys(): K[];
  remove(key: K): void;
}

export function createLru<K>(capacity: number): Lru<K> {
  const cap = Math.max(1, Math.floor(capacity));
  const order: K[] = [];

  const indexOf = (key: K): number => order.indexOf(key);

  return {
    touch(key: K): K | null {
      const idx = indexOf(key);
      if (idx !== -1) {
        order.splice(idx, 1);
        order.push(key);
        return null;
      }
      order.push(key);
      if (order.length > cap) {
        return order.shift() as K;
      }
      return null;
    },
    has(key: K): boolean {
      return indexOf(key) !== -1;
    },
    size(): number {
      return order.length;
    },
    keys(): K[] {
      return order.slice();
    },
    remove(key: K): void {
      const idx = indexOf(key);
      if (idx !== -1) order.splice(idx, 1);
    },
  };
}
