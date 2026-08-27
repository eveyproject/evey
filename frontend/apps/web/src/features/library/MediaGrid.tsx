import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";

interface MediaGridProps {
  children: ReactNode;
  isLoading?: boolean;
  onEndReached?: () => void;
}

// AuthLayout puts <main id="main-content"> as the scrollable region
// (overflow-y-auto) because the body itself is locked to viewport
// (h-screen overflow-hidden). The virtualizer's getScrollElement expects
// the actual scroll parent - never the window, which can't scroll here.
// Lookup is done client-side post-mount; null during SSR / first paint,
// which falls back to a skeleton (graceful).
function useMainScrollParent(): HTMLElement | null {
  const [el, setEl] = useState<HTMLElement | null>(null);
  useEffect(() => {
    setEl(document.getElementById("main-content"));
  }, []);
  return el;
}

function SkeletonCard() {
  return (
    <div className="w-full aspect-[2/3]">
      <div
        className="w-full h-full animate-pulse bg-[var(--surface-raised)] rounded-[var(--radius-md)]"
      />
      <div
        className="mt-2 h-4 w-3/4 animate-pulse bg-[var(--surface-raised)] rounded-[var(--radius-sm)]"
      />
    </div>
  );
}

// Auto-fill tracks rather than a fixed column count per breakpoint.
//
// MediaCard has a FIXED width (w-40 sm:w-48). With fixed counts the cell width
// is (viewport / count), which at some widths lands NARROWER than the card -
// e.g. 4 columns at 844px gives 171px cells for a 192px card, and the card
// overflowed its own cell. Deriving the count from the card width instead makes
// that impossible by construction, whatever the card grows to next.
const GRID_CLASS =
  "evey-media-grid grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] sm:grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-4 justify-items-start";

const GRID_GAP = 16; // gap-4
const GRID_MIN_CELL = 160; // 10rem, matches MediaCard's base w-40
const GRID_MIN_CELL_SM = 192; // 12rem, matches MediaCard's sm:w-48

// Same breakpoint the CSS grid used to switch minmax(10rem, ...) to
// minmax(12rem, ...) at - reproduced so the row chunking tracks the same
// column count the auto-fill grid rendered before virtualization.
// window.innerWidth + resize (not matchMedia - jsdom's matchMedia throws
// "Not implemented" without a polyfill, see utils/codec.ts's canDisplayHdr).
function useIsSmViewport(): boolean {
  const [isSm, setIsSm] = useState(() => typeof window !== "undefined" && window.innerWidth >= 640);
  useEffect(() => {
    const onResize = () => setIsSm(window.innerWidth >= 640);
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);
  return isSm;
}

// Reproduces CSS grid auto-fill's column count for a track of fixed minSize
// cells separated by `gap`: how many (minSize + gap) slots fit in the
// container, plus the one that doesn't need a trailing gap.
function computeCols(containerWidth: number, minCell: number): number {
  if (containerWidth <= 0) return 1;
  return Math.max(1, Math.floor((containerWidth + GRID_GAP) / (minCell + GRID_GAP)));
}

export function MediaGrid({ children, isLoading, onEndReached }: MediaGridProps) {
  const items = useMemo(
    () => (Array.isArray(children) ? children : children ? [children] : []),
    [children],
  );
  const scrollParent = useMainScrollParent();
  const parentRef = useRef<HTMLDivElement>(null);
  // Guess a reasonable width before the ResizeObserver's first callback lands,
  // same as PhotoGrid (routes/_auth/photos/index.lazy.tsx) - avoids a one-tick
  // flash of a single column.
  const [containerWidth, setContainerWidth] = useState(800);
  const isSm = useIsSmViewport();

  // Re-attach once the scrollParent/isLoading gate flips and the real grid
  // (with parentRef assigned) actually mounts.
  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    const onResize: ResizeObserverCallback = (entries) => {
      const w = entries[0]?.contentRect.width;
      if (w) setContainerWidth(w);
    };
    const ro = new ResizeObserver(onResize);
    ro.observe(el);
    setContainerWidth(el.clientWidth);
    return () => ro.disconnect();
  }, [scrollParent, isLoading]);

  const minCell = isSm ? GRID_MIN_CELL_SM : GRID_MIN_CELL;
  const cols = computeCols(containerWidth, minCell);

  const rows = useMemo(() => {
    const result: ReactNode[][] = [];
    for (let i = 0; i < items.length; i += cols) {
      result.push(items.slice(i, i + cols));
    }
    return result;
  }, [items, cols]);

  // Height estimate only - poster (aspect-[2/3]) plus caption below it.
  // Corrected per row by measureElement since caption content (and thus
  // real row height) isn't known to this generic container.
  const estimatedRowHeight = minCell * 1.5 + 56;

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollParent,
    estimateSize: () => estimatedRowHeight,
    overscan: 3,
    scrollMargin:
      parentRef.current && scrollParent
        ? parentRef.current.getBoundingClientRect().top - scrollParent.getBoundingClientRect().top + scrollParent.scrollTop
        : 0,
  });

  const virtualRows = virtualizer.getVirtualItems();
  const lastRenderedRow = virtualRows.length > 0 ? virtualRows[virtualRows.length - 1].index : -1;

  // Ask for the next page as soon as the last built row is rendered. The
  // virtualizer's overscan means that happens a few rows before the user sees
  // the bottom, so the grid keeps filling ahead of the scroll (mirrors PhotoGrid).
  useEffect(() => {
    if (rows.length === 0 || lastRenderedRow < rows.length - 1) return;
    onEndReached?.();
  }, [lastRenderedRow, rows.length, onEndReached]);

  if (isLoading) {
    return (
      <div className={GRID_CLASS}>
        {Array.from({ length: 12 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  // Defer mount until scrollParent is resolved. Mounting the virtualizer
  // with getScrollElement returning null then swapping to the real element
  // leaves it stale (onEndReached stops firing, items get clipped). A
  // skeleton placeholder covers the single-tick delay before AuthLayout's
  // main-content ref resolves.
  if (!scrollParent) {
    return (
      <div className={GRID_CLASS}>
        {Array.from({ length: 12 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  return (
    <div ref={parentRef}>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualRows.map((vRow) => (
          <div
            key={vRow.key}
            data-index={vRow.index}
            ref={virtualizer.measureElement}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              transform: `translateY(${vRow.start - virtualizer.options.scrollMargin}px)`,
            }}
          >
            <div
              className="grid gap-4 justify-items-start pb-4"
              style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
            >
              {rows[vRow.index].map((child, colIdx) => (
                // Callers of MediaGrid don't have to key their children (they
                // never did - the old itemContent(index) API rendered one at
                // a time too); the Fragment key is only to satisfy React's
                // array-render requirement for this row-local slice.
                <Fragment key={colIdx}>{child}</Fragment>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
