import React, { useMemo, type ReactNode } from "react";
import { VirtuosoGrid } from "react-virtuoso";

interface MediaGridProps {
  children: ReactNode;
  isLoading?: boolean;
  onEndReached?: () => void;
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
      useWindowScroll
      totalCount={items.length}
      overscan={400}
      components={gridComponents}
      itemContent={(index) => items[index]}
      endReached={onEndReached}
    />
  );
}
