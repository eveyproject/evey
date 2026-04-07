import { memo, useRef, useState, useEffect, useCallback } from "react";
import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight } from "lucide-react";

interface MediaSliderProps {
  title: string;
  seeAllHref?: string;
  seeAllParams?: Record<string, string>;
  children: ReactNode;
  isLoading?: boolean;
  skeletonCount?: number;
}

function SkeletonCard() {
  return (
    <div className="flex-shrink-0 w-32 sm:w-40 aspect-[2/3]">
      <div
        className="w-full h-full animate-shimmer rounded-[var(--radius-md)]"
      />
      <div className="mt-2 space-y-1 px-0.5">
        <div
          className="h-3 w-4/5 animate-shimmer rounded-[var(--radius-sm)]"
        />
      </div>
    </div>
  );
}

export const MediaSlider = memo(function MediaSlider({
  title,
  seeAllHref,
  seeAllParams,
  children,
  isLoading,
  skeletonCount = 6,
}: MediaSliderProps) {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [showLeft, setShowLeft] = useState(false);
  const [showRight, setShowRight] = useState(false);

  // Touch state
  const touchStartX = useRef<number | null>(null);
  const touchStartY = useRef<number | null>(null);
  const touchStartTime = useRef<number>(0);
  const touchLocked = useRef<"horizontal" | "vertical" | null>(null);

  // Mouse drag state
  const isDragging = useRef(false);
  const dragStartX = useRef(0);
  const dragScrollLeft = useRef(0);

  const CARD_WIDTH = window.innerWidth < 640 ? 128 : 160;

  const checkOverflow = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setShowLeft(el.scrollLeft > 0);
    setShowRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4);
  }, []);

  useEffect(() => {
    checkOverflow();
  }, [isLoading, children, checkOverflow]);

  const scroll = (dir: "left" | "right") => {
    const el = scrollRef.current;
    if (!el) return;
    const amount = el.clientWidth - CARD_WIDTH;
    el.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
  };

  // --- Touch handlers ---

  function handleTouchStart(e: React.TouchEvent) {
    touchStartX.current = e.touches[0].clientX;
    touchStartY.current = e.touches[0].clientY;
    touchStartTime.current = Date.now();
    touchLocked.current = null;
  }

  function handleTouchMove(e: React.TouchEvent) {
    if (touchStartX.current === null || touchStartY.current === null) return;
    const dx = e.touches[0].clientX - touchStartX.current;
    const dy = e.touches[0].clientY - touchStartY.current;

    if (touchLocked.current === null) {
      if (Math.abs(dx) > Math.abs(dy)) {
        touchLocked.current = "horizontal";
      } else {
        touchLocked.current = "vertical";
      }
    }

    if (touchLocked.current === "horizontal") {
      e.preventDefault();
      const el = scrollRef.current;
      if (el) {
        // Live tracking while dragging
        el.scrollLeft -= dx * 0.05; // small incremental nudge (full tracking via touchEnd)
      }
    }
  }

  function handleTouchEnd(e: React.TouchEvent) {
    if (touchStartX.current === null || touchLocked.current !== "horizontal") {
      touchStartX.current = null;
      touchStartY.current = null;
      touchLocked.current = null;
      return;
    }

    const dx = e.changedTouches[0].clientX - touchStartX.current;
    const dt = Date.now() - touchStartTime.current;
    const velocity = Math.abs(dx) / dt; // px/ms

    touchStartX.current = null;
    touchStartY.current = null;
    touchLocked.current = null;

    if (Math.abs(dx) < 30) return;

    // Momentum: high velocity → scroll more cards
    const cards = velocity > 0.8 ? 3 : velocity > 0.4 ? 2 : 1;
    const el = scrollRef.current;
    if (!el) return;
    el.scrollBy({
      left: dx < 0 ? CARD_WIDTH * cards : -(CARD_WIDTH * cards),
      behavior: "smooth",
    });
  }

  // --- Mouse drag handlers ---

  function handleMouseDown(e: React.MouseEvent) {
    const el = scrollRef.current;
    if (!el) return;
    isDragging.current = true;
    dragStartX.current = e.clientX;
    dragScrollLeft.current = el.scrollLeft;
    el.style.cursor = "grabbing";
    el.style.userSelect = "none";
  }

  function handleMouseMove(e: React.MouseEvent) {
    if (!isDragging.current) return;
    const el = scrollRef.current;
    if (!el) return;
    const dx = e.clientX - dragStartX.current;
    el.scrollLeft = dragScrollLeft.current - dx;
  }

  function stopDrag() {
    if (!isDragging.current) return;
    isDragging.current = false;
    const el = scrollRef.current;
    if (!el) return;
    el.style.cursor = "grab";
    el.style.userSelect = "";
  }

  return (
    <section className="mb-10 group/slider" data-testid="media-slider">
      <div className="flex items-center justify-between mb-3 px-1">
        <h2 className="text-lg sm:text-xl font-semibold tracking-tight text-[var(--text)]">
          {title}
        </h2>
        {seeAllHref && (
          <Link
            to={seeAllHref}
            params={seeAllParams as never}
            className="text-sm transition-colors-fast text-[var(--accent)]"
          >
            {t("common.seeAll")}
          </Link>
        )}
      </div>

      <div className="relative">
        {showLeft && (
          <button
            onClick={() => scroll("left")}
            className="absolute left-0 top-0 bottom-2 z-raised flex items-center justify-center w-12 rounded-r-[var(--radius-md)] opacity-0 group-hover/slider:opacity-100 transition-opacity-fast backdrop-blur-sm bg-[var(--glass-bg)] border-r border-r-[var(--glass-border)]"
            aria-label={t("common.scrollLeft")}
            title={t("common.scrollLeft")}
          >
            <ChevronLeft size={20} className="text-[var(--overlay-text)]" />
          </button>
        )}

        <div
          ref={scrollRef}
          onScroll={checkOverflow}
          onTouchStart={handleTouchStart}
          onTouchMove={handleTouchMove}
          onTouchEnd={handleTouchEnd}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={stopDrag}
          onMouseLeave={stopDrag}
          className="flex gap-4 overflow-x-auto pb-2 -mx-1 px-1 snap-x snap-mandatory cursor-grab scrollbar-none"
        >
          {isLoading
            ? Array.from({ length: skeletonCount }).map((_, i) => <SkeletonCard key={i} />)
            : children}
        </div>

        {showRight && (
          <button
            onClick={() => scroll("right")}
            className="absolute right-0 top-0 bottom-2 z-raised flex items-center justify-center w-12 rounded-l-[var(--radius-md)] opacity-0 group-hover/slider:opacity-100 transition-opacity-fast backdrop-blur-sm bg-[var(--glass-bg)] border-l border-l-[var(--glass-border)]"
            aria-label={t("common.scrollRight")}
            title={t("common.scrollRight")}
          >
            <ChevronRight size={20} className="text-[var(--overlay-text)]" />
          </button>
        )}
      </div>
    </section>
  );
});
