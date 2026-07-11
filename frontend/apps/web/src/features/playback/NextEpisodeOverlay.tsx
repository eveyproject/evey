import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

interface NextEpisodeOverlayProps {
  nextEpisode: { id: string; title?: string; indexNumber?: number } | null;
  currentTime: number;
  duration: number;
  onPlayNow: (id: string) => void;
  onCancel: () => void;
}

const CREDITS_THRESHOLD_SEC = 30;
const COUNTDOWN_SEC = 5;

export function NextEpisodeOverlay({
  nextEpisode,
  currentTime,
  duration,
  onPlayNow,
  onCancel,
}: NextEpisodeOverlayProps) {
  const { t } = useTranslation();
  const [countdown, setCountdown] = useState(COUNTDOWN_SEC);
  const [cancelled, setCancelled] = useState(false);
  const [ended, setEnded] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isInCredits =
    duration > 0 && currentTime > 0 && duration - currentTime <= CREDITS_THRESHOLD_SEC;
  const videoEnded = duration > 0 && currentTime >= duration - 0.5;

  // Sync the `ended` flag with videoEnded so a backward seek after the
  // video reached the end (or auto-advance was cancelled and then the
  // user scrubbed back) clears the countdown UI - otherwise it would
  // pop up again mid-episode despite the playhead being far from the end.
  useEffect(() => {
    setEnded(videoEnded);
  }, [videoEnded]);

  const visible = nextEpisode && !cancelled && (isInCredits || ended);

  useEffect(() => {
    if (!ended || cancelled || !nextEpisode) return;
    setCountdown(COUNTDOWN_SEC);
    intervalRef.current = setInterval(() => {
      setCountdown((prev) => prev - 1);
    }, 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [ended, cancelled, nextEpisode]);

  const handlePlayNow = useCallback(() => {
    if (intervalRef.current) clearInterval(intervalRef.current);
    if (nextEpisode) onPlayNow(nextEpisode.id);
  }, [nextEpisode, onPlayNow]);

  useEffect(() => {
    if (countdown <= 0 && nextEpisode && !cancelled) {
      if (intervalRef.current) clearInterval(intervalRef.current);
      onPlayNow(nextEpisode.id);
    }
  }, [countdown, nextEpisode, cancelled, onPlayNow]);

  const handleCancel = useCallback(() => {
    setCancelled(true);
    if (intervalRef.current) clearInterval(intervalRef.current);
    onCancel();
  }, [onCancel]);

  // Reset cancelled state when episode changes
  useEffect(() => {
    setCancelled(false);
    setEnded(false);
  }, [nextEpisode?.id]);

  if (!visible) return null;

  const label = nextEpisode.indexNumber != null
    ? t("player.nextEpisodeOverlay.episode", { number: nextEpisode.indexNumber })
    : t("player.nextEpisodeOverlay.nextEpisode");

  return (
    <div className="absolute bottom-24 right-4 z-float rounded-lg p-4 max-w-xs animate-in fade-in slide-in-from-right bg-[var(--overlay-scrim)] border border-[var(--border)]"
    >
      <p className="text-xs uppercase tracking-wider mb-1 text-[var(--text-muted)]">
        {ended ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(countdown, 0) }) : t("player.nextEpisodeOverlay.upNext")}
      </p>
      <p className="text-sm font-medium text-white mb-3 truncate">
        {label}{nextEpisode.title ? ` - ${nextEpisode.title}` : ""}
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
      {ended && (
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
