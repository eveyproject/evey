import { useMemo, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, itemLabel } from "@evey/api";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { Pencil } from "lucide-react";
import { ACTION_ICON_BOX } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";
import { BackButton } from "@/components/ui/BackButton";
import { GenreDot } from "@/components/ui/GenreDot";
import { PosterPlaceholder } from "./PosterPlaceholder";
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
  /** Rendered in the right column, under the genres - the synopsis, credits
      and action buttons. It belongs to the same flex row as the poster so the
      poster's full height is part of the row and the sections that follow
      clear it on their own. */
  children?: ReactNode;
}

export function MediaDetailHero({
  item,
  backdropUrl,
  posterUrl,
  duration,
  parentItem,
  onEditToggle,
  children,
}: MediaDetailHeroProps) {
  const { t, i18n } = useTranslation();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } });
  const profileRegion = (profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0])?.region;
  const certInfo = certInfoForLocale(item.certificationsByCountry, i18n.language, item.officialRating, profileRegion);
  const label = itemLabel(item, t);

  const backdropSrcSet = useMemo(() => {
    if (!backdropUrl) return undefined;
    const sep = backdropUrl.includes("?") ? "&" : "?";
    // These widths must EXIST server-side: /artwork snaps ?w= to the nearest
    // allowed variant, so before 1280/1920 were added every entry here
    // collapsed onto the same 780px file and the browser stretched it across
    // the full viewport. That was the "pixelated backdrop".
    return `${backdropUrl}${sep}w=780 780w, ${backdropUrl}${sep}w=1280 1280w, ${backdropUrl}${sep}w=1920 1920w`;
  }, [backdropUrl]);

  const posterSrcSet = useMemo(() => {
    if (!posterUrl) return undefined;
    const sep = posterUrl.includes("?") ? "&" : "?";
    // Use the widths the server actually has (185/342/500): w=300 and w=400
    // both snapped to 342, so the third entry was a duplicate and a retina
    // poster never got a sharper file.
    return `${posterUrl}${sep}w=185 185w, ${posterUrl}${sep}w=342 342w, ${posterUrl}${sep}w=500 500w`;
  }, [posterUrl]);

  return (
    // The hero is a positioning context, not a fixed band: the backdrop is a
    // clipped layer behind, and the poster + text row flows normally on top of
    // it. That is what lets the poster overhang the artwork without being cut
    // off, and what makes the sections below clear it automatically.
    <div className="evey-media-hero relative w-full -mx-4 sm:-mx-6 -mt-4 sm:-mt-6 mb-0">
      {/* Backdrop layer. Height follows the backdrop's own 16:9 shape instead
          of a fixed band: the old clamp capped at 420px while the image spans
          the full width, so on a wide viewport object-cover had to crop ~40% of
          the frame vertically - taking the top and bottom of the composition
          with it. Bounded so it never eats the whole screen. */}
      <div className="absolute inset-x-0 top-0 h-[var(--media-hero-h)] overflow-hidden">
        {backdropUrl ? (
          <img src={backdropUrl} srcSet={backdropSrcSet} sizes="100vw" alt={label} className="w-full h-full object-cover object-center" data-testid="media-backdrop" onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }} />
        ) : (
          <div className="w-full h-full bg-[var(--surface)]" />
        )}
        <div className="absolute inset-0 gradient-hero-fade" />
        <div className="absolute inset-0 gradient-hero-overlay" />
      </div>

      {/* Back button only. The breadcrumb repeated the hierarchy the title and
          the parent link already carry, and cost a line of chrome over the
          artwork on every media page. */}
      <div className="absolute top-0 left-0 right-0 z-raised px-4 sm:px-6 pt-4 sm:pt-5 gradient-hero-scrim">
        <BackButton fallback="/library" className="!text-white hover:!text-white/80" />
      </div>

      {/* A spacer, NOT a padding-top on the row below: a padding made that row's
          box span the whole backdrop, and since it comes later in the DOM at the
          same stacking level it swallowed every click aimed at the back button
          sitting on the artwork. */}
      <div aria-hidden className="h-[var(--media-hero-content-top)]" />

      {/* Poster and text are ONE flex row, so items-start aligns the poster's
          top edge with the metadata line instead of the two being separate
          blocks nudged at each other with negative margins. The row starts at
          the backdrop's bottom minus --media-hero-overlap, so it sits in slight
          superimposition over the artwork. */}
      <div className="relative z-raised px-4 sm:px-6">
        <div className="flex gap-8 lg:gap-10 items-start">
          {/* The poster grows with the viewport but stops at 25rem: the server
              only stores poster variants up to 500px wide, so a box wider than
              that starts upscaling a 500px file instead of getting a sharper
              one. */}
          <div className="flex-shrink-0 w-80 lg:w-96 xl:w-[25rem] hidden md:block">
            <div className="evey-poster relative w-full overflow-hidden rounded-[var(--radius-md)] aspect-[2/3] bg-[var(--surface-raised)] shadow-[var(--shadow-lg)] ring-1 ring-[var(--border)]">
              {posterUrl ? (
                <img
                  src={posterUrl}
                  srcSet={posterSrcSet}
                  sizes="(min-width: 1280px) 400px, (min-width: 1024px) 384px, 320px"
                  alt={label}
                  fetchPriority="high"
                  className="w-full h-full object-cover"
                  data-testid="media-poster"
                  onError={(e) => {
                    const t = e.currentTarget as HTMLImageElement;
                    t.style.display = "none";
                    const fb = t.nextElementSibling as HTMLElement | null;
                    if (fb) fb.style.display = "flex";
                  }}
                />
              ) : null}
              <PosterPlaceholder
                title={label}
                type={item.type}
                iconSize={36}
                className="absolute inset-0"
                style={{ display: posterUrl ? "none" : "flex" }}
              />
            </div>
          </div>

          <div className="flex-1 min-w-0">
        <div className="flex items-center gap-x-3 gap-y-2 mb-3 flex-wrap [&>*+*]:before:content-[''] [&>*+*]:before:hidden">
          {certInfo && (
            <span
              className="text-xs font-semibold tracking-[0.08em] px-2 py-0.5 border border-white/30 text-white/80 rounded-[var(--radius-sm)]"
              title={`${t(`media.certBody.${certInfo.country}`, { defaultValue: certInfo.country })} ${certInfo.rating}`}
            >
              {t(`media.certBody.${certInfo.country}`, { defaultValue: certInfo.country })} {certInfo.rating}
            </span>
          )}
          {duration && (
            <span className="text-base tabular-nums text-white/70">{duration}</span>
          )}
          {item.year && (
            <span className="text-base tabular-nums text-white/70">{item.year}</span>
          )}
          {item.communityRating !== undefined && item.communityRating > 0 && (
            <span className="flex items-center gap-1.5">
              <Stars rating={item.communityRating} />
              <span className="text-base tabular-nums text-white/70">{item.communityRating.toFixed(1)}</span>
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
            className="block mb-2 text-xs uppercase tracking-[0.18em] font-semibold text-white/60 hover:text-white transition-colors-fast no-underline"
          >
            {itemLabel(parentItem, t)}
          </Link>
        )}
        {/* The measure lives on the h1, not on the wrapper: in ch it scales
            with the element's own font-size, and on the wrapper it resolved
            against the inherited 16px instead of the display size - which
            capped the title around 368px however large the type got. */}
        <div className="flex items-end gap-3 mb-3">
          <h1
            className="max-w-[14ch] text-4xl sm:text-6xl lg:text-7xl font-medium tracking-[-0.035em] leading-[0.95] text-white text-balance [text-shadow:0_2px_24px_rgb(0_0_0/0.55)]"
            data-testid="media-title"
          >{label}</h1>
          {/* Client-side gate only: the server is what actually enforces this,
              this just stops a non-admin from seeing controls that would fail
              anyway (parity with the mobile detail screen). */}
          {item.id && isAdmin && (
            <button
              onClick={onEditToggle}
              title={t("media.detail.editMetadata")}
              aria-label={t("media.detail.editMetadata")}
              className={`${ACTION_ICON_BOX} rounded-full transition-opacity-fast hover:opacity-80 bg-[var(--overlay-chip-bg)] text-[var(--overlay-text-muted)]`}
              data-testid="media-edit-toggle-btn"
            >
              <Pencil size={ICON.xl} aria-hidden="true" />
            </button>
          )}
        </div>
        {item.genres && item.genres.length > 0 && (
          <div className="flex flex-wrap gap-2" data-testid="media-genres">
            {item.genres.map((g) => (
              <span
                key={g}
                className="text-xs uppercase tracking-[0.14em] font-medium px-3 py-1.5 border border-white/25 text-white/75 rounded-full inline-flex items-center gap-1.5"
              >
                <GenreDot label={g} />
                {g}
              </span>
            ))}
          </div>
        )}
            {children ? <div className="mt-6">{children}</div> : null}
          </div>
        </div>
      </div>
    </div>
  );
}
