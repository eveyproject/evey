import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
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
  // The library reads keyCode || code || key: a browser that sends the arrows
  // with keyCode 0 (LG's own browser, Magic Remote) hands it "ArrowUp", which
  // a numbers-only map ignored while back and play still worked.
  setKeyMap({
    left: [37, "ArrowLeft", "Left"],
    up: [38, "ArrowUp", "Up"],
    right: [39, "ArrowRight", "Right"],
    down: [40, "ArrowDown", "Down"],
    enter: [13, "Enter", "NumpadEnter"],
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

/** Back taken by an overlay while it is up, before the screen's own handler. */
export function useBackHandler(handler: (() => void) | null): void {
  useEffect(() => {
    if (!handler) return;
    const onKey = (e: KeyboardEvent) => {
      // A field being typed in takes BACK first: it only leaves the field.
      if (tvKeyOf(e) !== "back" || targetIsEditable(e.target)) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      handler();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [handler]);
}

// Where the focus stood on each screen the viewer left, by hash: going back
// lands on the card that was left, as the Android television does. Only a
// key a screen gave on purpose is kept - the library's generated keys
// ("sn:...") change at every mount and would point at nothing.
const leftFocus = new Map<string, string>();

/** Called by the router just before it leaves the current screen. */
export function rememberFocusBeforeLeaving(hash: string): void {
  const key = getCurrentFocusKey();
  if (key && !key.startsWith("sn:")) leftFocus.set(hash, key);
  else leftFocus.delete(hash);
}

/**
 * What a screen's focusables need to agree on who takes the focus on arrival:
 * the key to restore when coming back, and whether a child already claimed it.
 * The screen's own scope used to focus itself AFTER its children, and the
 * library then picked the child closest to the top-left corner - the header,
 * the track row - over the card or the Play button the screen had focused.
 */
interface ScreenFocus {
  restoreKey: string | null;
  claimed: { current: boolean };
  restored: { current: boolean };
}
const ScreenFocusContext = createContext<ScreenFocus | null>(null);

/**
 * The focus a focusable takes when it mounts: the one the viewer left, when
 * coming back to this screen, else the one the screen asked for (autoFocus).
 */
export function useArrivalFocus(focusKey: string | undefined, autoFocus: boolean | undefined, focusSelf: () => void): void {
  const screen = useContext(ScreenFocusContext);
  useEffect(() => {
    if (screen?.restoreKey && focusKey === screen.restoreKey) {
      screen.restored.current = true;
      screen.claimed.current = true;
      focusSelf();
      return;
    }
    if (!autoFocus || screen?.restoreKey) return;
    if (screen) screen.claimed.current = true;
    focusSelf();
  }, [autoFocus, focusKey, focusSelf, screen]);
}

// How long a screen waits for the card it should restore (its row may still
// be loading) before settling for its default focus.
const RESTORE_WAIT_MS = 400;

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
  /**
   * The child scope to land on when this scope takes the focus. Without it
   * the library picks the child closest to the top-left corner, which on a
   * title page is the track row under the poster, not the Play button.
   */
  preferredChildFocusKey?: string;
  /** A modal: the remote cannot walk out of it. */
  boundary?: boolean;
  children: ReactNode;
}

/** A focus scope: what the remote can reach on this screen, or in this row. */
export function FocusRoot({ focusKey, autoFocus, className, grid, preferredChildFocusKey, boundary, children }: FocusRootProps) {
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
    preferredChildFocusKey,
    isFocusBoundary: !!boundary,
    ...(grid ? { measureChildrenLayout: false, nextFocusResolver: gridResolver } : {}),
  });
  // A screen scope (autoFocus) owns the arrival: it restores the focus the
  // viewer left here, or leaves it to the child that claimed it, and only
  // focuses itself when nobody did.
  const screen = useMemo<ScreenFocus | null>(
    () => (autoFocus ? { restoreKey: leftFocus.get(window.location.hash) ?? null, claimed: { current: false }, restored: { current: false } } : null),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- decided once, at mount
    [],
  );
  useEffect(() => {
    if (!autoFocus || !screen) return;
    if (screen.restoreKey) {
      const timer = setTimeout(() => { if (!screen.restored.current) focusSelf(); }, RESTORE_WAIT_MS);
      return () => clearTimeout(timer);
    }
    if (!screen.claimed.current) focusSelf();
  }, [autoFocus, focusSelf, screen]);
  const body = (
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
  return screen ? <ScreenFocusContext.Provider value={screen}>{body}</ScreenFocusContext.Provider> : body;
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
