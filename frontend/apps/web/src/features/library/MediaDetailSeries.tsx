import { useEffect, useState } from "react";
import { ICON } from "@/constants/iconSizes";
import { Check, Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import type { V1MediaItem } from "@evey/api";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceMarkAsWatched,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { formatDurationMs, seasonLabel } from "@evey/api";
import { artworkUrl } from "./artworkUrl";
import { useSeriesPlayTarget, useWatchProgressMaps } from "@evey/client";
import { DownloadButton } from "@/features/playback/DownloadButton";
import { useStreamToken } from "@/hooks/useStreamToken";
import { useDemoLock } from "@/hooks/useDemoMode";
import { invalidateWatchedState } from "@/lib/invalidateWatchedState";

function sortByIndex(items: V1MediaItem[]): V1MediaItem[] {
  return [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
}

// EpisodeWatchedToggle marks one episode watched or unwatched from the season
// list, the way the card's badge does for a film. onMarked shows the answer at
// once; the shared invalidation then refreshes the history map the row reads,
// the series' Play target and the home rows.
function EpisodeWatchedToggle({ ep, isWatched, onMarked }: { ep: V1MediaItem; isWatched: boolean; onMarked: (watched: boolean) => void }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const done = (next: boolean) => () => {
    onMarked(next);
    toast.success(t(next ? "media.contextMenu.markedWatched" : "media.contextMenu.markedUnwatched"));
    invalidateWatchedState(qc, ep.id);
  };
  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: done(true) } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: done(false) } });
  const label = t(isWatched ? "media.contextMenu.markUnwatched" : "media.contextMenu.markWatched");

  return (
    <button
      type="button"
      {...lock}
      title={lock.title ?? label}
      aria-label={label}
      aria-pressed={isWatched}
      data-testid="episode-watched-toggle"
      disabled={lock.disabled || markWatched.isPending || markUnwatched.isPending}
      onClick={() => {
        if (!ep.id) return;
        if (!profileId) {
          toast.error(t("media.detail.noProfileSelected"));
          return;
        }
        const vars = { mediaItemId: ep.id, params: { profileId } };
        if (isWatched) markUnwatched.mutate(vars);
        else markWatched.mutate(vars);
      }}
      className={`tap-hit w-9 h-9 rounded-full flex items-center justify-center transition-colors-fast cursor-pointer disabled:cursor-not-allowed disabled:opacity-60 ${
        isWatched
          ? "bg-[var(--color-success)] text-white"
          : "border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-raised)]"
      }`}
    >
      <Check size={ICON.md} strokeWidth={3} aria-hidden="true" />
    </button>
  );
}

// EpisodeRow shows the episode as a card: still, number + title, synopsis, a
// play control layered on the image, and the watched toggle and download on
// the right.
//
// The row deliberately no longer links to a per-episode detail page. That page
// existed but had nothing an episode needs that isn't already here - the
// synopsis and the actions now live on the row itself - so it was one click of
// dead weight between the viewer and playback. Seasons keep their own page.
//
// The progress comes from the section's one watch-history fetch, as on mobile.
// A fully watched episode has its resume position reset, so its percent says
// nothing: played gets the check, the bar is for an episode left mid-way.
function EpisodeRow({ ep, percent, watched }: { ep: V1MediaItem; percent: number; watched: boolean }) {
  const { t } = useTranslation();
  const { fetchStreamToken } = useStreamToken();
  // A mark from the toggle, shown before the history map refetches; the
  // server's value takes over again as soon as it changes.
  const [marked, setMarked] = useState<boolean | null>(null);
  useEffect(() => setMarked(null), [watched]);
  const isWatched = marked ?? watched;
  const stillUrl = ep.id ? artworkUrl(ep.id, "backdrop", ep) : undefined;
  const stillSrcSet = stillUrl
    ? `${stillUrl}${stillUrl.includes("?") ? "&" : "?"}w=342 342w, ${stillUrl}${stillUrl.includes("?") ? "&" : "?"}w=500 500w`
    : undefined;

  return (
    <div className="flex min-w-0 gap-3 sm:gap-5 p-3 sm:p-4 hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]">
      {/* Still + play overlay. The whole thumbnail is the play target: it is the
          biggest, most obvious hit area and it is what the viewer wants. */}
      <Link
        to="/player/$mediaId"
        params={{ mediaId: ep.id ?? "" }}
        aria-label={t("media.detail.play")}
        title={t("media.detail.play")}
        className="relative flex-shrink-0 w-32 sm:w-60 lg:w-72 aspect-video rounded-[var(--radius-sm)] overflow-hidden bg-[var(--surface-raised)] group"
      >
        {stillUrl && (
          <img
            src={stillUrl}
            srcSet={stillSrcSet}
            sizes="(min-width: 1024px) 288px, (min-width: 640px) 240px, 128px"
            alt=""
            loading="lazy"
            className="w-full h-full object-cover"
            onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = "none"; }}
          />
        )}
        <span className="absolute inset-0 flex items-center justify-center bg-black/30 opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity-fast">
          <Play size={ICON["2xl"]} fill="currentColor" className="text-white" />
        </span>
        {isWatched && (
          <span
            data-testid="episode-watched"
            title={t("common.watched")}
            aria-label={t("common.watched")}
            className="absolute top-1.5 right-1.5 w-6 h-6 rounded-full flex items-center justify-center bg-[var(--color-success)] text-white"
          >
            <Check size={ICON.sm} strokeWidth={3} aria-hidden="true" />
          </span>
        )}
        {!isWatched && percent > 0 && (
          <span
            data-testid="episode-progress"
            role="progressbar"
            aria-valuenow={Math.round(percent)}
            aria-valuemin={0}
            aria-valuemax={100}
            className="absolute bottom-0 left-0 right-0 h-1.5 bg-[var(--overlay-bg)]"
          >
            <span className="block h-full bg-[var(--accent)]" style={{ width: `${percent}%` }} />
          </span>
        )}
      </Link>

      <div className="flex-1 min-w-0">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <span className="text-base flex-shrink-0 font-mono font-medium text-[var(--accent)]">
            E{ep.indexNumber ?? "?"}
          </span>
          <span className="text-base font-medium truncate">{ep.title}</span>
          {ep.durationMs && (
            <span className="text-sm flex-shrink-0 ml-auto text-[var(--text-muted)]">{formatDurationMs(ep.durationMs)}</span>
          )}
        </div>
        {ep.overview && (
          <p className="mt-2 text-sm leading-relaxed line-clamp-3 text-[var(--text-secondary)]">{ep.overview}</p>
        )}
      </div>

      <div className="flex-shrink-0 self-center flex items-center gap-2">
        <EpisodeWatchedToggle ep={ep} isWatched={isWatched} onMarked={setMarked} />
        {/* Mints its own media-scoped token at click time, like the season and
            series ZIP buttons: the account JWT must never ride in a URL. */}
        <DownloadButton
          mediaId={ep.id ?? ""}
          title={`${ep.title ?? "episode"}`}
          mintToken={() => fetchStreamToken(ep.id ?? "")}
          size="icon"
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
// Episodes have no server-side sort key: SortColumnMap knows title, added_at,
// year, premiere_date and community_rating, but not the episode index. So the
// server answers in sort_title order and every client re-sorts by index itself
// - which only works if the client holds the WHOLE season. These queries used
// to send no pagination at all, landing on the handler's default of 50
// (clampPagination, grpcapi/library_handler_mediaitems.go:203).
//
// Past 50 episodes that was not merely a short list: the 50 rows came back
// alphabetically, so sorting them by index client-side and taking [0] picked
// whichever episode happened to sort first by TITLE. "Play season" then started
// mid-season. The players already ask for 500 for the same query; this brings
// the detail page in line. Seasons-of-a-series queries keep the default - no
// series has 50 seasons.
const SEASON_EPISODES_PAGE_SIZE = 500;

// Seasons, not episodes. Well beyond any real series, and stated rather than
// left to the handler default: an omitted page size reads the same whether it
// was decided or forgotten, which is how three screens on this path ended up
// truncated without anyone noticing.
const SEASONS_PAGE_SIZE = 100;

// where the episode list now lives.
function SeasonCard({ seasonId }: { seasonId: string }) {
  const { t } = useTranslation();
  const { data: season } = useLibraryServiceGetMediaItem(seasonId);
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId, "pagination.pageSize": SEASON_EPISODES_PAGE_SIZE });
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  const label = seasonLabel(season ?? {}, t);

  const posterUrl = artworkUrl(seasonId, "poster", season);
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
          // pointer-events-none until it is actually revealed. `opacity-0` still
          // receives taps, and a phone never hovers - so this invisible overlay
          // was swallowing the tap on the poster and starting the first episode
          // instead of opening the season. Verified on a phone emulation.
          className="absolute inset-x-0 top-0 aspect-[2/3] flex items-center justify-center bg-black/40 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto transition-opacity-fast rounded-[var(--radius-md)]"
        >
          <Play size={ICON["2xl"]} fill="currentColor" className="text-white" />
        </Link>
      )}
      {/* The way into a season with a pointer. The poster above is covered by
          the play overlay as soon as the card is hovered (deliberately - see
          its comment), so this label is what actually opens the season. */}
      <Link
        to="/media/$mediaId"
        params={{ mediaId: seasonId }}
        data-testid="season-link"
        className="block mt-2 text-base font-medium truncate text-[var(--text)] hover:text-[var(--accent)] transition-colors-fast"
      >
        {label}
      </Link>
      <p className="text-sm text-[var(--text-muted)]">
        {t("media.detail.ep", { count: episodes.length })}
      </p>
    </div>
  );
}

// Play on a series starts where the profile is in it (the server's answer:
// the unfinished episode, else the next one across seasons). The first
// episode stays the fallback while that answer is out, or from a server too
// old to give one.
export function PlaySeriesButton({ seriesId, onPlay }: { seriesId: string; onPlay: (episodeId: string) => void }) {
  const { t } = useTranslation();
  const target = useSeriesPlayTarget(seriesId);
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: seriesId, "pagination.pageSize": SEASONS_PAGE_SIZE });
  const seasons = sortByIndex(seasonsData?.items ?? []);
  const firstSeason = seasons[0];

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: firstSeason?.id ?? "", "pagination.pageSize": SEASON_EPISODES_PAGE_SIZE },
    { query: { enabled: !!firstSeason?.id } } as never,
  );
  const episodes = sortByIndex(episodesData?.items ?? []);
  const episode = target.episode ?? episodes[0];

  if (!episode?.id) return null;

  return (
    <Button variant="primary" size="lg" data-testid="play-series-button" title={episode.title} onClick={() => onPlay(episode.id!)}>
      <Play size={ICON.md} fill="currentColor" />
      {target.started ? t("media.detail.resume") : t("media.detail.play")}
    </Button>
  );
}

export function PlaySeasonButton({ seasonId, onPlay }: { seasonId: string; onPlay: (episodeId: string) => void }) {
  const { t } = useTranslation();
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId, "pagination.pageSize": SEASON_EPISODES_PAGE_SIZE });
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  if (!firstEpisode?.id) return null;

  return (
    <Button variant="primary" size="lg" data-testid="play-button" onClick={() => onPlay(firstEpisode.id!)}>
      <Play size={ICON.md} fill="currentColor" />
      {t("media.detail.play")}
    </Button>
  );
}

export function SeriesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "", "pagination.pageSize": SEASONS_PAGE_SIZE });
  const seasons = sortByIndex(seasonsData?.items ?? []);
  if (!seasons.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-xs uppercase tracking-[0.18em] font-semibold text-[var(--text-secondary)]">{t("media.detail.seasons")}</h2>
        <span className="text-xs tabular-nums text-[var(--text-muted)]">{t("media.detail.seasonCount", { count: seasons.length })}</span>
      </div>
      <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(200px,1fr))]">
        {seasons.map((s) => (s.id ? <SeasonCard key={s.id} seasonId={s.id} /> : null))}
      </div>
    </div>
  );
}

export function SeasonEpisodesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "", "pagination.pageSize": SEASON_EPISODES_PAGE_SIZE });
  const episodes = sortByIndex(episodesData?.items ?? []);
  const { watchedMap, percentMap } = useWatchProgressMaps();
  if (!episodes.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-xs uppercase tracking-[0.18em] font-semibold text-[var(--text-secondary)]">{t("media.detail.episodes")}</h2>
        <span className="text-xs tabular-nums text-[var(--text-muted)]">{t("media.detail.episodeCount", { count: episodes.length })}</span>
      </div>
      {/* Bounded width: a row stretched across the whole page left the synopsis
          running for 150+ characters and the download button marooned far from
          the content it acts on. Capped, the still, the text and the action read
          as one object. */}
      <div className="grid xl:grid-cols-2 gap-x-6 [&>*]:border-b [&>*]:border-[var(--border)] border-t border-[var(--border)]">
        {episodes.map((ep) => (ep.id ? (
          <EpisodeRow key={ep.id} ep={ep} percent={percentMap.get(ep.id) ?? 0} watched={watchedMap.get(ep.id) === true} />
        ) : null))}
      </div>
    </div>
  );
}
