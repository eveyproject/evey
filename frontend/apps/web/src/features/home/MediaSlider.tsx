import { useRef, useState } from "react";
import type { ReactNode } from "react";

interface MediaSliderProps {
  title: string;
  seeAllHref?: string;
  children: ReactNode;
  isLoading?: boolean;
  skeletonCount?: number;
}

function SkeletonCard() {
  return (
    <div className="flex-shrink-0 w-40" style={{ aspectRatio: "2/3" }}>
      <div
        className="w-full h-full animate-pulse"
        style={{
          backgroundColor: "var(--surface-raised)",
          borderRadius: "var(--radius-md)",
        }}
      />
      <div className="mt-2 space-y-1 px-0.5">
        <div
          className="h-3 w-4/5 animate-pulse rounded"
          style={{ backgroundColor: "var(--surface-raised)" }}
        />
      </div>
    </div>
  );
}

export function MediaSlider({
  title,
  seeAllHref,
  children,
  isLoading,
  skeletonCount = 6,
}: MediaSliderProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [showLeft, setShowLeft] = useState(false);
  const [showRight, setShowRight] = useState(true);

  const scroll = (dir: "left" | "right") => {
    const el = scrollRef.current;
    if (!el) return;
    const amount = el.clientWidth * 0.8;
    el.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
  };

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    setShowLeft(el.scrollLeft > 0);
    setShowRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4);
  };

  return (
    <section className="mb-8 group/slider">
      <div className="flex items-center justify-between mb-3 px-1">
        <h2 className="text-lg font-semibold" style={{ color: "var(--text)" }}>
          {title}
        </h2>
        {seeAllHref && (
          <a
            href={seeAllHref}
            className="text-sm transition-colors"
            style={{ color: "var(--accent)" }}
          >
            See all →
          </a>
        )}
      </div>

      <div className="relative">
        {showLeft && (
          <button
            onClick={() => scroll("left")}
            className="absolute left-0 top-0 bottom-0 z-10 flex items-center justify-center w-10 opacity-0 group-hover/slider:opacity-100 transition-opacity"
            style={{ backgroundColor: "rgba(0,0,0,0.5)" }}
            aria-label="Scroll left"
          >
            <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
        )}

        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="flex gap-3 overflow-x-auto pb-2"
          style={{ scrollbarWidth: "none", scrollSnapType: "x mandatory" }}
        >
          {isLoading
            ? Array.from({ length: skeletonCount }).map((_, i) => <SkeletonCard key={i} />)
            : children}
        </div>

        {showRight && (
          <button
            onClick={() => scroll("right")}
            className="absolute right-0 top-0 bottom-0 z-10 flex items-center justify-center w-10 opacity-0 group-hover/slider:opacity-100 transition-opacity"
            style={{ backgroundColor: "rgba(0,0,0,0.5)" }}
            aria-label="Scroll right"
          >
            <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
        )}
      </div>
    </section>
  );
}
