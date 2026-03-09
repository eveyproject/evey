import { useRef, useState } from "react";
import { Link } from "@tanstack/react-router";

interface MediaCardItem {
  id?: string;
  title?: string;
  year?: number;
  type?: string;
  overview?: string;
  communityRating?: number;
  genres?: string[];
}

interface MediaCardProps {
  item: MediaCardItem;
  href?: string;
  posterSrc?: string;
  showProgress?: boolean;
  progressPercent?: number;
  showYear?: boolean;
  showType?: boolean;
}

const typeLabel = (t?: string) => {
  if (t === "MEDIA_ITEM_TYPE_MOVIE") return "Film";
  if (t === "MEDIA_ITEM_TYPE_SERIES") return "Serie";
  return "";
};

function HoverPreview({ item }: { item: MediaCardItem }) {
  const hasContent = item.overview || item.communityRating || (item.genres && item.genres.length > 0);
  if (!hasContent) return null;

  return (
    <div
      className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 rounded-lg p-3 shadow-xl z-50 pointer-events-none opacity-0 group-hover:opacity-100 transition-opacity duration-200 hidden sm:block"
      style={{
        backgroundColor: "var(--surface)",
        border: "1px solid var(--border)",
      }}
    >
      <p className="text-sm font-semibold mb-1 truncate" style={{ color: "var(--text)" }}>
        {item.title}
      </p>

      <div className="flex items-center gap-2 mb-1.5 text-xs" style={{ color: "var(--text-muted)" }}>
        {item.year && <span>{item.year}</span>}
        {item.communityRating != null && item.communityRating > 0 && (
          <span className="flex items-center gap-0.5">
            <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor" style={{ color: "var(--accent)" }}>
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
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
              className="text-[10px] px-1.5 py-0.5 rounded-full"
              style={{
                backgroundColor: "var(--surface-raised)",
                color: "var(--text-muted)",
              }}
            >
              {g}
            </span>
          ))}
        </div>
      )}

      {item.overview && (
        <p
          className="text-xs leading-relaxed line-clamp-3"
          style={{ color: "var(--text-secondary)" }}
        >
          {item.overview}
        </p>
      )}
    </div>
  );
}

export function MediaCard({
  item,
  href,
  posterSrc,
  showProgress,
  progressPercent,
  showYear = true,
  showType,
}: MediaCardProps) {
  const [imgLoaded, setImgLoaded] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showPreview, setShowPreview] = useState(false);
  const poster =
    posterSrc ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster` : null);
  const initial = item.title ? item.title[0].toUpperCase() : "?";
  const pct =
    progressPercent !== undefined
      ? Math.min(100, Math.max(0, Math.round(progressPercent)))
      : undefined;

  const linkTo = href ?? "/media/$mediaId";
  const linkParams = href ? undefined : { mediaId: item.id ?? "" };

  const hasPreviewData = !!(item.overview || item.communityRating || (item.genres && item.genres.length > 0));

  function handleMouseEnter() {
    if (!hasPreviewData) return;
    hoverTimer.current = setTimeout(() => setShowPreview(true), 400);
  }

  function handleMouseLeave() {
    if (hoverTimer.current) clearTimeout(hoverTimer.current);
    setShowPreview(false);
  }

  return (
    <Link
      to={linkTo}
      params={linkParams as never}
      className="group block flex-shrink-0 w-32 sm:w-40 relative"
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      {showPreview && <HoverPreview item={item} />}

      <div
        className="relative w-full overflow-hidden transition-all duration-200 ease-out group-hover:scale-105 group-hover:shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
        style={{
          aspectRatio: "2/3",
          borderRadius: "var(--radius-md)",
          boxShadow: "0 2px 8px rgba(0,0,0,0.2)",
        }}
      >
        {poster ? (
          <img
            src={poster}
            alt={item.title ?? ""}
            loading="lazy"
            className="w-full h-full object-cover transition-opacity duration-300"
            style={{ opacity: imgLoaded ? 1 : 0 }}
            onLoad={() => setImgLoaded(true)}
            onError={(e) => {
              const t = e.currentTarget;
              t.style.display = "none";
              const fb = t.nextElementSibling as HTMLElement | null;
              if (fb) fb.style.display = "flex";
            }}
          />
        ) : null}
        <div
          className="absolute inset-0 items-center justify-center text-2xl font-bold"
          style={{
            backgroundColor: "var(--surface-raised)",
            color: "var(--text-muted)",
            display: poster ? "none" : "flex",
          }}
        >
          {initial}
        </div>

        {showYear && item.year !== undefined && (
          <span
            className="absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5"
            style={{
              backgroundColor: "rgba(0,0,0,0.7)",
              color: "#fff",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {item.year}
          </span>
        )}

        {/* Hover overlay: play button + title */}
        <div
          className="absolute inset-0 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          style={{ background: "linear-gradient(to top, rgba(0,0,0,0.8) 0%, rgba(0,0,0,0.3) 50%, rgba(0,0,0,0.1) 100%)" }}
        >
          <div
            className="w-12 h-12 rounded-full flex items-center justify-center backdrop-blur-sm transition-transform duration-200 group-hover:scale-110"
            style={{ backgroundColor: "rgba(255,255,255,0.2)", border: "2px solid rgba(255,255,255,0.6)" }}
          >
            <svg
              className="w-5 h-5 text-white ml-0.5"
              fill="currentColor"
              viewBox="0 0 20 20"
            >
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
          <p className="absolute bottom-2 left-2 right-2 text-xs font-medium text-white truncate text-center">
            {item.title}
          </p>
        </div>

        {showProgress && pct !== undefined && (
          <div
            className="absolute bottom-0 left-0 right-0 h-1.5 z-10"
            style={{ backgroundColor: "rgba(0,0,0,0.5)" }}
          >
            <div
              className="h-full transition-all duration-300"
              style={{
                backgroundColor: "var(--accent)",
                width: `${pct}%`,
                boxShadow: "0 0 6px var(--accent)",
              }}
            />
          </div>
        )}
      </div>

      <div className="mt-2 px-0.5">
        <p
          className="text-sm font-medium truncate"
          style={{ color: "var(--text)" }}
        >
          {item.title}
        </p>
        {showType && typeLabel(item.type) && (
          <span
            className="text-xs px-1.5 py-0.5 rounded"
            style={{
              backgroundColor: "var(--surface-raised)",
              color: "var(--text-muted)",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {typeLabel(item.type)}
          </span>
        )}
      </div>
    </Link>
  );
}
