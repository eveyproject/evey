import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

interface NextEpisodeOverlayProps {
  nextEpisode: { id: string; title?: string; indexNumber?: number } | null;
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

const CREDITS_THRESHOLD_SEC = 30;
const COUNTDOWN_SEC = 5;

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

  const visible = !!nextEpisode && !cancelled && !suppressAutoAdvance && (isInCredits || ended);
  useEffect(() => {
    onVisibleChange?.(visible);
  }, [visible, onVisibleChange]);
  useEffect(() => () => onVisibleChange?.(false), [onVisibleChange]);

  // Counting down is the auto-advance itself: without it the card is a plain
  // "up next" prompt that waits for the viewer.
  const counting = ended && autoAdvance;

  useEffect(() => {
    if (!counting || cancelled || !nextEpisode || suppressAutoAdvance) return;
    setCountdown(COUNTDOWN_SEC);
    intervalRef.current = setInterval(() => {
      setCountdown((prev) => prev - 1);
    }, 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [counting, cancelled, nextEpisode, suppressAutoAdvance]);

  // Fire the advance exactly once per next-episode cycle. onPlayNow's identity
  // is not guaranteed stable by every caller, so without this guard any
  // re-render after countdown hit 0 would re-run the effect and navigate twice.
  const firedRef = useRef(false);
  useEffect(() => {
    firedRef.current = false;
  }, [nextEpisode?.id]);

  const handlePlayNow = useCallback(() => {
    if (firedRef.current) return;
    firedRef.current = true;
    if (intervalRef.current) clearInterval(intervalRef.current);
    if (nextEpisode) onPlayNow(nextEpisode.id);
  }, [nextEpisode, onPlayNow]);

  useEffect(() => {
    if (countdown <= 0 && counting && nextEpisode && !cancelled && !firedRef.current && !suppressAutoAdvance) {
      firedRef.current = true;
      if (intervalRef.current) clearInterval(intervalRef.current);
      onPlayNow(nextEpisode.id);
    }
  }, [countdown, counting, nextEpisode, cancelled, onPlayNow, suppressAutoAdvance]);

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
        {counting ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(countdown, 0) }) : t("player.nextEpisodeOverlay.upNext")}
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
