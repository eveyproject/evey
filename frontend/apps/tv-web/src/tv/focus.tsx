import { useCallback, useEffect, useRef, type ReactNode } from "react";
import { init, setKeyMap, useFocusable, FocusContext, setFocus, getCurrentFocusKey } from "@noriginmedia/norigin-spatial-navigation";
import { tvKeyOf, targetIsEditable } from "./keys";

// Spatial navigation: the library decides which focusable is "to the right"
// of the current one from geometry, so screens are built out of ordinary
// boxes and the remote walks them. Initialised once, before the first
// render. The key map feeds it the codes the television browsers send
// (see keys.ts) so webOS 6's keyCode-only events still move the focus.
let initialised = false;
export function initTvNavigation(): void {
  if (initialised) return;
  initialised = true;
  init({ debug: false, visualDebug: false, throttle: 0 });
  setKeyMap({
    left: [37],
    up: [38],
    right: [39],
    down: [40],
    enter: [13],
  });
}

export { useFocusable, FocusContext, setFocus, getCurrentFocusKey };

/**
 * Screen-level "back" handling. Focus movement and "select" belong to the
 * spatial library; back (Escape, the TV's back button) and the media keys
 * are the app's, and a text field keeps its own keys except back.
 */
export function useTvKeys(handler: (key: "back" | "play" | "pause" | "playPause" | "rewind" | "forward" | "stop" | "search", e: KeyboardEvent) => void): void {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const key = tvKeyOf(e);
      if (!key) return;
      if (key === "left" || key === "right" || key === "up" || key === "down" || key === "select") return;
      if (targetIsEditable(e.target) && key !== "back") return;
      e.preventDefault();
      handler(key, e);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handler]);
}

interface FocusRootProps {
  focusKey?: string;
  /** Focus the first child as soon as the screen mounts. */
  autoFocus?: boolean;
  className?: string;
  /**
   * This scope is a CSS grid of equal cards: the next focus is resolved by
   * arithmetic instead of by measuring every child.
   *
   * Measured on a three hundred title library: the coordinate search costs
   * about nine style recalculations per key press against one on a three
   * card library, because each move measures every focusable in the scope.
   * A grid's geometry is known - the column count and the order - so the
   * answer is an index away. Returning nothing at an edge lets the library
   * escalate to the parent, which is how the remote still leaves the grid.
   */
  grid?: boolean;
  children: ReactNode;
}

/** A focus scope: what the remote can reach on this screen, or in this row. */
export function FocusRoot({ focusKey, autoFocus, className, grid, children }: FocusRootProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const gridResolver = useCallback((direction: string, currentKey: string, siblings: { focusKey: string }[]) => {
    const index = siblings.findIndex((s) => s.focusKey === currentKey);
    if (index < 0) return null;
    const columns = gridColumnCount(containerRef.current);
    const column = index % columns;
    let target: number;
    if (direction === "right") target = column === columns - 1 ? -1 : index + 1;
    else if (direction === "left") target = column === 0 ? -1 : index - 1;
    else if (direction === "down") target = index + columns;
    else if (direction === "up") target = index - columns;
    else return null;
    // Past the last row, the last card is still the honest answer: a partial
    // final row must not swallow the press.
    if (direction === "down" && target >= siblings.length && index < siblings.length - 1) target = siblings.length - 1;
    if (target < 0 || target >= siblings.length) return null;
    return siblings[target] as never;
  }, []);

  const { ref, focusKey: key, focusSelf } = useFocusable({
    focusKey,
    trackChildren: true,
    saveLastFocusedChild: true,
    ...(grid ? { measureChildrenLayout: false, nextFocusResolver: gridResolver } : {}),
  });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <FocusContext.Provider value={key}>
      <div
        ref={(node: HTMLDivElement | null) => {
          containerRef.current = node;
          (ref as unknown as { current: HTMLDivElement | null }).current = node;
        }}
        className={className}
      >
        {children}
      </div>
    </FocusContext.Provider>
  );
}

/**
 * How many cards a row of this grid holds, read from the stylesheet rather
 * than from every child's box: one question to the engine instead of N.
 */
function gridColumnCount(container: HTMLElement | null): number {
  const grid = container?.querySelector(".grid") ?? container;
  if (!grid) return 1;
  const columns = getComputedStyle(grid).gridTemplateColumns;
  if (!columns || columns === "none") return 1;
  return Math.max(1, columns.split(" ").filter(Boolean).length);
}
