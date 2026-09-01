// Pure edits of the home feed configuration list, kept out of the modal so
// the persist step can be reasoned about: a click persists ONLY the rows it
// changed, from the value computed in the handler - never from inside a
// setState updater, which React StrictMode runs twice (audit 2026-08-25:
// every toggle sent its PUTs twice, and every toggle sent one PUT per feed).

export interface FeedConfigRow {
  feedType?: string;
  enabled?: boolean;
  displayOrder?: number;
}

export function toggleFeed<T extends FeedConfigRow>(list: T[], idx: number): T[] {
  return list.map((c, i) => (i === idx ? { ...c, enabled: !c.enabled } : c));
}

/** Swap with the neighbour and renumber; the same list when the move is off the ends. */
export function moveFeed<T extends FeedConfigRow>(list: T[], idx: number, dir: -1 | 1): T[] {
  const target = idx + dir;
  if (target < 0 || target >= list.length) return list;
  const arr = [...list];
  [arr[idx], arr[target]] = [arr[target], arr[idx]];
  return arr.map((c, i) => ({ ...c, displayOrder: i }));
}

/** The rows of `next` whose persisted fields differ from their `prev` counterpart (matched by feedType). */
export function changedFeeds<T extends FeedConfigRow>(prev: T[], next: T[]): T[] {
  const before = new Map(prev.map((c) => [c.feedType, c]));
  return next.filter((c) => {
    if (!c.feedType) return false;
    const b = before.get(c.feedType);
    return !b || b.enabled !== c.enabled || b.displayOrder !== c.displayOrder;
  });
}
