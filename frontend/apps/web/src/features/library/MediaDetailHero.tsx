import { useMemo } from "react";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useTranslation } from "react-i18next";
import { Pencil } from "lucide-react";
import { BackButton } from "@/components/ui/BackButton";
import { Breadcrumbs } from "@/components/ui/Breadcrumbs";
import { Stars } from "./MediaDetailStars";
import { certInfoForLocale } from "@/utils/certifications";

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
  const { t, i18n } = useTranslation();
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } });
  const profileRegion = (profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0])?.region;
  const certInfo = certInfoForLocale(item.certificationsByCountry, i18n.language, item.officialRating, profileRegion);

  const backdropSrcSet = useMemo(() => {
    if (!backdropUrl) return undefined;
    const sep = backdropUrl.includes("?") ? "&" : "?";
    return `${backdropUrl}${sep}w=800 800w, ${backdropUrl}${sep}w=1280 1280w, ${backdropUrl}${sep}w=1920 1920w`;
  }, [backdropUrl]);

  return (
    <div className="relative w-full overflow-hidden -mx-4 sm:-mx-6 -mt-4 sm:-mt-6 mb-0 h-[clamp(200px,50vw,420px)] sm:h-[clamp(260px,50vw,420px)]">
      <div className="absolute top-0 left-0 right-0 z-raised px-4 sm:px-6 pt-4 sm:pt-5 gradient-hero-scrim">
        <BackButton fallback="/library" className="mb-1 !text-white hover:!text-white/80" />
        <Breadcrumbs items={breadcrumbItems} />
      </div>
      {backdropUrl ? (
        <img src={backdropUrl} srcSet={backdropSrcSet} sizes="100vw" alt={item.title ?? ""} className="w-full h-full object-cover" data-testid="media-backdrop" onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }} />
      ) : (
        <div className="w-full h-full bg-[var(--surface)]" />
      )}
      <div className="absolute inset-0 gradient-hero-fade" />
      <div className="absolute inset-0 gradient-hero-overlay" />
      <div className="absolute bottom-0 left-0 right-0 px-4 sm:px-6 pb-4 sm:pb-6">
        <div className="flex items-center gap-2 mb-2 flex-wrap">
          {certInfo && (
            <span
              className="text-xs font-bold px-2 py-0.5 border text-[var(--overlay-text-bright)] border-[var(--overlay-dot-inactive)] rounded-[var(--radius-sm)]"
              title={`${t(`certBody.${certInfo.country}`, { defaultValue: certInfo.country })} ${certInfo.rating}`}
            >
              {t(`certBody.${certInfo.country}`, { defaultValue: certInfo.country })} {certInfo.rating}
            </span>
          )}
          {duration && (
            <span className="text-sm text-[var(--overlay-text-muted)]">{duration}</span>
          )}
          {item.year && (
            <span className="text-sm text-[var(--overlay-text-muted)]">{item.year}</span>
          )}
          {item.communityRating !== undefined && item.communityRating > 0 && (
            <span className="flex items-center gap-1.5">
              <Stars rating={item.communityRating} />
              <span className="text-sm text-[var(--overlay-text-muted)]">{item.communityRating.toFixed(1)}</span>
            </span>
          )}
          {item.providerIds?.imdb && (
            <a
              href={`https://www.imdb.com/title/${item.providerIds.imdb}/`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs font-bold px-1.5 py-0.5 rounded bg-[var(--badge-imdb-bg)] text-[var(--badge-imdb-text)] no-underline"
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
              className="text-xs font-bold px-1.5 py-0.5 rounded bg-[var(--badge-tmdb-bg)] text-[var(--badge-tmdb-text)] no-underline"
              onClick={(e) => e.stopPropagation()}
            >
              TMDB
            </a>
          )}
        </div>
        {parentItem && (
          <div className="mb-1">
            <Link to="/media/$mediaId" params={{ mediaId: item.parentId! }} className="text-sm font-medium hover:underline text-[var(--overlay-text-muted)]">
              {parentItem.title}
            </Link>
            <span className="text-sm mx-1 text-[var(--overlay-dot-inactive)]">&rsaquo;</span>
          </div>
        )}
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-2xl sm:text-4xl font-bold text-white drop-shadow-lg" data-testid="media-title">{item.title}</h1>
          {item.id && (
            <button
              onClick={onEditToggle}
              title={t("media.detail.editMetadata")}
              aria-label={t("media.detail.editMetadata")}
              className="flex-shrink-0 p-1.5 rounded-full transition-opacity-fast hover:opacity-80 bg-[var(--overlay-chip-bg)] text-[var(--overlay-text-muted)]"
            >
              <Pencil size={16}  aria-hidden="true" />
            </button>
          )}
        </div>
        {item.genres && item.genres.length > 0 && (
          <div className="flex flex-wrap gap-2" data-testid="media-genres">
            {item.genres.map((g) => (
              <span
                key={g}
                className="text-xs px-2.5 py-1 font-medium bg-[var(--overlay-btn-bg)] text-[var(--overlay-text-bright)] rounded-full backdrop-blur-[4px]"
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
