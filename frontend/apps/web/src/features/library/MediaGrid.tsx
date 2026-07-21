import React, { useEffect, useMemo, useState, type ReactNode } from "react";
import { VirtuosoGrid } from "react-virtuoso";

interface MediaGridProps {
  children: ReactNode;
  isLoading?: boolean;
  onEndReached?: () => void;
}

// AuthLayout puts <main id="main-content"> as the scrollable region
// (overflow-y-auto) because the body itself is locked to viewport
// (h-screen overflow-hidden). VirtuosoGrid's useWindowScroll mode
// expects the window to scroll - which can never happen here. We hand
// it the actual scroll parent instead so endReached fires properly and
// all virtualized cards become reachable. Lookup is done client-side
// post-mount; null during SSR / first paint, which falls back to
// VirtuosoGrid's default behavior (graceful).
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

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const gridComponents: any = {
  List: React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
    (props, ref) => (
      <div ref={ref} {...props} className={GRID_CLASS} />
    ),
  ),
  Item: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement> & { children?: ReactNode }) => (
    <div {...props}>{children}</div>
  ),
};

export function MediaGrid({ children, isLoading, onEndReached }: MediaGridProps) {
  const items = useMemo(
    () => (Array.isArray(children) ? children : children ? [children] : []),
    [children],
  );
  const scrollParent = useMainScrollParent();

  if (isLoading) {
    return (
      <div className={GRID_CLASS}>
        {Array.from({ length: 12 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  // Defer mount until scrollParent is resolved. Mounting VirtuosoGrid
  // with customScrollParent=undefined then swapping to the real element
  // leaves react-virtuoso's internal scroller stale (endReached stops
  // firing, items get clipped). A skeleton placeholder covers the
  // single-tick delay before AuthLayout's main-content ref resolves.
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
    <VirtuosoGrid
      customScrollParent={scrollParent}
      totalCount={items.length}
      overscan={600}
      components={gridComponents}
      itemContent={(index) => items[index]}
      endReached={onEndReached}
    />
  );
}
