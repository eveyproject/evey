import { useCallback, useRef, useSyncExternalStore, type DragEvent as ReactDragEvent, type PointerEvent as ReactPointerEvent } from "react";

// Drag and drop over pointer events, for the music screens: a track, an album
// or a playlist dragged onto a queue, and the rows of a queue reordered. The
// browser's own HTML5 drag and drop has no touch support at all, so it would
// work with a mouse and do nothing on a tablet.
//
// A mouse drag starts once the pointer has moved a few pixels, so a plain
// click still clicks. A touch drag starts on a long press: a finger that moves
// before that is scrolling the page, and the page scrolls.
//
// A drop zone is any element carrying data-drop-zone, registered with
// useDropZone. Inside it, the entries that give a position carry
// data-drop-index; the drop lands before the entry under the pointer, or after
// it when the pointer is on its lower half.

export interface DragState {
  active: boolean;
  data: unknown;
  sourceId: string | null;
  // The zone under the pointer that accepts the dragged data, and where in it
  // the drop would land (an insertion index from 0 to the entry count).
  overZone: string | null;
  overIndex: number | null;
}

interface ZoneEntry {
  el: HTMLElement;
  accepts: (data: unknown) => boolean;
  onDrop: (data: unknown, index: number | null) => void;
}

const IDLE: DragState = { active: false, data: null, sourceId: null, overZone: null, overIndex: null };
const MOUSE_THRESHOLD = 5;
const TOUCH_SLOP = 8;
const LONG_PRESS_MS = 300;
const EDGE = 40;
const SCROLL_STEP = 12;

const zones = new Map<string, ZoneEntry>();
let state: DragState = IDLE;
const listeners = new Set<() => void>();

function setState(patch: Partial<DragState>) {
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => { listeners.delete(l); };
}

function getState() {
  return state;
}

/** The current drag, for screens that show where things can be dropped. */
export function useDragState(): DragState {
  return useSyncExternalStore(subscribe, getState, () => IDLE);
}

/** Where the zone under (x, y) would take the dragged data, if anywhere. */
function hitTest(x: number, y: number, data: unknown): { zone: string; entry: ZoneEntry; index: number | null } | null {
  const el = document.elementFromPoint(x, y);
  const zoneEl = el?.closest<HTMLElement>("[data-drop-zone]");
  if (!zoneEl) return null;
  const id = zoneEl.getAttribute("data-drop-zone") ?? "";
  const entry = zones.get(id);
  if (!entry || !entry.accepts(data)) return null;
  const item = el?.closest<HTMLElement>("[data-drop-index]");
  if (item && zoneEl.contains(item)) {
    const i = Number(item.getAttribute("data-drop-index"));
    if (!Number.isNaN(i)) {
      const r = item.getBoundingClientRect();
      return { zone: id, entry, index: y > r.top + r.height / 2 ? i + 1 : i };
    }
  }
  return { zone: id, entry, index: null };
}

/** The scrollable element holding the zone under the pointer, if near its edge. */
function autoScroll(x: number, y: number) {
  const el = document.elementFromPoint(x, y);
  let node = el?.closest<HTMLElement>("[data-drop-zone]") ?? null;
  while (node && node !== document.body) {
    if (node.scrollHeight > node.clientHeight + 1) {
      const r = node.getBoundingClientRect();
      if (y < r.top + EDGE) node.scrollTop -= SCROLL_STEP;
      else if (y > r.bottom - EDGE) node.scrollTop += SCROLL_STEP;
      return;
    }
    node = node.parentElement;
  }
}

function makeGhost(label: string): HTMLDivElement {
  const g = document.createElement("div");
  g.textContent = label;
  g.setAttribute("aria-hidden", "true");
  Object.assign(g.style, {
    position: "fixed",
    left: "0",
    top: "0",
    zIndex: "10000",
    pointerEvents: "none",
    maxWidth: "260px",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    padding: "6px 10px",
    fontSize: "0.8rem",
    borderRadius: "var(--radius-sm)",
    border: "1px solid var(--accent)",
    background: "var(--surface-raised)",
    color: "var(--text)",
    boxShadow: "0 6px 18px rgba(0,0,0,0.35)",
  } satisfies Partial<CSSStyleDeclaration>);
  document.body.appendChild(g);
  return g;
}

function swallowNextClick() {
  const stop = (e: Event) => { e.preventDefault(); e.stopPropagation(); };
  window.addEventListener("click", stop, { capture: true, once: true });
  setTimeout(() => window.removeEventListener("click", stop, { capture: true }), 0);
}

function startGesture(e: ReactPointerEvent, sourceId: string, getData: () => unknown, getLabel: () => string) {
  if (e.button !== 0 || state.active) return;
  const target = e.target as HTMLElement | null;
  if (target?.closest("input, select, textarea, [data-no-drag]")) return;

  const pointerId = e.pointerId;
  const isTouch = e.pointerType !== "mouse";
  const startX = e.clientX;
  const startY = e.clientY;
  let lastX = startX;
  let lastY = startY;
  let started = false;
  let ghost: HTMLDivElement | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let raf = 0;
  const prevUserSelect = document.body.style.userSelect;

  const place = () => {
    if (ghost) ghost.style.transform = `translate(${lastX + 14}px, ${lastY + 14}px)`;
    const hit = hitTest(lastX, lastY, state.data);
    if (hit?.zone !== state.overZone || (hit?.index ?? null) !== state.overIndex) {
      setState({ overZone: hit?.zone ?? null, overIndex: hit?.index ?? null });
    }
  };

  const tick = () => {
    if (!started) return;
    autoScroll(lastX, lastY);
    place();
    raf = requestAnimationFrame(tick);
  };

  const begin = () => {
    if (started) return;
    started = true;
    document.body.style.userSelect = "none";
    ghost = makeGhost(getLabel());
    setState({ active: true, data: getData(), sourceId, overZone: null, overIndex: null });
    place();
    raf = requestAnimationFrame(tick);
    if (isTouch) navigator.vibrate?.(10);
  };

  const cleanup = () => {
    clearTimeout(timer);
    cancelAnimationFrame(raf);
    window.removeEventListener("pointermove", onMove);
    window.removeEventListener("pointerup", onUp);
    window.removeEventListener("pointercancel", onCancel);
    window.removeEventListener("touchmove", onTouchMove);
    window.removeEventListener("contextmenu", onContextMenu, true);
    ghost?.remove();
    ghost = null;
    if (started) {
      document.body.style.userSelect = prevUserSelect;
      setState(IDLE);
    }
  };

  function onMove(ev: PointerEvent) {
    if (ev.pointerId !== pointerId) return;
    lastX = ev.clientX;
    lastY = ev.clientY;
    if (!started) {
      const dist = Math.hypot(lastX - startX, lastY - startY);
      if (isTouch) {
        if (dist > TOUCH_SLOP) cleanup();
      } else if (dist > MOUSE_THRESHOLD) {
        begin();
      }
      return;
    }
    place();
  }

  function onUp(ev: PointerEvent) {
    if (ev.pointerId !== pointerId) return;
    if (started) {
      lastX = ev.clientX;
      lastY = ev.clientY;
      const data = state.data;
      const hit = hitTest(lastX, lastY, data);
      swallowNextClick();
      cleanup();
      hit?.entry.onDrop(data, hit.index);
      return;
    }
    cleanup();
  }

  function onCancel(ev: PointerEvent) {
    if (ev.pointerId === pointerId) cleanup();
  }

  // Once a touch drag has started, the finger moves the ghost, not the page.
  function onTouchMove(ev: TouchEvent) {
    if (started) ev.preventDefault();
  }

  // A long press opens the context menu on Android.
  function onContextMenu(ev: Event) {
    ev.preventDefault();
  }

  window.addEventListener("pointermove", onMove);
  window.addEventListener("pointerup", onUp);
  window.addEventListener("pointercancel", onCancel);
  if (isTouch) {
    window.addEventListener("touchmove", onTouchMove, { passive: false });
    window.addEventListener("contextmenu", onContextMenu, true);
    timer = setTimeout(begin, LONG_PRESS_MS);
  }
}

/**
 * Props that make an element draggable. `getData` is read when the drag
 * starts, `label` is what the ghost under the pointer says.
 */
export function useDraggable(id: string, getData: () => unknown, label: string | (() => string), disabled = false) {
  const dataRef = useRef(getData);
  const labelRef = useRef(label);
  dataRef.current = getData;
  labelRef.current = label;
  const dragging = useSyncExternalStore(subscribe, () => state.active && state.sourceId === id, () => false);
  const onPointerDown = useCallback(
    (e: ReactPointerEvent) => {
      if (disabled) return;
      startGesture(
        e,
        id,
        () => dataRef.current(),
        () => (typeof labelRef.current === "function" ? labelRef.current() : labelRef.current),
      );
    },
    [id, disabled],
  );
  return {
    dragging,
    props: {
      onPointerDown,
      // The browser's own image and link dragging would take over a mouse drag.
      onDragStart: (e: ReactDragEvent) => e.preventDefault(),
    },
  };
}

/**
 * Registers an element as a drop zone. Spread `props` on it and pass `ref`.
 * `over` is true while an accepted drag hovers it, `index` where it would land
 * (null: at the end), `armed` while any accepted drag is under way.
 */
export function useDropZone<T>(
  id: string,
  accepts: (data: unknown) => data is T,
  onDrop: (data: T, index: number | null) => void,
) {
  const elRef = useRef<HTMLElement | null>(null);
  const acceptsRef = useRef(accepts);
  const dropRef = useRef(onDrop);
  acceptsRef.current = accepts;
  dropRef.current = onDrop;

  const ref = useCallback(
    (el: HTMLElement | null) => {
      const prev = elRef.current;
      if (prev && zones.get(id)?.el === prev) zones.delete(id);
      elRef.current = el;
      if (el) {
        zones.set(id, {
          el,
          accepts: (d) => acceptsRef.current(d),
          onDrop: (d, i) => dropRef.current(d as T, i),
        });
      }
    },
    [id],
  );

  const s = useDragState();
  const over = s.active && s.overZone === id;
  return {
    ref,
    props: { "data-drop-zone": id },
    over,
    index: over ? s.overIndex : null,
    armed: s.active && acceptsRef.current(s.data),
  };
}

/**
 * The index an entry dragged from `from` ends up at when dropped at insertion
 * position `insertAt` of the same list.
 */
export function moveTargetIndex(from: number, insertAt: number): number {
  return insertAt > from ? insertAt - 1 : insertAt;
}
