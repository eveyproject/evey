/**
 * Moving one member of a collection one place up or down.
 *
 * Pure so the button wiring cannot get the arithmetic wrong: the server takes
 * the full new order, and an off-by-one there silently rewrites how a saga
 * reads for everyone. The web and the phone each kept a copy until 2026-09-24.
 */
export function moveItem(ids: string[], id: string, direction: "up" | "down"): string[] {
  const from = ids.indexOf(id);
  if (from < 0) return ids;
  const to = direction === "up" ? from - 1 : from + 1;
  if (to < 0 || to >= ids.length) return ids; // already at the edge
  const next = [...ids];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}
