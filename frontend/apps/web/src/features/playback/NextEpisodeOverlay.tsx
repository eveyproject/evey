import { useCallback, useEffect, useRef, useState } from "react";

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
  const [countdown, setCountdown] = useState(COUNTDOWN_SEC);
  const [cancelled, setCancelled] = useState(false);
  const [ended, setEnded] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isInCredits =
    duration > 0 && currentTime > 0 && duration - currentTime <= CREDITS_THRESHOLD_SEC;
  const videoEnded = duration > 0 && currentTime >= duration - 0.5;

  useEffect(() => {
    if (videoEnded && !ended) setEnded(true);
  }, [videoEnded, ended]);

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
    ? `Episode ${nextEpisode.indexNumber}`
    : "Next Episode";

  return (
    <div className="absolute bottom-24 right-4 z-30 rounded-lg p-4 max-w-xs animate-in fade-in slide-in-from-right"
      style={{ backgroundColor: "rgba(0,0,0,0.85)", border: "1px solid var(--border)" }}
    >
      <p className="text-xs uppercase tracking-wider mb-1" style={{ color: "var(--text-muted)" }}>
        {ended ? `Next in ${Math.max(countdown, 0)}s` : "Up Next"}
      </p>
      <p className="text-sm font-medium text-white mb-3 truncate">
        {label}{nextEpisode.title ? ` - ${nextEpisode.title}` : ""}
      </p>
      <div className="flex gap-2">
        <button
          onClick={handlePlayNow}
          className="px-3 py-1.5 rounded text-sm font-medium text-black transition-colors"
          style={{ backgroundColor: "var(--accent)" }}
        >
          Play Now
        </button>
        <button
          onClick={handleCancel}
          className="px-3 py-1.5 rounded text-sm font-medium text-white transition-colors hover:bg-white/20"
          style={{ backgroundColor: "rgba(255,255,255,0.1)" }}
        >
          Cancel
        </button>
      </div>
      {ended && (
        <div className="mt-2 h-0.5 rounded-full overflow-hidden" style={{ backgroundColor: "rgba(255,255,255,0.2)" }}>
          <div
            className="h-full rounded-full transition-all duration-1000 ease-linear"
            style={{
              backgroundColor: "var(--accent)",
              width: `${((COUNTDOWN_SEC - Math.max(countdown, 0)) / COUNTDOWN_SEC) * 100}%`,
            }}
          />
        </div>
      )}
    </div>
  );
}
