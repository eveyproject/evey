import { useState } from "react";
import { ChevronDown, Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import type { V1MediaItem } from "@evey/api";
import { useLibraryServiceGetMediaItem, useLibraryServiceListMediaItems } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { formatDuration } from "./mediaDetailUtils";

function SeasonRow({ seasonId }: { seasonId: string }) {
  const { t } = useTranslation();
  const { data: season } = useLibraryServiceGetMediaItem(seasonId);
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: seasonId });
  const [open, setOpen] = useState(false);
  const episodes = [...(episodesData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));

  const seasonLabel = season?.indexNumber != null
    ? t("media.season", { number: season.indexNumber })
    : (season?.title ?? t("media.season", { number: "" }));

  return (
    <div className="mb-2 border border-[var(--border)] rounded-[var(--radius-md)]">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between px-4 py-3 text-left bg-[var(--surface)] rounded-[var(--radius-md)]"
        aria-expanded={open}
      >
        <span className="font-medium text-[var(--text)]">{seasonLabel}</span>
        <span className="text-sm text-[var(--text-muted)]">
          {t("media.detail.ep", { count: episodes.length })}
          <ChevronDown className="inline-block ml-1 transition-transform-fast" size={16} style={{ transform: open ? "rotate(180deg)" : "rotate(0)" }} />
        </span>
      </button>
      {open && (
        <div className="divide-y max-h-96 overflow-y-auto divide-[var(--border)]">
          {episodes.map((ep) => (
            <Link
              key={ep.id}
              to="/player/$mediaId"
              params={{ mediaId: ep.id ?? "" }}
              className="flex items-center gap-3 px-4 py-2 hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
            >
              <span className="text-sm w-10 flex-shrink-0 font-mono font-medium text-[var(--accent)]">
                E{ep.indexNumber ?? "?"}
              </span>
              <span className="text-sm flex-1 truncate">{ep.title}</span>
              {ep.durationMs && (
                <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">{formatDuration(ep.durationMs)}</span>
              )}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export function PlaySeriesButton({ seriesId, onPlay }: { seriesId: string; onPlay: (episodeId: string) => void }) {
  const { t } = useTranslation();
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: seriesId });
  const seasons = [...(seasonsData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  const firstSeason = seasons[0];

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: firstSeason?.id ?? "" },
    { query: { enabled: !!firstSeason?.id } } as never,
  );
  const episodes = [...(episodesData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  const firstEpisode = episodes[0];

  if (!firstEpisode?.id) return null;

  return (
    <Button variant="primary" onClick={() => onPlay(firstEpisode.id!)}>
      <Play size={16} fill="currentColor" />
      {t("media.detail.play")}
    </Button>
  );
}

export function SeriesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: seasonsData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "" });
  const seasons = [...(seasonsData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  if (!seasons.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.detail.seasons")}</h2>
        <Badge variant="info">{t("media.detail.seasonCount", { count: seasons.length })}</Badge>
      </div>
      {seasons.map((s) => s.id ? <SeasonRow key={s.id} seasonId={s.id} /> : null)}
    </div>
  );
}

export function SeasonEpisodesSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const { data: episodesData } = useLibraryServiceListMediaItems({ parentId: item.id ?? "" });
  const episodes = [...(episodesData?.items ?? [])].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  if (!episodes.length) return null;

  return (
    <div className="mt-10">
      <div className="flex items-center gap-3 mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">{t("media.detail.episodes")}</h2>
        <Badge variant="info">{t("media.detail.episodeCount", { count: episodes.length })}</Badge>
      </div>
      <div className="divide-y border border-[var(--border)] rounded-[var(--radius-md)]">
        {episodes.map((ep) => (
          <Link
            key={ep.id}
            to="/player/$mediaId"
            params={{ mediaId: ep.id ?? "" }}
            className="flex items-center gap-3 px-4 py-2 hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
          >
            <span className="text-sm w-10 flex-shrink-0 font-mono font-medium text-[var(--accent)]">
              E{ep.indexNumber ?? "?"}
            </span>
            <span className="text-sm flex-1 truncate">{ep.title}</span>
            {ep.durationMs && (
              <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">{formatDuration(ep.durationMs)}</span>
            )}
          </Link>
        ))}
      </div>
    </div>
  );
}
