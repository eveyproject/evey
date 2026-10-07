import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { CalendarDays, ExternalLink, Star } from "lucide-react";
import { externalTitleSite, externalTitleUrl, formatRating, formatReleaseDate } from "@evey/api";
import { ICON } from "@/constants/iconSizes";

// The three things a search result shows and a request keeps: a link to the
// provider's page in the viewer's language, the release date and the rating,
// and the synopsis. Shared by the search grid, the requester's list and the
// operator's queue so the three read the same.

type TitleRef = { provider?: string; providerId?: string; mediaType?: string };

/**
 * Wraps the title (or poster) in a link to its TMDB / IMDb / MusicBrainz page,
 * opened in a new tab. Rendered as a plain span when the request names no
 * provider id - a free-text request has nowhere to go.
 */
export function ExternalTitleLink({ title, children, className = "" }: { title: TitleRef; children: ReactNode; className?: string }) {
  const { t, i18n } = useTranslation();
  const url = externalTitleUrl(title, i18n?.language);
  const site = externalTitleSite(title.provider);
  if (!url || !site) return <span className={className}>{children}</span>;
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      title={t("requests.openOn", { site })}
      aria-label={t("requests.openOn", { site })}
      className={`inline-flex items-center gap-1 hover:text-[var(--accent)] hover:underline ${className}`}
      data-testid="request-external-link"
    >
      {children}
      <ExternalLink size={ICON.xs} aria-hidden="true" className="shrink-0 opacity-70" />
    </a>
  );
}

/** Release date and rating on one muted line; nothing when both are unknown. */
export function RequestFacts({ releaseDate, rating, className = "" }: { releaseDate?: string; rating?: number; className?: string }) {
  const { t, i18n } = useTranslation();
  const date = formatReleaseDate(releaseDate, i18n?.language);
  const score = formatRating(rating);
  if (!date && !score) return null;
  return (
    <p className={`flex items-center gap-3 text-xs text-[var(--text-muted)] ${className}`} data-testid="request-facts">
      {date && (
        <span className="inline-flex items-center gap-1" title={t("requests.releaseDateLabel", { date })}>
          <CalendarDays size={ICON.xs} aria-hidden="true" />
          <span className="sr-only">{t("requests.releaseDateLabel", { date })}</span>
          <span aria-hidden="true">{date}</span>
        </span>
      )}
      {score && (
        <span className="inline-flex items-center gap-1" title={t("requests.ratingLabel", { rating: score })}>
          <Star size={ICON.xs} aria-hidden="true" className="fill-current" />
          <span className="sr-only">{t("requests.ratingLabel", { rating: score })}</span>
          <span aria-hidden="true">{score}</span>
        </span>
      )}
    </p>
  );
}

/** The synopsis, clamped to a few lines; nothing when the provider had none. */
export function RequestOverview({ overview, lines = 3, className = "" }: { overview?: string; lines?: 2 | 3 | 4; className?: string }) {
  const text = overview?.trim();
  if (!text) return null;
  const clamp = lines === 2 ? "line-clamp-2" : lines === 4 ? "line-clamp-4" : "line-clamp-3";
  return (
    <p className={`text-xs text-[var(--text-muted)] ${clamp} ${className}`} title={text} data-testid="request-overview">
      {text}
    </p>
  );
}
