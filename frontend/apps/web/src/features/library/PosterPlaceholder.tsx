import type { CSSProperties } from "react";
import { Clapperboard, Tv, Film } from "lucide-react";

// Pick a media-kind icon for a posterless item.
export function posterIconFor(type?: string) {
  if (
    type === "MEDIA_ITEM_TYPE_SERIES" ||
    type === "MEDIA_ITEM_TYPE_EPISODE" ||
    type === "MEDIA_ITEM_TYPE_SEASON"
  )
    return Tv;
  if (type === "MEDIA_ITEM_TYPE_MOVIE") return Clapperboard;
  return Film;
}

interface PosterPlaceholderProps {
  title?: string;
  type?: string;
  iconSize?: number;
  /** Show the title under the icon. Off for tiny thumbnails that already label elsewhere. */
  showTitle?: boolean;
  className?: string;
  style?: CSSProperties;
}

// Soft, intentional stand-in for a missing or failed poster: a faint accent-tinted
// gradient (.evey-poster-fallback) with the media-kind icon and, optionally, the title —
// so a posterless card reads as designed rather than broken.
export function PosterPlaceholder({
  title,
  type,
  iconSize = 30,
  showTitle = true,
  className = "",
  style,
}: PosterPlaceholderProps) {
  const Icon = posterIconFor(type);
  const initial = title ? title[0].toUpperCase() : "?";
  return (
    <div
      role="img"
      aria-label={title || "Media poster"}
      className={`evey-poster-fallback flex flex-col items-center justify-center gap-2.5 px-3 text-center ${className}`}
      style={style}
    >
      <Icon
        size={iconSize}
        strokeWidth={1.5}
        className="text-[var(--text-muted)] opacity-50"
        aria-hidden="true"
      />
      {showTitle && (
        <span className="line-clamp-3 text-xs font-medium leading-snug text-[var(--text-secondary)]">
          {title || initial}
        </span>
      )}
    </div>
  );
}
