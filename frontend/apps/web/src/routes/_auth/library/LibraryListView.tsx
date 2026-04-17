import { memo } from "react";
import { Star } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { typeLabel } from "@/utils/typeLabel";
import type { V1MediaItem } from "@evey/api";

function formatDuration(ms: string): string {
  const totalMin = Math.round(parseInt(ms, 10) / 60000);
  if (totalMin < 60) return `${totalMin}m`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

const ListRow = memo(function ListRow({ item }: { item: V1MediaItem }) {
  const title = item.title ?? "";
  const year = item.year;
  const rating = item.communityRating;
  const type = item.type;
  const id = item.id;
  const durationMs = item.durationMs;
  const poster = id ? `/api/v1/media/${id}/artwork/poster` : null;

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
        {durationMs ? formatDuration(durationMs) : "\u2014"}
      </span>
    </Link>
  );
});

export function LibraryListView({ items }: { items: V1MediaItem[] }) {
  const { t } = useTranslation();

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
      {items.map((item) => (
        <ListRow key={item.id} item={item} />
      ))}
    </div>
  );
}
