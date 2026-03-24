import { useRef, useState, useEffect, useMemo, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";

interface MediaGridProps {
  children: ReactNode;
  isLoading?: boolean;
}

function SkeletonCard() {
  return (
    <div className="w-36" style={{ aspectRatio: "2/3" }}>
      <div
        className="w-full h-full animate-pulse"
        style={{
          backgroundColor: "var(--surface-raised)",
          borderRadius: "var(--radius-md)",
        }}
      />
      <div
        className="mt-2 h-4 w-3/4 animate-pulse"
        style={{ backgroundColor: "var(--surface-raised)", borderRadius: "var(--radius-sm)" }}
      />
    </div>
  );
}

const COL_BREAKPOINTS = [
  { minWidth: 1280, cols: 6 },
  { minWidth: 1024, cols: 5 },
  { minWidth: 768,  cols: 4 },
  { minWidth: 640,  cols: 3 },
  { minWidth: 0,    cols: 2 },
];

const GAP = 16;
const CARD_TITLE_HEIGHT = 28;

function getColCount(width: number): number {
  for (const bp of COL_BREAKPOINTS) {
    if (width >= bp.minWidth) return bp.cols;
  }
  return 2;
}

function getRowHeight(containerWidth: number, cols: number): number {
  const cardWidth = (containerWidth - GAP * (cols - 1)) / cols;
  const cardHeight = (cardWidth * 3) / 2;
  return cardHeight + CARD_TITLE_HEIGHT + GAP;
}

interface VirtualGridProps {
  items: ReactNode[];
}

function VirtualGrid({ items }: VirtualGridProps) {
  const parentRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(800);

  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width;
      if (width) setContainerWidth(width);
    });
    ro.observe(el);
    setContainerWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const cols = getColCount(containerWidth);
  const rowHeight = getRowHeight(containerWidth, cols);

  const rows = useMemo(() => {
    const result: ReactNode[][] = [];
    for (let i = 0; i < items.length; i += cols) {
      result.push(items.slice(i, i + cols));
    }
    return result;
  }, [items, cols]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => document.scrollingElement as HTMLElement,
    estimateSize: () => rowHeight,
    overscan: 3,
    scrollMargin: parentRef.current?.offsetTop ?? 0,
  });

  return (
    <div ref={parentRef}>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((vRow) => (
          <div
            key={vRow.index}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              transform: `translateY(${vRow.start - virtualizer.options.scrollMargin}px)`,
              height: rowHeight,
            }}
            className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4"
          >
            {rows[vRow.index]}
          </div>
        ))}
      </div>
    </div>
  );
}

export function MediaGrid({ children, isLoading }: MediaGridProps) {
  const items = useMemo(
    () => (Array.isArray(children) ? children : children ? [children] : []),
    [children],
  );

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
        {Array.from({ length: 12 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  return <VirtualGrid items={items} />;
}
