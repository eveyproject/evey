import { useState } from "react";
import { ChevronDown, Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useLibraryServiceGetMediaItem, useLibraryServiceListMediaItems } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { getSeriesTrackPrefs, setSeriesTrackPrefs } from "@/hooks/usePlaybackPrefs";
import { formatDuration } from "./mediaDetailUtils";

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

// EpisodeRow links the row to the episode's media page and exposes a separate
// play control that jumps straight to the player.
function EpisodeRow({ ep }: { ep: V1MediaItem }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]">
      <Link
        to="/media/$mediaId"
        params={{ mediaId: ep.id ?? "" }}
        className="flex items-center gap-3 px-4 py-2 flex-1 min-w-0"
      >
        <span className="text-sm w-10 flex-shrink-0 font-mono font-medium text-[var(--accent)]">
          E{ep.indexNumber ?? "?"}
        </span>
        <span className="text-sm flex-1 truncate">{ep.title}</span>
        {ep.durationMs && (
          <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">{formatDuration(ep.durationMs)}</span>
        )}
      </Link>
      <Link
        to="/player/$mediaId"
        params={{ mediaId: ep.id ?? "" }}
        aria-label={t("media.detail.play")}
        title={t("media.detail.play")}
        className="flex items-center justify-center px-4 py-2 flex-shrink-0 text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors-fast"
      >
        <Play size={16} fill="currentColor" />
      </Link>
    </div>
  );
}

function SeasonRow({ seasonId }: { seasonId: string }) {
  const { t } = useTranslation();
  const { data: season } = useLibraryServiceGetMediaItem(seasonId);
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId });
  const [open, setOpen] = useState(false);
  const episodes = sortByIndex(episodesData?.items ?? []);
  const firstEpisode = episodes[0];

  const seasonLabel = season?.indexNumber != null
    ? t("media.season", { number: season.indexNumber })
    : (season?.title ?? t("media.season", { number: "" }));

  return (
    <div className="mb-2 border border-[var(--border)] rounded-[var(--radius-md)]">
      <div className="w-full flex items-center justify-between gap-2 px-4 py-3 bg-[var(--surface)] rounded-[var(--radius-md)]">
        <Link
          to="/media/$mediaId"
          params={{ mediaId: seasonId }}
          className="font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors-fast truncate min-w-0 flex-1"
        >
          {seasonLabel}
        </Link>
        <div className="flex items-center gap-3 flex-shrink-0">
          {firstEpisode?.id && (
            <Link
              to="/player/$mediaId"
              params={{ mediaId: firstEpisode.id }}
              aria-label={t("media.detail.play")}
              title={t("media.detail.play")}
              className="flex items-center justify-center text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors-fast"
            >
              <Play size={16} fill="currentColor" />
            </Link>
          )}
          <button
            onClick={() => setOpen((v) => !v)}
            className="flex items-center gap-1 text-sm text-[var(--text-muted)] bg-transparent border-none cursor-pointer"
            aria-expanded={open}
            aria-label={t("media.detail.episodes")}
          >
            {t("media.detail.ep", { count: episodes.length })}
            <ChevronDown className="transition-transform-fast" size={16} style={{ transform: open ? "rotate(180deg)" : "rotate(0)" }} />
          </button>
        </div>
      </div>
      {open && (
        <div className="divide-y max-h-96 overflow-y-auto divide-[var(--border)]">
          {episodes.map((ep) => (ep.id ? <EpisodeRow key={ep.id} ep={ep} /> : null))}
        </div>
      )}
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
      {seasons.map((s) => (s.id ? <SeasonRow key={s.id} seasonId={s.id} /> : null))}
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
      <div className="divide-y border border-[var(--border)] rounded-[var(--radius-md)]">
        {episodes.map((ep) => (ep.id ? <EpisodeRow key={ep.id} ep={ep} /> : null))}
      </div>
    </div>
  );
}
