import { useCallback, useEffect, useRef, useState } from "react";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { Select } from "@/components/ui/Select";
import type { Chapter } from "@/hooks/useChapters";

export interface EpisodeNavItem { id: string; title?: string; indexNumber?: number }
export interface SeasonNavItem { id: string; title?: string; indexNumber?: number }

const QUALITY_OPTIONS = [
  { label: "Original", value: 0 },
  { label: "1080p", value: 1080 },
  { label: "720p", value: 720 },
  { label: "480p", value: 480 },
  { label: "360p", value: 360 },
];

interface EpisodesPanelProps {
  seasons: SeasonNavItem[];
  currentSeasonId: string;
  episodes: EpisodeNavItem[];
  currentEpisodeId: string;
  onSeasonChange: (id: string) => void;
  onEpisodeChange: (id: string) => void;
  onOpenChange: (open: boolean) => void;
}

function EpisodesPanel({ seasons, currentSeasonId, episodes, currentEpisodeId, onSeasonChange, onEpisodeChange, onOpenChange }: EpisodesPanelProps) {
  const [open, setOpen] = useState(false);

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    const next = !open;
    setOpen(next);
    onOpenChange(next);
  }

  return (
    <div className="relative">
      <button onClick={toggle} aria-label="Episodes" className="text-white hover:text-[var(--accent)] transition-colors p-1">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M3 13h2v-2H3v2zm0 4h2v-2H3v2zm0-8h2V7H3v2zm4 4h14v-2H7v2zm0 4h14v-2H7v2zM7 7v2h14V7H7z" />
        </svg>
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-20"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          {seasons.length > 1 && (
            <div className="px-3 py-2" style={{ borderBottom: "1px solid var(--border)" }}>
              <Select
                value={currentSeasonId}
                onChange={onSeasonChange}
                options={seasons.map((s) => ({
                  value: s.id,
                  label: s.title ?? `Season ${s.indexNumber}`,
                }))}
              />
            </div>
          )}
          <div className="max-h-72 overflow-y-auto">
            {episodes.map((ep) => (
              <button
                key={ep.id}
                onClick={() => { onEpisodeChange(ep.id); setOpen(false); onOpenChange(false); }}
                className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors text-left"
                style={{ color: ep.id === currentEpisodeId ? "var(--accent)" : "var(--text)" }}
              >
                <span className="w-8 flex-shrink-0 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                  E{ep.indexNumber ?? "?"}
                </span>
                <span className="flex-1 truncate">{ep.title}</span>
                {ep.id === currentEpisodeId && (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" className="flex-shrink-0">
                    <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                  </svg>
                )}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

interface ChaptersPanelProps {
  chapters: Chapter[];
  currentTimeSec: number;
  onSeek: (timeSec: number) => void;
  onOpenChange: (open: boolean) => void;
}

function ChaptersPanel({ chapters, currentTimeSec, onSeek, onOpenChange }: ChaptersPanelProps) {
  const [open, setOpen] = useState(false);

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    const next = !open;
    setOpen(next);
    onOpenChange(next);
  }

  const currentMs = currentTimeSec * 1000;

  return (
    <div className="relative">
      <button onClick={toggle} aria-label="Chapters" className="text-white hover:text-[var(--accent)] transition-colors p-1">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM10 9h8v2h-8V9zm0 3h4v2h-4v-2zm0-6h8v2h-8V6z" />
        </svg>
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-20"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
            Chapters
          </div>
          <div className="max-h-72 overflow-y-auto">
            {chapters.map((ch, i) => {
              const startSec = ch.startTimeMs / 1000;
              const isActive = currentMs >= ch.startTimeMs && (i === chapters.length - 1 || currentMs < chapters[i + 1].startTimeMs);
              return (
                <button
                  key={i}
                  onClick={() => { onSeek(startSec); setOpen(false); onOpenChange(false); }}
                  className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors text-left"
                  style={{ color: isActive ? "var(--accent)" : "var(--text)" }}
                >
                  <span className="w-12 flex-shrink-0 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                    {formatTime(startSec)}
                  </span>
                  <span className="flex-1 truncate">{ch.title || `Chapter ${i + 1}`}</span>
                  {isActive && (
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" className="flex-shrink-0">
                      <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                    </svg>
                  )}
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

interface PlayerControlsProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  knownDurationSec?: number;
  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  title?: string;
  onClose?: () => void;
  currentAudioIndex: number;
  onAudioChange: (index: number) => void;
  currentQuality: number;
  onQualityChange: (quality: number) => void;
  currentSubtitleTrackIndex: number | null;
  onSubtitleChange: (trackIndex: number | null) => void;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  chapters?: Chapter[];
}

function formatTime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

interface SettingsPanelProps {
  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  currentAudioIndex: number;
  onAudioChange: (index: number) => void;
  currentQuality: number;
  onQualityChange: (quality: number) => void;
  currentSubtitleTrackIndex: number | null;
  onSubtitleChange: (trackIndex: number | null) => void;
  onOpenChange: (open: boolean) => void;
}

function SettingsPanel({
  audioStreams,
  subtitleStreams,
  currentAudioIndex,
  onAudioChange,
  currentQuality,
  onQualityChange,
  currentSubtitleTrackIndex,
  onSubtitleChange,
  onOpenChange,
}: SettingsPanelProps) {
  const [open, setOpen] = useState(false);

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    const next = !open;
    setOpen(next);
    onOpenChange(next);
  }

  function select(fn: () => void) {
    fn();
    setOpen(false);
    onOpenChange(false);
  }

  return (
    <div className="relative">
      <button
        onClick={toggle}
        aria-label="Settings"
        className="text-white hover:text-[var(--accent)] transition-colors p-1"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
          <path d="M19.14 12.94c.04-.3.06-.61.06-.94s-.02-.64-.07-.94l2.03-1.58a.49.49 0 0 0 .12-.61l-1.92-3.32a.49.49 0 0 0-.59-.22l-2.39.96a7.01 7.01 0 0 0-1.62-.94l-.36-2.54a.484.484 0 0 0-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.48.48 0 0 0-.59.22L2.74 8.87a.47.47 0 0 0 .12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 0 0-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32a.47.47 0 0 0-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z" />
        </svg>
      </button>

      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-56 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-20"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Quality */}
          <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
            Quality
          </div>
          {QUALITY_OPTIONS.map((q) => (
            <button
              key={q.value}
              onClick={() => select(() => onQualityChange(q.value))}
              className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
              style={{ color: q.value === currentQuality ? "var(--accent)" : "var(--text)" }}
            >
              <span>{q.label}</span>
              {q.value === currentQuality && (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                </svg>
              )}
            </button>
          ))}

          {/* Audio */}
          {audioStreams.length > 0 && (
            <>
              <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
              <div className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
                Audio
              </div>
              {audioStreams.map((s) => {
                const label = s.title ?? s.language ?? `Track ${s.index}`;
                return (
                  <button
                    key={s.index}
                    onClick={() => select(() => onAudioChange(s.index))}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: s.index === currentAudioIndex ? "var(--accent)" : "var(--text)" }}
                  >
                    <span>{label}</span>
                    {s.index === currentAudioIndex && (
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                      </svg>
                    )}
                  </button>
                );
              })}
            </>
          )}

          {/* Subtitle */}
          {subtitleStreams.length > 0 && (
            <>
              <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
              <div className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
                Subtitles
              </div>
              <button
                onClick={() => select(() => onSubtitleChange(null))}
                className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                style={{ color: currentSubtitleTrackIndex === null ? "var(--accent)" : "var(--text)" }}
              >
                <span>Off</span>
                {currentSubtitleTrackIndex === null && (
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                  </svg>
                )}
              </button>
              {subtitleStreams.map((s) => {
                const label = s.title ?? s.language ?? `Track ${s.index}`;
                return (
                  <button
                    key={s.index}
                    onClick={() => select(() => onSubtitleChange(s.trackIndex))}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: s.trackIndex === currentSubtitleTrackIndex ? "var(--accent)" : "var(--text)" }}
                  >
                    <span>{label}</span>
                    {s.trackIndex === currentSubtitleTrackIndex && (
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                      </svg>
                    )}
                  </button>
                );
              })}
            </>
          )}
        </div>
      )}
    </div>
  );
}

export function PlayerControls({
  videoRef,
  knownDurationSec,
  audioStreams,
  subtitleStreams,
  title,
  onClose,
  currentAudioIndex,
  onAudioChange,
  currentQuality,
  onQualityChange,
  currentSubtitleTrackIndex,
  onSubtitleChange,
  seasons = [],
  currentSeasonId = "",
  episodes = [],
  currentEpisodeId = "",
  onSeasonChange,
  onEpisodeChange,
  chapters = [],
}: PlayerControlsProps) {
  const [visible, setVisible] = useState(true);
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedEnd, setBufferedEnd] = useState(0);
  const [volume, setVolume] = useState(1);
  const [fullscreen, setFullscreen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [episodesOpen, setEpisodesOpen] = useState(false);
  const [chaptersOpen, setChaptersOpen] = useState(false);
  const [seekHoverPos, setSeekHoverPos] = useState<number | null>(null);
  const [seekHoverX, setSeekHoverX] = useState(0);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seekBarRef = useRef<HTMLDivElement | null>(null);

  // In fMP4 transcode mode, video.duration reports only buffered amount (e.g. 11s for a 2h movie).
  // Always prefer knownDurationSec from the DB when available.
  const effectiveDuration = (knownDurationSec && knownDurationSec > 0)
    ? knownDurationSec
    : (duration && isFinite(duration) && duration > 0) ? duration : 0;

  const scheduleHide = useCallback(() => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setVisible(false), 3000);
  }, []);

  const resetHideTimer = useCallback(() => {
    setVisible(true);
    if (!settingsOpen) scheduleHide();
  }, [settingsOpen, scheduleHide]);

  // Keep visible and cancel timer when settings, episodes, or chapters panel open
  useEffect(() => {
    if (settingsOpen || episodesOpen || chaptersOpen) {
      if (hideTimer.current) clearTimeout(hideTimer.current);
      setVisible(true);
    } else {
      scheduleHide();
    }
  }, [settingsOpen, episodesOpen, chaptersOpen, scheduleHide]);

  useEffect(() => {
    scheduleHide();
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current);
    };
  }, [scheduleHide]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    function onPlay() { setPlaying(true); }
    function onPause() { setPlaying(false); }
    function onTimeUpdate() { setCurrentTime(video!.currentTime); }
    function onDurationChange() {
      const d = video!.duration;
      if (d && isFinite(d) && d > 0) setDuration(d);
    }
    function onVolumeChange() { setVolume(video!.volume); }
    function onFullscreenChange() { setFullscreen(!!document.fullscreenElement); }
    function onProgress() {
      const b = video!.buffered;
      if (b.length > 0) setBufferedEnd(b.end(b.length - 1));
    }

    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("volumechange", onVolumeChange);
    video.addEventListener("progress", onProgress);
    document.addEventListener("fullscreenchange", onFullscreenChange);

    setPlaying(!video.paused);
    setCurrentTime(video.currentTime);
    const d = video.duration;
    if (d && isFinite(d) && d > 0) setDuration(d);
    setVolume(video.volume);

    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("durationchange", onDurationChange);
      video.removeEventListener("volumechange", onVolumeChange);
      video.removeEventListener("progress", onProgress);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [videoRef]);

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  function handleSeek(e: React.ChangeEvent<HTMLInputElement>) {
    const video = videoRef.current;
    if (!video) return;
    video.currentTime = Number(e.target.value);
  }

  function handleVolume(e: React.ChangeEvent<HTMLInputElement>) {
    const video = videoRef.current;
    if (!video) return;
    video.volume = Number(e.target.value);
  }

  function toggleFullscreen() {
    const video = videoRef.current;
    if (!video) return;
    const container = video.parentElement;
    if (!container) return;
    if (!document.fullscreenElement) {
      void container.requestFullscreen();
    } else {
      void document.exitFullscreen();
    }
  }

  function handleOverlayClick(e: React.MouseEvent) {
    // Ignore clicks on control buttons (bottom bar, top bar)
    if ((e.target as HTMLElement).closest("button, input, [data-controls]")) return;
    if (clickTimer.current) {
      clearTimeout(clickTimer.current);
      clickTimer.current = null;
      toggleFullscreen();
    } else {
      clickTimer.current = setTimeout(() => {
        clickTimer.current = null;
        resetHideTimer();
      }, 250);
    }
  }

  function handleSeekHover(e: React.MouseEvent) {
    const bar = seekBarRef.current;
    if (!bar || effectiveDuration <= 0) { setSeekHoverPos(null); return; }
    const rect = bar.getBoundingClientRect();
    const x = Math.max(0, Math.min(e.clientX - rect.left, rect.width));
    const ratio = x / rect.width;
    setSeekHoverPos(ratio * effectiveDuration);
    setSeekHoverX(x);
  }

  return (
    <div
      className="absolute inset-0"
      onMouseMove={resetHideTimer}
      onClick={handleOverlayClick}
      style={{ cursor: visible ? "default" : "none" }}
    >
      {/* top bar */}
      <div
        className={`absolute top-0 left-0 right-0 flex items-center gap-3 px-4 py-3 transition-opacity duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to bottom, rgba(0,0,0,0.7), transparent)" }}
        data-controls
      >
        <button
          onClick={onClose}
          aria-label="Back"
          className="text-white hover:text-[var(--accent)] transition-colors"
        >
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M19 12H5M12 5l-7 7 7 7" />
          </svg>
        </button>
        {title && <span className="text-white text-sm font-medium truncate">{title}</span>}
      </div>

      {/* bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 px-4 pb-4 pt-8 flex flex-col gap-2 transition-opacity duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to top, rgba(0,0,0,0.8), transparent)" }}
        data-controls
      >
        {/* seek bar */}
        <div
          ref={seekBarRef}
          className="relative w-full h-5 flex items-center group cursor-pointer"
          onMouseMove={handleSeekHover}
          onMouseLeave={() => setSeekHoverPos(null)}
          data-controls
        >
          {/* hover tooltip */}
          {seekHoverPos !== null && (() => {
            const hoverMs = seekHoverPos * 1000;
            const hoverChapter = chapters.find((ch, i) =>
              hoverMs >= ch.startTimeMs && (i === chapters.length - 1 || hoverMs < chapters[i + 1].startTimeMs)
            );
            return (
              <div
                className="absolute bottom-full mb-2 -translate-x-1/2 px-2 py-1 rounded text-xs text-white pointer-events-none text-center"
                style={{ left: seekHoverX, backgroundColor: "rgba(0,0,0,0.85)", maxWidth: 200 }}
              >
                {hoverChapter?.title && <div className="truncate">{hoverChapter.title}</div>}
                <div>{formatTime(seekHoverPos)}</div>
              </div>
            );
          })()}
          {/* background track */}
          <div className="absolute left-0 right-0 h-1 rounded bg-white/20 group-hover:h-1.5 transition-all" />
          {/* buffered track */}
          <div
            className="absolute left-0 h-1 rounded bg-white/40 group-hover:h-1.5 transition-all"
            style={{ width: effectiveDuration > 0 ? `${(bufferedEnd / effectiveDuration) * 100}%` : "0%" }}
          />
          {/* progress track */}
          <div
            className="absolute left-0 h-1 rounded group-hover:h-1.5 transition-all"
            style={{
              width: effectiveDuration > 0 ? `${(currentTime / effectiveDuration) * 100}%` : "0%",
              backgroundColor: "var(--accent)",
            }}
          />
          {/* chapter markers */}
          {effectiveDuration > 0 && chapters.map((ch, i) => {
            const pct = (ch.startTimeMs / 1000 / effectiveDuration) * 100;
            if (pct <= 0) return null;
            return (
              <div
                key={i}
                className="absolute top-1/2 -translate-y-1/2 w-0.5 h-3 bg-white/70 pointer-events-none z-10"
                style={{ left: `${pct}%` }}
              />
            );
          })}
          <input
            type="range"
            min={0}
            max={effectiveDuration || 0}
            step={0.5}
            value={currentTime}
            onChange={handleSeek}
            className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
            aria-label="Seek"
          />
        </div>

        <div className="flex items-center gap-2 sm:gap-3">
          {/* play/pause - larger touch target on mobile */}
          <button
            onClick={togglePlay}
            aria-label={playing ? "Pause" : "Play"}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            {playing ? (
              <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
                <rect x="6" y="4" width="4" height="16" />
                <rect x="14" y="4" width="4" height="16" />
              </svg>
            ) : (
              <svg width="28" height="28" viewBox="0 0 24 24" fill="currentColor">
                <path d="M5 3l14 9-14 9V3z" />
              </svg>
            )}
          </button>

          {/* time */}
          <span className="text-white text-xs tabular-nums">
            {formatTime(currentTime)} / {formatTime(effectiveDuration)}
          </span>

          {/* volume - hidden on mobile (hardware volume) */}
          <div className="hidden sm:flex items-center gap-1">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" className="text-white shrink-0">
              <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z" />
            </svg>
            <input
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={volume}
              onChange={handleVolume}
              className="w-20 accent-[var(--accent)] cursor-pointer"
              aria-label="Volume"
            />
          </div>

          <div className="flex-1" />

          {/* Chapters panel */}
          {chapters.length > 0 && (
            <ChaptersPanel
              chapters={chapters}
              currentTimeSec={currentTime}
              onSeek={(t) => { if (videoRef.current) videoRef.current.currentTime = t; }}
              onOpenChange={setChaptersOpen}
            />
          )}

          {/* Episodes panel (only for TV episodes) */}
          {episodes.length > 0 && onEpisodeChange && (
            <EpisodesPanel
              seasons={seasons}
              currentSeasonId={currentSeasonId}
              episodes={episodes}
              currentEpisodeId={currentEpisodeId}
              onSeasonChange={onSeasonChange ?? (() => {})}
              onEpisodeChange={onEpisodeChange}
              onOpenChange={setEpisodesOpen}
            />
          )}

          {/* Settings gear */}
          <SettingsPanel
            audioStreams={audioStreams}
            subtitleStreams={subtitleStreams}
            currentAudioIndex={currentAudioIndex}
            onAudioChange={onAudioChange}
            currentQuality={currentQuality}
            onQualityChange={onQualityChange}
            currentSubtitleTrackIndex={currentSubtitleTrackIndex}
            onSubtitleChange={onSubtitleChange}
            onOpenChange={setSettingsOpen}
          />

          {/* fullscreen */}
          <button
            onClick={toggleFullscreen}
            aria-label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            {fullscreen ? (
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z" />
              </svg>
            ) : (
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z" />
              </svg>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
