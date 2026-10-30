import { useTranslation } from "react-i18next";
import { RotateCcw, RotateCw, Play, Pause, Loader, TriangleAlert, Volume2, Sun, Clock, MessageSquare } from "lucide-react";
import { canRetryShed, shedI18nKey } from "@evey/client";
import { ICON } from "@/constants/iconSizes";
import { DownloadButton } from "./DownloadButton";
import type { ShedReason } from "./hooks/useTranscodePlayback";

// The overlays HLSPlayer draws over the picture. None holds state: the player
// decides when each shows, these only say how.

// What a tap, a double-tap or a swipe just did, flashed where the finger was.
export function GestureFeedback({
  seekFlash,
  swipeIndicator,
  actionFlash,
}: {
  seekFlash: "left" | "right" | null;
  swipeIndicator: { type: "volume" | "brightness"; value: number } | null;
  actionFlash: "play" | "pause" | null;
}) {
  return (
    <>
      {seekFlash === "left" && (
        <div className="absolute left-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCcw size={ICON["2xl"]} className="text-white" />
          </div>
        </div>
      )}
      {seekFlash === "right" && (
        <div className="absolute right-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCw size={ICON["2xl"]} className="text-white" />
          </div>
        </div>
      )}
      {swipeIndicator && (
        <div
          className={`absolute top-0 bottom-0 w-16 flex items-center justify-center pointer-events-none z-[6] ${swipeIndicator.type === "volume" ? "right-4" : "left-4"}`}
        >
          <div className="flex flex-col items-center gap-2 rounded-xl bg-black/60 px-3 py-4 backdrop-blur-sm">
            {swipeIndicator.type === "volume" ? <Volume2 size={ICON.lg} className="text-white" /> : <Sun size={ICON.lg} className="text-white" />}
            <div className="w-1 h-24 rounded-full bg-white/20 relative overflow-hidden">
              <div
                className="absolute bottom-0 left-0 right-0 rounded-full bg-white"
                style={{ height: `${Math.round((swipeIndicator.type === "volume" ? swipeIndicator.value : (swipeIndicator.value - 0.2) / 1.3) * 100)}%` }}
              />
            </div>
            <span className="text-white text-xs font-medium">{Math.round(swipeIndicator.value * 100)}%</span>
          </div>
        </div>
      )}
      {actionFlash && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-black/40 w-16 h-16 flex items-center justify-center animate-ping-once">
            {actionFlash === "play" ? (
              <Play size={ICON["2xl"]} className="text-white ml-1" fill="currentColor" />
            ) : (
              <Pause size={ICON["2xl"]} className="text-white" fill="currentColor" />
            )}
          </div>
        </div>
      )}
    </>
  );
}

// A message an admin sent to this session from the dashboard.
export function AdminMessageBanner({ message }: { message: string }) {
  return (
    <div className="absolute top-6 inset-x-0 flex justify-center pointer-events-none z-[8]">
      <div
        role="status"
        data-testid="admin-message"
        className="flex items-center gap-3 max-w-xl mx-4 px-4 py-3 rounded-[var(--radius-md)] bg-black/75 border border-white/20 text-white shadow-lg backdrop-blur-sm animate-fade-in"
      >
        <MessageSquare size={ICON.md} className="shrink-0 text-white/80" />
        <span className="text-sm font-medium break-words">{message}</span>
      </div>
    </div>
  );
}

export function BufferingIndicator() {
  const { t } = useTranslation();
  return (
    <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-[5]">
      <div className="flex flex-col items-center gap-2">
        <Loader size={ICON["3xl"]} className="animate-spin text-white/80" />
        <span className="text-white/80 text-sm font-medium">{t("player.buffering")}</span>
      </div>
    </div>
  );
}

export function FatalErrorOverlay({
  mediaId,
  streamToken,
  title,
  onRetry,
}: {
  mediaId: string;
  streamToken?: string | null;
  title?: string;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="absolute inset-0 flex items-center justify-center z-[10] bg-[var(--overlay-bg-strong)]">
      <div className="flex flex-col items-center gap-4 text-white">
        <TriangleAlert size={ICON["4xl"]} className="text-[var(--color-danger)]" strokeWidth={1.5} />
        <p className="text-lg font-medium">{t("player.playbackFailed")}</p>
        <p className="text-sm text-[var(--overlay-text-muted)]">{t("player.unableToPlayAnyQuality")}</p>
        <div className="flex items-center gap-2">
          <button
            onClick={onRetry}
            className="px-4 py-2 rounded text-sm font-medium bg-[var(--overlay-btn-hover-bg)]"
          >
            {t("player.retry")}
          </button>
          {/* Playability fallback: if the server can't deliver a playable
              stream, let the user download the original file instead of
              being stuck on the error. */}
          {streamToken && (
            <DownloadButton mediaId={mediaId} token={streamToken} title={title} />
          )}
        </div>
      </div>
    </div>
  );
}

// Server-side load shed (503): distinct from the fatal error - nothing is
// broken, the server just isn't ready to serve THIS request yet. Calm icon (no
// red TriangleAlert) and copy that names neither "transcoding" nor any other
// internal term.
export function ShedOverlay({ shedReason, onRetry }: { shedReason: NonNullable<ShedReason>; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="absolute inset-0 flex items-center justify-center z-[10] bg-[var(--overlay-bg-strong)]">
      <div className="flex flex-col items-center gap-4 text-white text-center px-6">
        {canRetryShed(shedReason)
          ? <Clock size={ICON["4xl"]} className="text-white/80" strokeWidth={1.5} />
          : <TriangleAlert size={ICON["4xl"]} className="text-white/80" strokeWidth={1.5} />}
        <p className="text-lg font-medium">
          {t(shedI18nKey(shedReason, "title"))}
        </p>
        <p className="text-sm text-[var(--overlay-text-muted)] max-w-sm">
          {t(shedI18nKey(shedReason, "body"))}
        </p>
        {canRetryShed(shedReason) && (
          <button
            onClick={onRetry}
            className="px-4 py-2 rounded text-sm font-medium bg-[var(--overlay-btn-hover-bg)]"
          >
            {t("player.retry")}
          </button>
        )}
      </div>
    </div>
  );
}
