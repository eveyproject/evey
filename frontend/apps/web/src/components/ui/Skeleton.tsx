export function Skeleton({ className }: { className?: string }) {
  return (
    <div aria-busy="true" className={`animate-pulse rounded bg-[var(--surface-raised)] ${className ?? ""}`}>
      <span className="sr-only">Loading…</span>
    </div>
  );
}

export function MediaCardSkeleton() {
  return (
    <div className="w-40 flex-shrink-0">
      <div
        className="w-full animate-pulse aspect-[2/3] bg-[var(--surface-raised)] rounded-[var(--radius-md)]"
      />
      <div className="mt-2 px-0.5 space-y-1.5">
        <div
          className="h-3.5 w-4/5 animate-pulse rounded bg-[var(--surface-raised)]"
        />
        <div
          className="h-3 w-1/2 animate-pulse rounded bg-[var(--surface-raised)]"
        />
      </div>
    </div>
  );
}

export function MediaGridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div aria-busy="true" className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
      {Array.from({ length: count }).map((_, i) => (
        <MediaCardSkeleton key={i} />
      ))}
    </div>
  );
}

export function MediaDetailSkeleton() {
  return (
    <div aria-busy="true" className="animate-pulse">
      {/* Backdrop placeholder */}
      <div
        className="w-full -mx-6 -mt-6 mb-0 h-[420px] bg-[var(--surface)]"
      />
      {/* Content */}
      <div className="flex gap-6 mt-6">
        {/* Poster */}
        <div
          className="flex-shrink-0 w-48 hidden md:block aspect-[2/3] bg-[var(--surface-raised)] rounded-[var(--radius-md)]"
        />
        {/* Right side */}
        <div className="flex-1 space-y-4">
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-4 w-1/3" />
          <div className="space-y-2 pt-2">
            <Skeleton className="h-3.5 w-full" />
            <Skeleton className="h-3.5 w-full" />
            <Skeleton className="h-3.5 w-4/5" />
          </div>
          <div className="flex gap-3 pt-4">
            <Skeleton className="h-10 w-24 rounded-[var(--radius-md)]" />
            <Skeleton className="h-10 w-32 rounded-[var(--radius-md)]" />
          </div>
        </div>
      </div>
    </div>
  );
}

export function SliderRowSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div aria-busy="true" className="mb-8">
      <div className="flex items-center justify-between mb-3 px-1">
        <Skeleton className="h-5 w-40" />
      </div>
      <div className="flex gap-3 overflow-hidden">
        {Array.from({ length: count }).map((_, i) => (
          <MediaCardSkeleton key={i} />
        ))}
      </div>
    </div>
  );
}
