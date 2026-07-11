import { Children, memo, useRef, useState, useEffect, useCallback, useSyncExternalStore } from "react";
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

function subscribeToResize(cb: () => void) {
  window.addEventListener("resize", cb);
  return () => window.removeEventListener("resize", cb);
}

function getCardWidth() {
  return window.innerWidth < 640 ? 128 : 160;
}

// Enough to fill the row on a wide desktop; phones show ~3.
const INITIAL_CARDS = 12;

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
  // Cap the initially-mounted cards: a home page stacks 6+ sliders and
  // mounting 20 cards each (hooks, image decode, DOM) is the single biggest
  // chunk of its first render. The rest mounts on the first horizontal
  // scroll / arrow press - before any capped card can enter the viewport.
  const [expanded, setExpanded] = useState(false);
  const allChildren = Children.toArray(children);
  const hasCapped = !expanded && allChildren.length > INITIAL_CARDS;
  const visibleChildren = hasCapped ? allChildren.slice(0, INITIAL_CARDS) : allChildren;

  // Mouse drag state
  const isDragging = useRef(false);
  const dragStartX = useRef(0);
  const dragScrollLeft = useRef(0);

  const CARD_WIDTH = useSyncExternalStore(subscribeToResize, getCardWidth);

  const checkOverflow = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    setShowLeft(el.scrollLeft > 0);
    // Capped rows always advertise the right arrow: the capped tail may not
    // overflow the container yet (ultra-wide screens) but there IS more.
    setShowRight(hasCapped || el.scrollLeft + el.clientWidth < el.scrollWidth - 4);
  }, [hasCapped]);

  useEffect(() => {
    checkOverflow();
  }, [isLoading, children, checkOverflow]);

  const scroll = (dir: "left" | "right") => {
    const el = scrollRef.current;
    if (!el) return;
    if (dir === "right") setExpanded(true);
    const amount = el.clientWidth - CARD_WIDTH;
    el.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
  };

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
        {seeAllHref ? (
          <Link
            to={seeAllHref}
            params={seeAllParams as never}
            className="group/title inline-flex items-baseline gap-1 no-underline hover:opacity-90 transition-opacity-fast"
          >
            <h2 className="text-lg sm:text-xl font-semibold tracking-tight text-[var(--text)]">
              {title}
            </h2>
          </Link>
        ) : (
          <h2 className="text-lg sm:text-xl font-semibold tracking-tight text-[var(--text)]">
            {title}
          </h2>
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
            <ChevronLeft size={20} className="text-[var(--overlay-text)]"  aria-hidden="true" />
          </button>
        )}

        <div
          ref={scrollRef}
          onScroll={() => {
            setExpanded(true);
            checkOverflow();
          }}
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={stopDrag}
          onMouseLeave={stopDrag}
          className="flex gap-4 overflow-x-auto pb-2 -mx-1 px-1 snap-x snap-mandatory cursor-grab scrollbar-none"
        >
          {isLoading
            ? Array.from({ length: skeletonCount }).map((_, i) => <SkeletonCard key={i} />)
            : visibleChildren}
        </div>

        {showRight && (
          <button
            onClick={() => scroll("right")}
            className="absolute right-0 top-0 bottom-2 z-raised flex items-center justify-center w-12 rounded-l-[var(--radius-md)] opacity-0 group-hover/slider:opacity-100 transition-opacity-fast backdrop-blur-sm bg-[var(--glass-bg)] border-l border-l-[var(--glass-border)]"
            aria-label={t("common.scrollRight")}
            title={t("common.scrollRight")}
          >
            <ChevronRight size={20} className="text-[var(--overlay-text)]"  aria-hidden="true" />
          </button>
        )}
      </div>
    </section>
  );
});
