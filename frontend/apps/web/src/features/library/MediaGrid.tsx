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
// expects the window to scroll — which can never happen here. We hand
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

const GRID_CLASS = "grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4";

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

  return (
    <VirtuosoGrid
      customScrollParent={scrollParent ?? undefined}
      totalCount={items.length}
      overscan={200}
      components={gridComponents}
      itemContent={(index) => items[index]}
      endReached={onEndReached}
    />
  );
}
