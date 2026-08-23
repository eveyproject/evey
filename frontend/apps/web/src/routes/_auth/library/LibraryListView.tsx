import { memo, useEffect, useRef, useState } from "react";
import { Star } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useVirtualizer } from "@tanstack/react-virtual";
import { typeLabel } from "@/utils/typeLabel";
import type { V1MediaItem } from "@evey/api";
import { formatDurationMs, itemLabel } from "@evey/api";
import { artworkUrl } from "@/features/library/artworkUrl";

// Rows have no wrapping text (title is truncated to one line) and every icon/
// image is a fixed size, so the row height never varies with content - a
// constant estimate is exact, not an approximation, and measureElement would
// just be dead weight. 40px poster + 2*8px vertical padding + 1px border.
const ROW_HEIGHT = 57;

// Same pattern as PhotoGrid (routes/_auth/photos/index.lazy.tsx): the page's
// actual scroll container is an ancestor (AuthLayout's #main-content), not
// this component's own root, so the virtualizer needs that ancestor plus a
// scrollMargin to translate virtual offsets into on-screen positions.
function findScrollParent(el: HTMLElement | null): HTMLElement | null {
  let node: HTMLElement | null = el?.parentElement ?? null;
  while (node) {
    const style = getComputedStyle(node);
    if (/(auto|scroll|overlay)/.test(style.overflowY)) return node;
    node = node.parentElement;
  }
  return (document.scrollingElement as HTMLElement) ?? document.documentElement;
}

const ListRow = memo(function ListRow({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const title = itemLabel(item, t);
  const year = item.year;
  const rating = item.communityRating;
  const type = item.type;
  const id = item.id;
  const durationMs = item.durationMs;
  const poster = id ? artworkUrl(id, "poster", item) : null;

  return (
    <Link
      to="/media/$mediaId"
      params={{ mediaId: id ?? "" }}
      className="flex items-center gap-4 px-3 py-2 rounded-[var(--radius-md)] transition-colors-fast hover:bg-[var(--surface-raised)] border-b border-[var(--border)]"
      data-testid="list-row"
    >
      <div
        className="flex-shrink-0 w-7 h-10 rounded overflow-hidden bg-[var(--surface-raised)]"
      >
        {poster && (
          <img src={poster} alt={title} className="w-full h-full object-cover" loading="lazy" />
        )}
      </div>
      <span className="flex-1 min-w-0 text-sm font-medium truncate text-[var(--text)]">
        {title}
      </span>
      <span className="hidden sm:block w-16 text-xs text-right flex-shrink-0 text-[var(--text-muted)]">
        {year ?? "\u2014"}
      </span>
      <span className="hidden md:flex items-center gap-1 w-16 text-xs flex-shrink-0 justify-end text-[var(--text-muted)]">
        {rating != null && rating > 0 ? (
          <>
            <Star size={12} fill="currentColor" className="text-[var(--accent)]" />
            {rating.toFixed(1)}
          </>
        ) : "\u2014"}
      </span>
      {typeLabel(type) && (
        <span
          className="hidden lg:block text-xs px-1.5 py-0.5 flex-shrink-0 bg-[var(--surface-raised)] text-[var(--text-muted)] rounded-[var(--radius-sm)]"
        >
          {typeLabel(type)}
        </span>
      )}
      <span className="hidden lg:block w-16 text-xs text-right flex-shrink-0 text-[var(--text-muted)]">
        {formatDurationMs(durationMs) || "\u2014"}
      </span>
    </Link>
  );
});

interface LibraryListViewProps {
  items: V1MediaItem[];
  // Optional infinite-scroll wiring: the caller owns the paged query, so it
  // hands down its own hasNextPage/isFetchingNextPage state and a callback to
  // fetch the next page. Left undefined, the view just renders the items it
  // was given (unchanged behavior for callers that don't paginate).
  hasNextPage?: boolean;
  isFetchingNextPage?: boolean;
  onLoadMore?: () => void;
}

export function LibraryListView({ items, hasNextPage = false, isFetchingNextPage = false, onLoadMore }: LibraryListViewProps) {
  const { t } = useTranslation();
  const parentRef = useRef<HTMLDivElement>(null);
  const [scrollEl, setScrollEl] = useState<HTMLElement | null>(null);

  useEffect(() => {
    setScrollEl(findScrollParent(parentRef.current));
  }, []);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
    scrollMargin:
      parentRef.current && scrollEl
        ? parentRef.current.getBoundingClientRect().top - scrollEl.getBoundingClientRect().top + scrollEl.scrollTop
        : 0,
  });
  const virtualItems = virtualizer.getVirtualItems();

  // Windowing means a sentinel at the end of the DOM no longer lines up with
  // the actual bottom of the scroll - trigger from the last *virtual* item
  // reaching the end of the list instead (mirrors LogsExplorer/WatchHistory).
  useEffect(() => {
    const last = virtualItems[virtualItems.length - 1];
    if (!last) return;
    if (last.index >= items.length - 1 && hasNextPage && !isFetchingNextPage) onLoadMore?.();
  }, [virtualItems, items.length, hasNextPage, isFetchingNextPage, onLoadMore]);

  return (
    <div
      className="rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]"
    >
      <div
        className="flex items-center gap-4 px-3 py-2 text-xs font-semibold text-[var(--text-muted)] border-b border-[var(--border)]"
      >
        <span className="w-7 flex-shrink-0" />
        <span className="flex-1 min-w-0">{t("media.library.titleSort")}</span>
        <span className="hidden sm:block w-16 text-right flex-shrink-0">{t("media.library.year")}</span>
        <span className="hidden md:block w-16 text-right flex-shrink-0">{t("media.library.ratingSort")}</span>
        <span className="hidden lg:block w-12 flex-shrink-0">{t("media.library.type")}</span>
        <span className="hidden lg:block w-16 text-right flex-shrink-0">{t("media.library.duration")}</span>
      </div>
      <div ref={parentRef} style={{ height: virtualizer.getTotalSize(), width: "100%", position: "relative" }}>
        {virtualItems.map((vItem) => (
          <div
            key={vItem.key}
            data-index={vItem.index}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              transform: `translateY(${vItem.start - virtualizer.options.scrollMargin}px)`,
            }}
          >
            <ListRow item={items[vItem.index]} />
          </div>
        ))}
      </div>
    </div>
  );
}
