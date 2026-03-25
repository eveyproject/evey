import { Link } from "@tanstack/react-router";
import type { V1MediaItem, V1MediaPerson } from "@evey/api";
import { useTranslation } from "react-i18next";
import { Pencil } from "lucide-react";
import { BackButton } from "@/components/ui/BackButton";
import { Breadcrumbs } from "@/components/ui/Breadcrumbs";
import { Stars } from "./MediaDetailStars";

interface MediaDetailHeroProps {
  item: V1MediaItem;
  backdropUrl?: string;
  posterUrl?: string;
  duration: string;
  breadcrumbItems: Array<{ label: string; to?: string }>;
  parentItem?: V1MediaItem;
  onEditToggle: () => void;
}

export function MediaDetailHero({
  item,
  backdropUrl,
  duration,
  breadcrumbItems,
  parentItem,
  onEditToggle,
}: MediaDetailHeroProps) {
  const { t } = useTranslation();

  return (
    <div className="relative w-full overflow-hidden -mx-4 sm:-mx-6 -mt-4 sm:-mt-6 mb-0" style={{ height: "clamp(260px, 50vw, 420px)" }}>
      <div className="absolute top-0 left-0 right-0 z-raised px-4 sm:px-6 pt-4 sm:pt-5" style={{ background: `linear-gradient(to bottom, var(--overlay-scrim) 0%, var(--overlay-chip-bg) 60%, transparent 100%)`, textShadow: `0 1px 6px var(--overlay-scrim)` }}>
        <BackButton fallback="/library" className="mb-1 !text-white hover:!text-white/80" />
        <Breadcrumbs items={breadcrumbItems} />
      </div>
      {backdropUrl ? (
        <img src={backdropUrl} alt={item.title ?? ""} className="w-full h-full object-cover" data-testid="media-backdrop" onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }} />
      ) : (
        <div className="w-full h-full" style={{ backgroundColor: "var(--surface)" }} />
      )}
      <div
        className="absolute inset-0"
        style={{
          background: "linear-gradient(to top, var(--bg) 0%, var(--bg) 2%, transparent 100%)",
        }}
      />
      <div
        className="absolute inset-0"
        style={{
          background: "linear-gradient(to top, var(--bg) 0%, var(--overlay-bg) 40%, var(--overlay-border-subtle) 70%, transparent 100%)",
        }}
      />
      <div className="absolute bottom-0 left-0 right-0 px-4 sm:px-6 pb-4 sm:pb-6">
        <div className="flex items-center gap-2 mb-2 flex-wrap">
          {item.officialRating && (
            <span
              className="text-xs font-medium px-2 py-0.5 border"
              style={{ color: "var(--overlay-text-bright)", borderColor: "var(--overlay-dot-inactive)", borderRadius: "var(--radius-sm)" }}
            >
              {item.officialRating}
            </span>
          )}
          {duration && (
            <span className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>{duration}</span>
          )}
          {item.year && (
            <span className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>{item.year}</span>
          )}
          {item.communityRating !== undefined && item.communityRating > 0 && (
            <span className="flex items-center gap-1.5">
              <Stars rating={item.communityRating} />
              <span className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>{item.communityRating.toFixed(1)}</span>
            </span>
          )}
          {item.providerIds?.imdb && (
            <a
              href={`https://www.imdb.com/title/${item.providerIds.imdb}/`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs font-bold px-1.5 py-0.5 rounded"
              style={{ backgroundColor: "var(--badge-imdb-bg)", color: "var(--badge-imdb-text)", textDecoration: "none" }}
              onClick={(e) => e.stopPropagation()}
            >
              IMDb
            </a>
          )}
          {item.providerIds?.tmdb && (
            <a
              href={`https://www.themoviedb.org/${item.type === "MEDIA_ITEM_TYPE_MOVIE" ? "movie" : "tv"}/${item.providerIds.tmdb}`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs font-bold px-1.5 py-0.5 rounded"
              style={{ backgroundColor: "var(--badge-tmdb-bg)", color: "var(--badge-tmdb-text)", textDecoration: "none" }}
              onClick={(e) => e.stopPropagation()}
            >
              TMDB
            </a>
          )}
        </div>
        {parentItem && (
          <div className="mb-1">
            <Link to="/media/$mediaId" params={{ mediaId: item.parentId! }} className="text-sm font-medium hover:underline" style={{ color: "var(--overlay-text-muted)" }}>
              {parentItem.title}
            </Link>
            <span className="text-sm mx-1" style={{ color: "var(--overlay-dot-inactive)" }}>&rsaquo;</span>
          </div>
        )}
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-2xl sm:text-4xl font-bold text-white drop-shadow-lg" data-testid="media-title">{item.title}</h1>
          {item.id && (
            <button
              onClick={onEditToggle}
              title={t("media.detail.editMetadata")}
              className="flex-shrink-0 p-1.5 rounded-full transition-opacity hover:opacity-80"
              style={{ backgroundColor: "var(--overlay-chip-bg)", color: "var(--overlay-text-muted)" }}
            >
              <Pencil size={16} />
            </button>
          )}
        </div>
        {item.genres && item.genres.length > 0 && (
          <div className="flex flex-wrap gap-2" data-testid="media-genres">
            {item.genres.map((g) => (
              <span
                key={g}
                className="text-xs px-2.5 py-1 font-medium"
                style={{
                  backgroundColor: "var(--overlay-btn-bg)",
                  color: "var(--overlay-text-bright)",
                  borderRadius: "9999px",
                  backdropFilter: "blur(4px)",
                }}
              >
                {g}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
