import { useMemo } from "react";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useTranslation } from "react-i18next";
import { Pencil } from "lucide-react";
import { BackButton } from "@/components/ui/BackButton";
import { Stars } from "./MediaDetailStars";
import { certInfoForLocale } from "@/utils/certifications";

// Per-source critic scores (from the enricher's provider_ratings). tmdb is
// excluded - the prominent Stars rating already represents the community score.
const RATING_SOURCE_LABELS: Record<string, string> = {
  imdb: "IMDb",
  rt: "RT",
  metacritic: "Metacritic",
};

function formatRating(value: number, maxValue: number): string {
  // 100-scale sources (RT, Metacritic) read as a score; 10-scale (IMDb) keeps a decimal.
  return maxValue === 100 ? `${Math.round(value)}` : value.toFixed(1);
}

interface MediaDetailHeroProps {
  item: V1MediaItem;
  backdropUrl?: string;
  posterUrl?: string;
  duration: string;
  parentItem?: V1MediaItem;
  onEditToggle: () => void;
}

export function MediaDetailHero({
  item,
  backdropUrl,
  duration,
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
    // These widths must EXIST server-side: /artwork snaps ?w= to the nearest
    // allowed variant, so before 1280/1920 were added every entry here
    // collapsed onto the same 780px file and the browser stretched it across
    // the full viewport. That was the "pixelated backdrop".
    return `${backdropUrl}${sep}w=780 780w, ${backdropUrl}${sep}w=1280 1280w, ${backdropUrl}${sep}w=1920 1920w`;
  }, [backdropUrl]);

  return (
    // Height follows the backdrop's own 16:9 shape instead of a fixed band.
    // The old clamp capped at 420px while the image spans the full width, so on
    // a wide viewport object-cover had to crop ~40% of the frame vertically -
    // taking the top and bottom of the composition with it. Letting the box
    // keep the source aspect ratio (bounded, so it never eats the whole screen)
    // means there is little left to crop.
    <div className="relative w-full overflow-hidden -mx-4 sm:-mx-6 -mt-4 sm:-mt-6 mb-0 aspect-[16/9] max-h-[min(70vh,620px)] min-h-[220px]">
      {/* Back button only. The breadcrumb repeated the hierarchy the title and
          the parent link already carry, and cost a line of chrome over the
          artwork on every media page. */}
      <div className="absolute top-0 left-0 right-0 z-raised px-4 sm:px-6 pt-4 sm:pt-5 gradient-hero-scrim">
        <BackButton fallback="/library" className="!text-white hover:!text-white/80" />
      </div>
      {backdropUrl ? (
        <img src={backdropUrl} srcSet={backdropSrcSet} sizes="100vw" alt={item.title ?? ""} className="w-full h-full object-cover object-[center_28%]" data-testid="media-backdrop" onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }} />
      ) : (
        <div className="w-full h-full bg-[var(--surface)]" />
      )}
      <div className="absolute inset-0 gradient-hero-fade" />
      <div className="absolute inset-0 gradient-hero-overlay" />
      {/* Indented on desktop so the overlapping poster (rendered by MediaDetail
          just below, pulled up over this artwork) never sits on top of the
          title. */}
      <div className="absolute bottom-0 left-0 right-0 px-4 sm:px-6 pb-4 sm:pb-6 md:pl-[22rem]">
        <div className="flex items-center gap-x-3 gap-y-2 mb-3 flex-wrap [&>*+*]:before:content-[''] [&>*+*]:before:hidden">
          {certInfo && (
            <span
              className="text-[11px] font-semibold tracking-[0.08em] px-2 py-0.5 border border-white/30 text-white/80 rounded-[var(--radius-sm)]"
              title={`${t(`media.certBody.${certInfo.country}`, { defaultValue: certInfo.country })} ${certInfo.rating}`}
            >
              {t(`media.certBody.${certInfo.country}`, { defaultValue: certInfo.country })} {certInfo.rating}
            </span>
          )}
          {duration && (
            <span className="text-sm tabular-nums text-white/70">{duration}</span>
          )}
          {item.year && (
            <span className="text-sm tabular-nums text-white/70">{item.year}</span>
          )}
          {item.communityRating !== undefined && item.communityRating > 0 && (
            <span className="flex items-center gap-1.5">
              <Stars rating={item.communityRating} />
              <span className="text-sm tabular-nums text-white/70">{item.communityRating.toFixed(1)}</span>
            </span>
          )}
          {(item.ratings ?? [])
            .filter((r) => r.source && RATING_SOURCE_LABELS[r.source] && (r.value ?? 0) > 0)
            .map((r) => (
              <span key={r.source} className="text-sm text-[var(--overlay-text-muted)]">
                <span className="font-semibold">{RATING_SOURCE_LABELS[r.source!]}</span>{" "}
                {formatRating(r.value ?? 0, r.maxValue ?? 10)}
                {r.source === "rt" ? "%" : ""}
              </span>
            ))}
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
        {/* The parent title reads as an eyebrow, not as a breadcrumb: no
            chevron, no trail. It says WHAT this belongs to, which the season
            title alone cannot, without rebuilding the navigation chrome the
            back button already covers. */}
        {parentItem && (
          <Link
            to="/media/$mediaId"
            params={{ mediaId: item.parentId! }}
            className="block mb-2 text-[11px] uppercase tracking-[0.18em] font-semibold text-white/60 hover:text-white transition-colors-fast no-underline"
          >
            {parentItem.title}
          </Link>
        )}
        <div className="flex items-end gap-3 mb-3 max-w-[min(100%,20ch)] sm:max-w-[min(85%,30ch)] lg:max-w-[min(62%,46ch)]">
          <h1
            className="text-3xl sm:text-5xl lg:text-6xl font-medium tracking-[-0.035em] leading-[0.95] text-white text-balance [text-shadow:0_2px_24px_rgb(0_0_0/0.55)]"
            data-testid="media-title"
          >{item.title}</h1>
          {item.id && (
            <button
              onClick={onEditToggle}
              title={t("media.detail.editMetadata")}
              aria-label={t("media.detail.editMetadata")}
              className="flex-shrink-0 p-1.5 rounded-full transition-opacity-fast hover:opacity-80 bg-[var(--overlay-chip-bg)] text-[var(--overlay-text-muted)]"
              data-testid="media-edit-toggle-btn"
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
                className="text-[11px] uppercase tracking-[0.14em] font-medium px-2.5 py-1 border border-white/25 text-white/75 rounded-full"
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
