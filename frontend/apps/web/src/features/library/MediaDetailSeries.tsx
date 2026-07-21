import { useState } from "react";
import { Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useLibraryServiceGetMediaItem, useLibraryServiceListMediaItems } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { getSeriesTrackPrefs, setSeriesTrackPrefs } from "@/hooks/usePlaybackPrefs";
import { formatDuration } from "./mediaDetailUtils";
import { artworkUrl } from "./artworkUrl";
import { useAuthStore } from "@evey/store";
import { DownloadButton } from "@/features/playback/DownloadButton";
import { useStreamToken } from "@/hooks/useStreamToken";

// GaplessToggle: "lecture enchaînée" - request the server's concatenated,
// multi-episode HLS session instead of reloading the player at each episode
// boundary. Per-series (localStorage), opt-in, off by default. The actual
// packaging (and its own eligibility/compatibility checks) happens server-side
// per playback session - this toggle only records the user's intent.
function GaplessToggle({ seriesId }: { seriesId: string }) {
  const { t } = useTranslation();
  const [gapless, setGapless] = useState(() => !!getSeriesTrackPrefs(seriesId)?.gapless);

  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-[var(--text-muted)]">{t("media.detail.gaplessPlayback")}</span>
      <Switch
        checked={gapless}
        data-testid="gapless-toggle"
        label={t("media.detail.gaplessPlayback")}
        onChange={() => {
          const next = !gapless;
          setGapless(next);
          const cur = getSeriesTrackPrefs(seriesId) ?? {};
          setSeriesTrackPrefs(seriesId, { ...cur, gapless: next });
        }}
      />
    </div>
  );
}

function sortByIndex(items: V1MediaItem[]): V1MediaItem[] {
  return [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
}

// EpisodeRow shows the episode as a card: still, number + title, synopsis, a
// play control layered on the image, and a download on the right.
//
// The row deliberately no longer links to a per-episode detail page. That page
// existed but had nothing an episode needs that isn't already here - the
// synopsis and the actions now live on the row itself - so it was one click of
// dead weight between the viewer and playback. Seasons keep their own page.
function EpisodeRow({ ep }: { ep: V1MediaItem }) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const { fetchStreamToken } = useStreamToken();
  const stillUrl = ep.id ? artworkUrl(ep.id, "backdrop", ep, accessToken || undefined) : undefined;
  const stillSrcSet = stillUrl
    ? `${stillUrl}${stillUrl.includes("?") ? "&" : "?"}w=342 342w, ${stillUrl}${stillUrl.includes("?") ? "&" : "?"}w=500 500w`
    : undefined;

  return (
    <div className="flex gap-4 p-3 hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]">
      {/* Still + play overlay. The whole thumbnail is the play target: it is the
          biggest, most obvious hit area and it is what the viewer wants. */}
      <Link
        to="/player/$mediaId"
        params={{ mediaId: ep.id ?? "" }}
        aria-label={t("media.detail.play")}
        title={t("media.detail.play")}
        className="relative flex-shrink-0 w-52 sm:w-64 aspect-video rounded-[var(--radius-sm)] overflow-hidden bg-[var(--surface-raised)] group"
      >
        {stillUrl && (
          <img
            src={stillUrl}
            srcSet={stillSrcSet}
            sizes="256px"
            alt=""
            loading="lazy"
            className="w-full h-full object-cover"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        )}
        <span className="absolute inset-0 flex items-center justify-center bg-black/30 opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity-fast">
          <Play size={28} fill="currentColor" className="text-white" />
        </span>
      </Link>

      <div className="flex-1 min-w-0">
        <div className="flex items-baseline gap-2">
          <span className="text-base flex-shrink-0 font-mono font-medium text-[var(--accent)]">
            E{ep.indexNumber ?? "?"}
          </span>
          <span className="text-base font-medium truncate">{ep.title}</span>
          {ep.durationMs && (
            <span className="text-sm flex-shrink-0 ml-auto text-[var(--text-muted)]">{formatDuration(ep.durationMs)}</span>
          )}
        </div>
        {ep.overview && (
          <p className="mt-1.5 text-sm leading-relaxed line-clamp-3 text-[var(--text-secondary)]">{ep.overview}</p>
        )}
      </div>

      <div className="flex-shrink-0 self-center">
        {/* Mints its own media-scoped token at click time, like the season and
            series ZIP buttons: the account JWT must never ride in a URL. */}
        <DownloadButton
          mediaId={ep.id ?? ""}
          title={`${ep.title ?? "episode"}`}
          mintToken={() => fetchStreamToken(ep.id ?? "")}
          label=""
        />
      </div>
    </div>
  );
}

// SeasonCard is a poster tile per season rather than a text accordion row.
//
// The accordion made every season look the same - a line of text - and hid the
// episodes one click deep. A season has its own artwork, so use it: the grid is
// scannable at a glance and the tile navigates to the season page, which is
// where the episode list now lives.
function SeasonCard({ seasonId }: { seasonId: string }) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data: season } = useLibraryServiceGetMediaItem(seasonId);
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId });
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  const seasonLabel = season?.indexNumber != null
    ? t("media.season", { number: season.indexNumber })
    : (season?.title ?? t("media.season", { number: "" }));

  const posterUrl = artworkUrl(seasonId, "poster", season, accessToken || undefined);
  const posterSrcSet = posterUrl
    ? `${posterUrl}${posterUrl.includes("?") ? "&" : "?"}w=185 185w, ${posterUrl}${posterUrl.includes("?") ? "&" : "?"}w=342 342w`
    : undefined;

  return (
    <div className="group relative">
      <Link
        to="/media/$mediaId"
        params={{ mediaId: seasonId }}
        className="block rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] aspect-[2/3] border border-[var(--border)]"
      >
        {posterUrl && (
          <img
            src={posterUrl}
            srcSet={posterSrcSet}
            sizes="234px"
            alt=""
            loading="lazy"
            className="w-full h-full object-cover transition-transform-fast group-hover:scale-105"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        )}
      </Link>
      {firstEpisode?.id && (
        <Link
          to="/player/$mediaId"
          params={{ mediaId: firstEpisode.id }}
          aria-label={t("media.detail.play")}
          title={t("media.detail.play")}
          className="absolute inset-x-0 top-0 aspect-[2/3] flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity-fast rounded-[var(--radius-md)]"
        >
          <Play size={32} fill="currentColor" className="text-white" />
        </Link>
      )}
      <Link
        to="/media/$mediaId"
        params={{ mediaId: seasonId }}
        className="block mt-2 text-base font-medium truncate text-[var(--text)] hover:text-[var(--accent)] transition-colors-fast"
      >
        {seasonLabel}
      </Link>
      <p className="text-sm text-[var(--text-muted)]">
        {t("media.detail.ep", { count: episodes.length })}
      </p>
    </div>
  );
}

export function PlaySeriesButton({ seriesId, onPlay }: { seriesId: string; onPlay: (episodeId: string) => void }) {
  const { t } = useTranslation();
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: seriesId });
  const seasons = sortByIndex(seasonsData?.items ?? []);
  const firstSeason = seasons[0];

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: firstSeason?.id ?? "" },
    { query: { enabled: !!firstSeason?.id } } as never,
  );
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  if (!firstEpisode?.id) return null;

  return (
    <Button variant="primary" onClick={() => onPlay(firstEpisode.id!)}>
      <Play size={16} fill="currentColor" />
      {t("media.detail.play")}
    </Button>
  );
}

export function PlaySeasonButton({ seasonId, onPlay }: { seasonId: string; onPlay: (episodeId: string) => void }) {
  const { t } = useTranslation();
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId });
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  if (!firstEpisode?.id) return null;

  return (
    <Button variant="primary" data-testid="play-button" onClick={() => onPlay(firstEpisode.id!)}>
      <Play size={16} fill="currentColor" />
      {t("media.detail.play")}
    </Button>
  );
}

export function SeriesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "" });
  const seasons = sortByIndex(seasonsData?.items ?? []);
  if (!seasons.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.detail.seasons")}</h2>
          <Badge variant="info">{t("media.detail.seasonCount", { count: seasons.length })}</Badge>
        </div>
        {item.id && <GaplessToggle seriesId={item.id} />}
      </div>
      <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(182px,1fr))]">
        {seasons.map((s) => (s.id ? <SeasonCard key={s.id} seasonId={s.id} /> : null))}
      </div>
    </div>
  );
}

export function SeasonEpisodesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "" });
  const episodes = sortByIndex(episodesData?.items ?? []);
  if (!episodes.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.detail.episodes")}</h2>
        <Badge variant="info">{t("media.detail.episodeCount", { count: episodes.length })}</Badge>
      </div>
      {/* Bounded width: a row stretched across the whole page left the synopsis
          running for 150+ characters and the download button marooned far from
          the content it acts on. Capped, the still, the text and the action read
          as one object. */}
      <div className="max-w-3xl divide-y border border-[var(--border)] rounded-[var(--radius-md)]">
        {episodes.map((ep) => (ep.id ? <EpisodeRow key={ep.id} ep={ep} /> : null))}
      </div>
    </div>
  );
}
