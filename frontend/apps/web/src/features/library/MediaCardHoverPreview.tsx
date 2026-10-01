import { memo } from "react";
import { useTranslation } from "react-i18next";
import { itemLabel } from "@evey/api";
import { Star } from "lucide-react";
import type { MediaCardItem } from "./MediaCard";
import { typeLabel } from "@/utils/typeLabel";
import { GenreDot } from "@/components/ui/GenreDot";

export const HoverPreview = memo(function HoverPreview({ item }: { item: MediaCardItem }) {
  const { t } = useTranslation();
  const hasContent = item.overview || item.communityRating || (item.genres && item.genres.length > 0);
  if (!hasContent) return null;

  return (
    <div
      className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 rounded-lg p-3 shadow-xl z-overlay pointer-events-none transition-opacity-fast duration-200 hidden sm:block bg-[var(--surface)] border border-[var(--border)]"
    >
      <p className="text-sm font-semibold mb-1 truncate text-[var(--text)]">
        {itemLabel(item, t)}
      </p>

      <div className="flex items-center gap-2 mb-1.5 text-xs text-[var(--text-muted)]">
        {!!item.year && <span>{item.year}</span>}
        {item.communityRating != null && item.communityRating > 0 && (
          <span className="flex items-center gap-0.5">
            <Star size={12} fill="currentColor" className="text-[var(--accent)]" />
            {item.communityRating.toFixed(1)}
          </span>
        )}
        {typeLabel(item.type) && <span>{typeLabel(item.type)}</span>}
      </div>

      {item.genres && item.genres.length > 0 && (
        <div className="flex flex-wrap gap-1 mb-1.5">
          {item.genres.slice(0, 3).map((g) => (
            <span
              key={g}
              className="text-xs px-1.5 py-0.5 rounded-full bg-[var(--surface-raised)] text-[var(--text-muted)] inline-flex items-center gap-1"
            >
              <GenreDot label={g} />
              {g}
            </span>
          ))}
        </div>
      )}

      {item.overview && (
        <p
          className="text-xs leading-relaxed line-clamp-3 text-[var(--text-secondary)]"
        >
          {item.overview}
        </p>
      )}
    </div>
  );
});
