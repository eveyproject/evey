import { useCallback, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { episodeLabel } from "@evey/api";
import { COUNTDOWN_SEC, useUpNextCountdown } from "@evey/client";

interface NextEpisodeOverlayProps {
  nextEpisode: { id: string; title?: string; indexNumber?: number; seasonIndexNumber?: number } | null;
  currentTime: number;
  duration: number;
  onPlayNow: (id: string) => void;
  onCancel: () => void;
  /** When true, never show the overlay or auto-advance - e.g. the viewer set
   * a "stop at end of episode" sleep timer, which should just stop playback
   * instead of offering to continue. */
  suppressAutoAdvance?: boolean;
  /** The profile's choice: start the next episode on its own once this one
   *  ends. Off keeps the card and its "Play now" button, drops the countdown.
   *  Defaults to on, the behaviour every player had before the choice existed. */
  autoAdvance?: boolean;
  /** Fires whenever the card appears or goes, so a sibling overlay (the skip
   *  button, same corner) can step aside instead of stacking on it. */
  onVisibleChange?: (visible: boolean) => void;
}


export function NextEpisodeOverlay({
  nextEpisode,
  currentTime,
  duration,
  onPlayNow,
  onCancel,
  suppressAutoAdvance = false,
  autoAdvance = true,
  onVisibleChange,
}: NextEpisodeOverlayProps) {
  const { t } = useTranslation();
  // The card's life (credits, end, countdown, a single move on) is
  // @evey/client's, shared with the phone and the televisions.
  const { visible, counting, countdown, stillWatching, playNow, cancel } = useUpNextCountdown({
    nextEpisodeId: nextEpisode?.id,
    currentTime,
    duration,
    onPlay: onPlayNow,
    autoAdvance,
    suppress: suppressAutoAdvance,
  });
  useEffect(() => {
    onVisibleChange?.(visible);
  }, [visible, onVisibleChange]);
  useEffect(() => () => onVisibleChange?.(false), [onVisibleChange]);

  const handlePlayNow = playNow;
  const handleCancel = useCallback(() => {
    cancel();
    onCancel();
  }, [cancel, onCancel]);

  if (!visible || !nextEpisode) return null;

  // The series is obvious while its next episode is announced: code and title.
  const line = nextEpisode.indexNumber != null
    ? episodeLabel({ seasonNumber: nextEpisode.seasonIndexNumber, episodeNumber: nextEpisode.indexNumber, title: nextEpisode.title }) ||
      t("player.nextEpisodeOverlay.episode", { number: nextEpisode.indexNumber })
    : [t("player.nextEpisodeOverlay.nextEpisode"), nextEpisode.title].filter(Boolean).join(" · ");

  return (
    <div className="absolute bottom-24 right-4 z-float rounded-lg p-4 max-w-xs animate-in fade-in slide-in-from-right bg-[var(--overlay-scrim)] border border-[var(--border)]"
    >
      <p className="text-xs uppercase tracking-wider mb-1 text-[var(--text-muted)]">
        {stillWatching
          ? t("player.nextEpisodeOverlay.stillWatching")
          : counting ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(countdown, 0) }) : t("player.nextEpisodeOverlay.upNext")}
      </p>
      <p className="text-sm font-medium text-white mb-3 truncate">
        {line}
      </p>
      <div className="flex gap-2">
        <button
          onClick={handlePlayNow}
          className="px-3 py-1.5 rounded text-sm font-medium text-black transition-colors-fast bg-[var(--accent)]"
        >
          {t("player.nextEpisodeOverlay.playNow")}
        </button>
        <button
          onClick={handleCancel}
          className="px-3 py-1.5 rounded text-sm font-medium text-white transition-colors-fast hover:bg-white/20 bg-[var(--overlay-border-subtle)]"
        >
          {t("common.cancel")}
        </button>
      </div>
      {counting && (
        <div className="mt-2 h-0.5 rounded-full overflow-hidden bg-[var(--overlay-btn-bg)]">
          <div
            className="h-full rounded-full transition-all duration-1000 ease-linear bg-[var(--accent)]"
            style={{ width: `${((COUNTDOWN_SEC - Math.max(countdown, 0)) / COUNTDOWN_SEC) * 100}%` }}
          />
        </div>
      )}
    </div>
  );
}
