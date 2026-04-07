import { useState, useRef, useCallback, useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Play, Pause, X, Maximize } from "lucide-react";
import { usePlayerStore } from "@evey/store";

function clamp(val: number, min: number, max: number) {
  return Math.max(min, Math.min(max, val));
}

export function MiniPlayer() {
  const { t } = useTranslation();
  const { mediaId, mediaTitle, videoElement: _videoEl, stop, setMode } = usePlayerStore();
  const videoElement = _videoEl as HTMLVideoElement | null;
  const navigate = useNavigate();
  const containerRef = useRef<HTMLDivElement>(null);
  const videoContainerRef = useRef<HTMLDivElement>(null);
  const [playing, setPlaying] = useState(!videoElement?.paused);
  const [progress, setProgress] = useState(0);

  // Drag state
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const dragRef = useRef<{ startX: number; startY: number; originX: number; originY: number } | null>(null);
  const wasDrag = useRef(false);

  // Reparent video element into mini player container
  useEffect(() => {
    const container = videoContainerRef.current;
    const video = videoElement;
    if (!container || !video) return;

    video.className = "w-full h-full object-contain";
    container.appendChild(video);

    return () => {
      if (container.contains(video)) {
        container.removeChild(video);
      }
    };
  }, [videoElement]);

  useEffect(() => {
    const video = videoElement;
    if (!video) return;
    setPlaying(!video.paused);

    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onTime = () => {
      if (video.duration > 0) setProgress(video.currentTime / video.duration);
    };

    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onTime);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("timeupdate", onTime);
    };
  }, [videoElement]);

  const togglePlay = useCallback(() => {
    if (!videoElement) return;
    if (videoElement.paused) void videoElement.play();
    else videoElement.pause();
  }, [videoElement]);

  const expand = useCallback(() => {
    setMode("full");
    void navigate({ to: "/player/$mediaId", params: { mediaId: mediaId! } });
  }, [navigate, mediaId, setMode]);

  const close = useCallback(() => {
    stop();
  }, [stop]);

  // Drag handlers
  const onPointerDown = useCallback((e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest("button")) return;
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) return;
    wasDrag.current = false;
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      originX: pos?.x ?? window.innerWidth - rect.width - 16,
      originY: pos?.y ?? window.innerHeight - rect.height - 16,
    };
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }, [pos]);

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    if (!dragRef.current) return;
    const { startX, startY, originX, originY } = dragRef.current;
    const dx = e.clientX - startX;
    const dy = e.clientY - startY;
    if (Math.abs(dx) > 3 || Math.abs(dy) > 3) wasDrag.current = true;
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) return;
    setPos({
      x: clamp(originX + dx, 0, window.innerWidth - rect.width),
      y: clamp(originY + dy, 0, window.innerHeight - rect.height),
    });
  }, []);

  const onPointerUp = useCallback(() => {
    dragRef.current = null;
  }, []);

  if (!mediaId) return null;

  const style: React.CSSProperties = pos
    ? { left: pos.x, top: pos.y }
    : { right: 16, bottom: 16 };

  return (
    <div
      ref={containerRef}
      className="fixed z-overlay-raised w-[320px] rounded-lg overflow-hidden shadow-2xl border border-[var(--border)] bg-black select-none"
      style={style}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
    >
      {/* Video container — video element gets reparented here */}
      <div
        ref={videoContainerRef}
        className="aspect-video bg-black cursor-pointer"
        onClick={() => { if (!wasDrag.current) expand(); }}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === "Enter") expand(); }}
      />

      {/* Controls bar */}
      <div className="flex items-center gap-2 px-3 py-2 bg-[var(--surface)]">
        <button
          onClick={togglePlay}
          className="p-1 rounded hover:opacity-80 text-[var(--text)]"
          aria-label={playing ? t("common.aria.pause") : t("common.aria.play")}
        >
          {playing ? <Pause size={16} /> : <Play size={16} />}
        </button>

        {/* Progress bar */}
        <div className="flex-1 h-1 rounded-full bg-[var(--surface-raised)] overflow-hidden">
          <div
            className="h-full bg-[var(--accent)] transition-[width] duration-300"
            style={{ width: `${progress * 100}%` }}
          />
        </div>

        <span className="text-xs text-[var(--text-muted)] max-w-[100px] truncate" title={mediaTitle ?? ""}>
          {mediaTitle}
        </span>

        <button
          onClick={expand}
          className="p-1 rounded hover:opacity-80 text-[var(--text-muted)]"
          aria-label={t("common.aria.expand")}
        >
          <Maximize size={14} />
        </button>

        <button
          onClick={close}
          className="p-1 rounded hover:opacity-80 text-[var(--text-muted)]"
          aria-label={t("common.aria.close")}
        >
          <X size={14} />
        </button>
      </div>
    </div>
  );
}
