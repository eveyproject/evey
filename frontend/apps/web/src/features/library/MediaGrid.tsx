import type { ReactNode } from "react";

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

export function MediaGrid({ children, isLoading }: MediaGridProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
        {Array.from({ length: 12 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
      {children}
    </div>
  );
}
