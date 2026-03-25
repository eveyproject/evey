import { memo } from "react";

interface FeedSectionProps {
  title: string;
  children: React.ReactNode;
  isLoading?: boolean;
}

function SkeletonCard() {
  return (
    <div
      className="flex-shrink-0 w-40 animate-pulse aspect-[2/3]"
    >
      <div
        className="w-full h-full bg-[var(--surface-raised)] rounded-[var(--radius-md)]"
      />
      <div className="mt-2 space-y-1 px-0.5">
        <div
          className="h-3 w-4/5 rounded bg-[var(--surface-raised)]"
        />
        <div
          className="h-3 w-2/5 rounded bg-[var(--surface-raised)]"
        />
      </div>
    </div>
  );
}

export const FeedSection = memo(function FeedSection({ title, children, isLoading }: FeedSectionProps) {
  return (
    <section className="mb-8">
      <h2
        className="text-xl font-semibold mb-4 text-[var(--text)]"
      >
        {title}
      </h2>
      <div className="overflow-x-auto pb-2" style={{ scrollbarWidth: "thin" }}>
        <div className="flex gap-4 w-max">
          {isLoading
            ? Array.from({ length: 5 }).map((_, i) => (
                <SkeletonCard key={i} />
              ))
            : children}
        </div>
      </div>
    </section>
  );
});
