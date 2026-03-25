import { useCallback, useEffect, useRef, useState } from "react";
import { List, BookOpen, Check, ChevronLeft, ChevronRight, Settings, ArrowLeft, SkipBack, SkipForward, Pause, Play, VolumeX, Volume1, Volume2, Upload, Minimize, Maximize, PictureInPicture, PictureInPicture2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { Select } from "@/components/ui/Select";
import type { Chapter } from "@/hooks/useChapters";
import { TrickplayPreview } from "./TrickplayPreview";
import { ChapterMarkers } from "./ChapterMarkers";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import { SPEED_OPTIONS } from "./constants";

export interface EpisodeNavItem { id: string; title?: string; indexNumber?: number }
export interface SeasonNavItem { id: string; title?: string; indexNumber?: number }

function useQualityOptions() {
  const { t } = useTranslation();
  return [
    { label: t("player.quality.original"), value: 0 },
    { label: "1080p", value: 1080 },
    { label: "720p", value: 720 },
    { label: "480p", value: 480 },
    { label: "360p", value: 360 },
  ];
}

interface EpisodesPanelProps {
  seasons: SeasonNavItem[];
  currentSeasonId: string;
  episodes: EpisodeNavItem[];
  currentEpisodeId: string;
  onSeasonChange: (id: string) => void;
  onEpisodeChange: (id: string) => void;
  open: boolean;
  onToggle: () => void;
}

function EpisodesPanel({ seasons, currentSeasonId, episodes, currentEpisodeId, onSeasonChange, onEpisodeChange, open, onToggle }: EpisodesPanelProps) {
  const { t } = useTranslation();
  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  return (
    <div className="relative">
      <button onClick={toggle} aria-label={t("player.controls.episodes")} title={t("player.controls.episodes")} className="text-white hover:text-[var(--accent)] transition-colors p-1">
        <List size={20} />
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-badge"
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
                  label: s.title ?? t("media.season", { number: s.indexNumber }),
                }))}
              />
            </div>
          )}
          <div className="max-h-72 overflow-y-auto">
            {episodes.map((ep) => (
              <button
                key={ep.id}
                onClick={() => { onEpisodeChange(ep.id); onToggle(); }}
                className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors text-left"
                style={{ color: ep.id === currentEpisodeId ? "var(--accent)" : "var(--text)" }}
              >
                <span className="w-8 flex-shrink-0 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                  E{ep.indexNumber ?? "?"}
                </span>
                <span className="flex-1 truncate">{ep.title}</span>
                {ep.id === currentEpisodeId && (
                  <Check size={12} className="flex-shrink-0" />
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
  open: boolean;
  onToggle: () => void;
}

function ChaptersPanel({ chapters, currentTimeSec, onSeek, open, onToggle }: ChaptersPanelProps) {
  const { t } = useTranslation();
  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  const currentMs = currentTimeSec * 1000;

  return (
    <div className="relative">
      <button onClick={toggle} aria-label={t("player.controls.chapters")} title={t("player.controls.chapters")} className="text-white hover:text-[var(--accent)] transition-colors p-1">
        <BookOpen size={20} />
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-badge"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
            {t("player.controls.chapters")}
          </div>
          <div className="max-h-72 overflow-y-auto">
            {chapters.map((ch, i) => {
              const startSec = ch.startTimeMs / 1000;
              const isActive = currentMs >= ch.startTimeMs && (i === chapters.length - 1 || currentMs < chapters[i + 1].startTimeMs);
              return (
                <button
                  key={i}
                  onClick={() => { onSeek(startSec); onToggle(); }}
                  className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors text-left"
                  style={{ color: isActive ? "var(--accent)" : "var(--text)" }}
                >
                  <span className="w-12 flex-shrink-0 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                    {formatTime(startSec)}
                  </span>
                  <span className="flex-1 truncate">{ch.title || t("media.chapter", { number: i + 1 })}</span>
                  {isActive && (
                    <Check size={12} className="flex-shrink-0" />
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
  playbackSpeed?: number;
  onSpeedChange?: (speed: number) => void;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  chapters?: Chapter[];
  mediaId?: string;
  mediaType?: string;
  onSeekRequest?: (absoluteTimeSec: number) => void;
  transcodeOffset?: number;
  autoQuality?: boolean;
  onAutoQualityToggle?: (auto: boolean) => void;
  autoQualityLabel?: string;
  skipForwardSeconds?: number;
  skipBackwardSeconds?: number;
  onExternalSubtitle?: (blobUrl: string, label: string) => void;
  subtitleStyle?: SubtitleStyle;
  onSubtitleStyleChange?: (patch: Partial<SubtitleStyle>) => void;
  onPanelOpenChange?: (anyOpen: boolean) => void;
  closePanelRef?: React.MutableRefObject<(() => void) | null>;
  onActionFlash?: (action: "play" | "pause") => void;
  streamToken?: string;
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
  playbackSpeed?: number;
  onSpeedChange?: (speed: number) => void;
  open: boolean;
  onToggle: () => void;
  autoQuality?: boolean;
  onAutoQualityToggle?: (auto: boolean) => void;
  autoQualityLabel?: string;
  onExternalSubtitle?: (blobUrl: string, label: string) => void;
  subtitleStyle?: SubtitleStyle;
  onSubtitleStyleChange?: (patch: Partial<SubtitleStyle>) => void;
}

const SUBTITLE_COLORS = [
  { labelKey: "player.colors.white", value: "#ffffff" },
  { labelKey: "player.colors.yellow", value: "#fde047" },
  { labelKey: "player.colors.green", value: "#4ade80" },
  { labelKey: "player.colors.cyan", value: "#22d3ee" },
  { labelKey: "player.colors.orange", value: "#fb923c" },
];

const SUBTITLE_BG_COLORS = [
  { labelKey: "player.bgColors.black", value: "#000000" },
  { labelKey: "player.bgColors.darkGrey", value: "#1a1a1a" },
  { labelKey: "player.bgColors.navy", value: "#1e3a5f" },
  { labelKey: "player.bgColors.none", value: "#000000", opacity: 0 },
];

const SUBTITLE_FONTS = [
  { labelKey: "player.fonts.default", value: "" },
  { labelKey: "player.fonts.sansSerif", value: "Arial, sans-serif" },
  { labelKey: "player.fonts.serif", value: "Georgia, serif" },
  { labelKey: "player.fonts.monospace", value: "'Courier New', monospace" },
  { labelKey: "player.fonts.comicSans", value: "'Comic Sans MS', cursive" },
];

function srtToVtt(srt: string): string {
  const vtt = srt
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    // SRT timestamps use comma for ms, VTT uses period
    .replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, "$1.$2");
  return `WEBVTT\n\n${vtt.trim()}\n`;
}

async function loadSubtitleFile(file: File): Promise<string> {
  const text = await file.text();
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  if (ext === "srt" || ext === "ass" || ext === "ssa") {
    const vtt = srtToVtt(text);
    const blob = new Blob([vtt], { type: "text/vtt" });
    return URL.createObjectURL(blob);
  }
  return URL.createObjectURL(file);
}

const CheckIcon = () => <Check size={14} />;

const ChevronRightIcon = () => <ChevronRight size={14} style={{ color: "var(--text-muted)" }} />;

function SubMenuHeader({ label, onBack }: { label: string; onBack: () => void }) {
  return (
    <button onClick={onBack} className="flex items-center gap-2 w-full px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider hover:bg-[var(--surface-raised)] transition-colors" style={{ color: "var(--text-muted)" }}>
      <ChevronLeft size={14} />
      {label}
    </button>
  );
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
  playbackSpeed = 1,
  onSpeedChange,
  open,
  onToggle,
  autoQuality,
  onAutoQualityToggle,
  autoQualityLabel,
  onExternalSubtitle,
  subtitleStyle,
  onSubtitleStyleChange,
}: SettingsPanelProps) {
  const { t } = useTranslation();
  const QUALITY_OPTIONS = useQualityOptions();
  const subtitleFontSize = subtitleStyle?.fontSize ?? 100;
  const subtitleColor = subtitleStyle?.color ?? "#ffffff";
  const subtitleFontFamily = subtitleStyle?.fontFamily ?? "";
  const subtitleBgColor = subtitleStyle?.bgColor ?? "#000000";
  const subtitleBgOpacity = subtitleStyle?.bgOpacity ?? 75;
  const subtitlePosition = subtitleStyle?.position ?? "bottom";
  const subtitleShadow = subtitleStyle?.shadow ?? false;
  const extSubInputRef = useRef<HTMLInputElement>(null);
  const [menu, setMenu] = useState<"main" | "audio" | "subtitles" | "speed" | "subtitle-style">("main");

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    setMenu("main");
    onToggle();
  }

  function select(fn: () => void) {
    fn();
    setMenu("main");
    if (open) onToggle();
  }

  const currentAudioLabel = audioStreams.find((s) => s.index === currentAudioIndex)?.title
    ?? audioStreams.find((s) => s.index === currentAudioIndex)?.language
    ?? (audioStreams.length > 0 ? t("media.track", { number: currentAudioIndex }) : "—");

  const currentSubLabel = currentSubtitleTrackIndex === null
    ? t("player.settings.off")
    : (subtitleStreams.find((s) => s.trackIndex === currentSubtitleTrackIndex)?.title
      ?? subtitleStreams.find((s) => s.trackIndex === currentSubtitleTrackIndex)?.language
      ?? t("media.track", { number: currentSubtitleTrackIndex }));

  const currentSpeedLabel = playbackSpeed === 1 ? t("player.settings.normal") : `${playbackSpeed}x`;

  return (
    <div className="relative">
      <button
        onClick={toggle}
        aria-label={t("player.controls.settings")}
        title={t("player.controls.settings")}
        className="text-white hover:text-[var(--accent)] transition-colors p-1"
      >
        <Settings size={20} />
      </button>

      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-56 rounded-[var(--radius-md)] shadow-xl overflow-y-auto z-badge max-h-[80vh]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          {menu === "main" && (
            <>
              <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
                {t("player.settings.quality")}
              </div>
              {onAutoQualityToggle && (
                <button
                  onClick={() => select(() => onAutoQualityToggle(true))}
                  className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                  style={{ color: autoQuality ? "var(--accent)" : "var(--text)" }}
                >
                  <span>{t("player.settings.auto")}{autoQuality && autoQualityLabel ? ` (${autoQualityLabel})` : ""}</span>
                  {autoQuality && <CheckIcon />}
                </button>
              )}
              {QUALITY_OPTIONS.map((q) => (
                <button
                  key={q.value}
                  onClick={() => select(() => { onAutoQualityToggle?.(false); onQualityChange(q.value); })}
                  className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                  style={{ color: !autoQuality && q.value === currentQuality ? "var(--accent)" : "var(--text)" }}
                >
                  <span>{q.label}</span>
                  {!autoQuality && q.value === currentQuality && <CheckIcon />}
                </button>
              ))}

              {audioStreams.length > 0 && (
                <>
                  <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
                  <button
                    onClick={() => setMenu("audio")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: "var(--text)" }}
                  >
                    <span>{t("player.settings.audio")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs" style={{ color: "var(--text-muted)" }}>{currentAudioLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {subtitleStreams.length > 0 && (
                <>
                  <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
                  <button
                    onClick={() => setMenu("subtitles")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: "var(--text)" }}
                  >
                    <span>{t("player.settings.subtitles")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs" style={{ color: "var(--text-muted)" }}>{currentSubLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {onSpeedChange && (
                <>
                  <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
                  <button
                    onClick={() => setMenu("speed")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: "var(--text)" }}
                  >
                    <span>{t("player.settings.speed")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs" style={{ color: "var(--text-muted)" }}>{currentSpeedLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {onSubtitleStyleChange && (
                <>
                  <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
                  <button
                    onClick={() => setMenu("subtitle-style")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: "var(--text)" }}
                  >
                    <span>{t("player.settings.subtitleStyle")}</span>
                    <ChevronRightIcon />
                  </button>
                </>
              )}
            </>
          )}

          {menu === "audio" && (
            <>
              <SubMenuHeader label={t("player.settings.audio")} onBack={() => setMenu("main")} />
              {audioStreams.map((s) => {
                const label = s.title ?? s.language ?? t("media.track", { number: s.index });
                return (
                  <button
                    key={s.index}
                    onClick={() => select(() => onAudioChange(s.index))}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: s.index === currentAudioIndex ? "var(--accent)" : "var(--text)" }}
                  >
                    <span>{label}</span>
                    {s.index === currentAudioIndex && <CheckIcon />}
                  </button>
                );
              })}
            </>
          )}

          {menu === "subtitles" && (
            <>
              <SubMenuHeader label={t("player.settings.subtitles")} onBack={() => setMenu("main")} />
              <button
                onClick={() => select(() => onSubtitleChange(null))}
                className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                style={{ color: currentSubtitleTrackIndex === null ? "var(--accent)" : "var(--text)" }}
              >
                <span>{t("player.settings.off")}</span>
                {currentSubtitleTrackIndex === null && <CheckIcon />}
              </button>
              {subtitleStreams.map((s) => {
                const label = s.title ?? s.language ?? t("media.track", { number: s.index });
                return (
                  <button
                    key={s.index}
                    onClick={() => select(() => onSubtitleChange(s.trackIndex))}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: s.trackIndex === currentSubtitleTrackIndex ? "var(--accent)" : "var(--text)" }}
                  >
                    <span>
                      {label}
                      {s.burnIn && (
                        <span className="ml-1 text-xs opacity-60">{t("media.burnIn")}</span>
                      )}
                    </span>
                    {s.trackIndex === currentSubtitleTrackIndex && <CheckIcon />}
                  </button>
                );
              })}
              {onExternalSubtitle && (
                <>
                  <div className="mx-3 my-1" style={{ borderTop: "1px solid var(--border)" }} />
                  <input
                    ref={extSubInputRef}
                    type="file"
                    accept=".srt,.vtt,.ass,.ssa"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (!file) return;
                      const name = file.name;
                      void loadSubtitleFile(file).then((url) => {
                        onExternalSubtitle(url, name);
                        setMenu("main");
                        if (open) onToggle();
                      });
                      e.target.value = "";
                    }}
                  />
                  <button
                    onClick={() => extSubInputRef.current?.click()}
                    className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                    style={{ color: "var(--text)" }}
                  >
                    <Upload size={16} className="flex-shrink-0" />
                    {t("player.settings.uploadSubtitle")}
                  </button>
                </>
              )}
            </>
          )}

          {menu === "speed" && onSpeedChange && (
            <>
              <SubMenuHeader label={t("player.settings.speed")} onBack={() => setMenu("main")} />
              {SPEED_OPTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => select(() => onSpeedChange(s))}
                  className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors"
                  style={{ color: s === playbackSpeed ? "var(--accent)" : "var(--text)" }}
                >
                  <span>{s === 1 ? t("player.settings.normal") : `${s}x`}</span>
                  {s === playbackSpeed && <CheckIcon />}
                </button>
              ))}
            </>
          )}

          {menu === "subtitle-style" && onSubtitleStyleChange && (
            <>
              <SubMenuHeader label={t("player.settings.subtitleStyle")} onBack={() => setMenu("main")} />
              <div className="px-4 py-2">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
                    {t("player.subtitleStyle.size")}
                  </span>
                  <span className="text-xs" style={{ color: "var(--text-muted)" }}>{subtitleFontSize}%</span>
                </div>
                <input
                  type="range"
                  min={50}
                  max={200}
                  step={10}
                  value={subtitleFontSize}
                  onChange={(e) => onSubtitleStyleChange({ fontSize: Number(e.target.value) })}
                  className="w-full accent-[var(--accent)]"
                  aria-label="Subtitle font size"
                />
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2" style={{ color: "var(--text-muted)" }}>
                  {t("player.subtitleStyle.textColor")}
                </span>
                <div className="flex gap-2">
                  {SUBTITLE_COLORS.map((c) => (
                    <button
                      key={c.value}
                      title={t(c.labelKey)}
                      onClick={() => onSubtitleStyleChange({ color: c.value })}
                      className="w-5 h-5 rounded-full transition-transform hover:scale-110"
                      style={{
                        backgroundColor: c.value,
                        outline: subtitleColor === c.value ? "2px solid var(--accent)" : "2px solid transparent",
                        outlineOffset: "2px",
                      }}
                    />
                  ))}
                </div>
              </div>
              <div className="px-4 py-2">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-xs font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>
                    {t("player.subtitleStyle.background")}
                  </span>
                  <span className="text-xs" style={{ color: "var(--text-muted)" }}>{subtitleBgOpacity}%</span>
                </div>
                <div className="flex gap-2 mb-2">
                  {SUBTITLE_BG_COLORS.map((c) => (
                    <button
                      key={c.labelKey}
                      title={t(c.labelKey)}
                      onClick={() => onSubtitleStyleChange({ bgColor: c.value, bgOpacity: c.opacity !== undefined ? c.opacity : subtitleBgOpacity })}
                      className="w-5 h-5 rounded transition-transform hover:scale-110 border border-[var(--border)]"
                      style={{
                        backgroundColor: c.value,
                        outline: subtitleBgColor === c.value ? "2px solid var(--accent)" : "2px solid transparent",
                        outlineOffset: "2px",
                      }}
                    />
                  ))}
                </div>
                <input
                  type="range"
                  min={0}
                  max={100}
                  step={5}
                  value={subtitleBgOpacity}
                  onChange={(e) => onSubtitleStyleChange({ bgOpacity: Number(e.target.value) })}
                  className="w-full accent-[var(--accent)]"
                  aria-label="Background opacity"
                />
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2" style={{ color: "var(--text-muted)" }}>
                  {t("player.subtitleStyle.font")}
                </span>
                <select
                  value={subtitleFontFamily}
                  onChange={(e) => onSubtitleStyleChange({ fontFamily: e.target.value })}
                  className="w-full text-sm px-2 py-1 rounded"
                  style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)", border: "1px solid var(--border)" }}
                >
                  {SUBTITLE_FONTS.map((f) => (
                    <option key={f.value} value={f.value}>{t(f.labelKey)}</option>
                  ))}
                </select>
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2" style={{ color: "var(--text-muted)" }}>
                  {t("player.subtitleStyle.position")}
                </span>
                <div className="flex gap-2">
                  {(["bottom", "top"] as const).map((pos) => (
                    <button
                      key={pos}
                      onClick={() => onSubtitleStyleChange({ position: pos })}
                      className="flex-1 text-xs py-1 rounded transition-colors"
                      style={{
                        backgroundColor: subtitlePosition === pos ? "var(--accent)" : "var(--surface-raised)",
                        color: subtitlePosition === pos ? "var(--filter-active-text)" : "var(--text)",
                        border: "1px solid var(--border)",
                      }}
                    >
                      {t(`player.subtitleStyle.${pos}`)}
                    </button>
                  ))}
                </div>
              </div>
              <div className="px-4 py-2">
                <button
                  onClick={() => onSubtitleStyleChange({ shadow: !subtitleShadow })}
                  className="flex items-center gap-2 text-xs w-full"
                  style={{ color: "var(--text)" }}
                >
                  <span
                    className="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0"
                    style={{
                      backgroundColor: subtitleShadow ? "var(--accent)" : "transparent",
                      borderColor: subtitleShadow ? "var(--accent)" : "var(--border)",
                    }}
                  >
                    {subtitleShadow && (
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="white">
                        <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                      </svg>
                    )}
                  </span>
                  <span className="font-semibold uppercase tracking-wider" style={{ color: "var(--text-muted)" }}>{t("player.subtitleStyle.textShadow")}</span>
                </button>
              </div>
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
  playbackSpeed = 1,
  onSpeedChange,
  seasons = [],
  currentSeasonId = "",
  episodes = [],
  currentEpisodeId = "",
  onSeasonChange,
  onEpisodeChange,
  chapters = [],
  mediaId,
  mediaType,
  onSeekRequest,
  transcodeOffset = 0,
  autoQuality = false,
  onAutoQualityToggle,
  autoQualityLabel,
  skipForwardSeconds = 10,
  skipBackwardSeconds = 10,
  onExternalSubtitle,
  subtitleStyle,
  onSubtitleStyleChange,
  onPanelOpenChange,
  closePanelRef,
  onActionFlash,
  streamToken,
}: PlayerControlsProps) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(true);
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedEnd, setBufferedEnd] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [openPanel, setOpenPanel] = useState<"settings" | "episodes" | "chapters" | null>(null);
  const settingsOpen = openPanel === "settings";
  const episodesOpen = openPanel === "episodes";
  const chaptersOpen = openPanel === "chapters";

  useEffect(() => {
    onPanelOpenChange?.(openPanel !== null);
  }, [openPanel, onPanelOpenChange]);

  useEffect(() => {
    if (closePanelRef) closePanelRef.current = () => setOpenPanel(null);
    return () => { if (closePanelRef) closePanelRef.current = null; };
  }, [closePanelRef]);
  const [isPip, setIsPip] = useState(false);
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
    let lastTimeUpdate = 0;
    function onTimeUpdate() {
      const now = performance.now();
      if (now - lastTimeUpdate < 250) return;
      lastTimeUpdate = now;
      setCurrentTime(video!.currentTime);
    }
    function onDurationChange() {
      const d = video!.duration;
      if (d && isFinite(d) && d > 0) setDuration(d);
    }
    function onVolumeChange() { setVolume(video!.volume); setMuted(video!.muted); }
    function onFullscreenChange() { setFullscreen(!!document.fullscreenElement); }
    function onEnterPip() { setIsPip(true); }
    function onLeavePip() { setIsPip(false); }
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
    video.addEventListener("enterpictureinpicture", onEnterPip);
    video.addEventListener("leavepictureinpicture", onLeavePip);
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
      video.removeEventListener("enterpictureinpicture", onEnterPip);
      video.removeEventListener("leavepictureinpicture", onLeavePip);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [videoRef]);

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  const absoluteTime = transcodeOffset + currentTime;

  function handleSeek(e: React.ChangeEvent<HTMLInputElement>) {
    const targetAbsolute = Number(e.target.value);
    if (onSeekRequest) {
      onSeekRequest(targetAbsolute);
    } else {
      const video = videoRef.current;
      if (video) video.currentTime = targetAbsolute;
    }
  }

  function skipForward() {
    const target = Math.min(effectiveDuration, absoluteTime + skipForwardSeconds);
    if (onSeekRequest) onSeekRequest(target);
    else if (videoRef.current) videoRef.current.currentTime += skipForwardSeconds;
  }

  function skipBackward() {
    const target = Math.max(0, absoluteTime - skipBackwardSeconds);
    if (onSeekRequest) onSeekRequest(target);
    else if (videoRef.current) videoRef.current.currentTime -= skipBackwardSeconds;
  }

  const prevEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx <= 0) return null;
    return episodes[idx - 1];
  })();

  function handleVolume(e: React.ChangeEvent<HTMLInputElement>) {
    const video = videoRef.current;
    if (!video) return;
    video.volume = Number(e.target.value);
  }

  function togglePip() {
    const video = videoRef.current;
    if (!video || !document.pictureInPictureEnabled) return;
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture();
    } else {
      void video.requestPictureInPicture();
    }
  }

  function toggleMute() {
    const video = videoRef.current;
    if (video) video.muted = !video.muted;
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
    if ((e.target as HTMLElement).closest("button, input, [data-controls]")) return;
    if (openPanel) {
      setOpenPanel(null);
      return;
    }
    if (clickTimer.current) {
      clearTimeout(clickTimer.current);
      clickTimer.current = null;
      toggleFullscreen();
    } else {
      clickTimer.current = setTimeout(() => {
        clickTimer.current = null;
        const video = videoRef.current;
        if (video) {
          if (video.paused) {
            void video.play();
            onActionFlash?.("play");
          } else {
            video.pause();
            onActionFlash?.("pause");
          }
        }
        resetHideTimer();
      }, 200);
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
        style={{ background: "linear-gradient(to bottom, var(--overlay-bg-strong), transparent)" }}
        data-controls
      >
        <button
          onClick={onClose}
          aria-label={t("common.back")}
          className="text-white hover:text-[var(--accent)] transition-colors"
        >
          <ArrowLeft size={24} />
        </button>
        {title && <span className="text-white text-sm font-medium truncate">{title}</span>}
      </div>

      {/* bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 px-4 pb-4 pt-8 flex flex-col gap-2 transition-opacity duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to top, var(--overlay-bg-strong), transparent)" }}
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
          {/* hover tooltip with trickplay preview + chapter */}
          {seekHoverPos !== null && (() => {
            const hoverMs = seekHoverPos * 1000;
            const hoverChapter = chapters.find((ch, i) =>
              hoverMs >= ch.startTimeMs && (i === chapters.length - 1 || hoverMs < chapters[i + 1].startTimeMs)
            );
            return (
              <div
                className="absolute bottom-full mb-2 -translate-x-1/2 flex flex-col items-center pointer-events-none"
                style={{ left: seekHoverX }}
              >
                {mediaId && (
                  <TrickplayPreview mediaId={mediaId} hoverTime={seekHoverPos} mediaType={mediaType} streamToken={streamToken} />
                )}
                <div
                  className="px-2 py-1 rounded text-xs text-white mt-1 text-center"
                  style={{ backgroundColor: "var(--overlay-scrim)", maxWidth: 200 }}
                >
                  {hoverChapter?.title && <div className="truncate">{hoverChapter.title}</div>}
                  <div>{formatTime(seekHoverPos)}</div>
                </div>
              </div>
            );
          })()}
          {/* background track */}
          <div className="absolute left-0 right-0 h-1 rounded bg-white/20 group-hover:h-1.5 transition-all" />
          {/* buffered track */}
          <div
            className="absolute left-0 h-1 rounded bg-white/40 group-hover:h-1.5 transition-all"
            style={{ width: effectiveDuration > 0 ? `${((transcodeOffset + bufferedEnd) / effectiveDuration) * 100}%` : "0%" }}
          />
          {/* progress track */}
          <div
            className="absolute left-0 h-1 rounded group-hover:h-1.5 transition-all"
            style={{
              width: effectiveDuration > 0 ? `${(absoluteTime / effectiveDuration) * 100}%` : "0%",
              backgroundColor: "var(--accent)",
            }}
          />
          {/* chapter markers — clickable ticks above the range input */}
          <ChapterMarkers
            chapters={chapters}
            durationSec={effectiveDuration}
            onSeek={(t) => onSeekRequest ? onSeekRequest(t) : (videoRef.current && (videoRef.current.currentTime = t))}
          />
          <input
            type="range"
            min={0}
            max={effectiveDuration || 0}
            step={0.5}
            value={absoluteTime}
            onChange={handleSeek}
            className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-raised"
            aria-label={t("player.controls.seek")}
          />
        </div>

        <div className="flex items-center gap-2 sm:gap-3">
          {/* previous episode */}
          {prevEpisode && onEpisodeChange && (
            <button
              onClick={() => onEpisodeChange(prevEpisode.id)}
              aria-label={t("player.controls.previousEpisode")}
              className="text-white hover:text-[var(--accent)] transition-colors p-1"
              title={prevEpisode.title ? `E${prevEpisode.indexNumber} - ${prevEpisode.title}` : undefined}
            >
              <SkipBack size={22} fill="currentColor" />
            </button>
          )}

          {/* skip backward */}
          <button
            onClick={skipBackward}
            aria-label={t("player.controls.rewind", { seconds: skipBackwardSeconds })}
            title={`${t("player.controls.rewind", { seconds: skipBackwardSeconds })} (J)`}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
              <path d="M11.99 5V1l-5 5 5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
              <text x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="currentColor">{skipBackwardSeconds}</text>
            </svg>
          </button>

          {/* play/pause */}
          <button
            onClick={togglePlay}
            aria-label={playing ? t("player.controls.pause") : t("player.controls.play")}
            title={playing ? `${t("player.controls.pause")} (K)` : `${t("player.controls.play")} (K)`}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            {playing ? (
              <Pause size={28} fill="currentColor" />
            ) : (
              <Play size={28} fill="currentColor" />
            )}
          </button>

          {/* skip forward */}
          <button
            onClick={skipForward}
            aria-label={t("player.controls.forward", { seconds: skipForwardSeconds })}
            title={`${t("player.controls.forward", { seconds: skipForwardSeconds })} (L)`}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12.01 5V1l5 5-5 5V7c-3.31 0-6 2.69-6 6s2.69 6 6 6 6-2.69 6-6h2c0 4.42-3.58 8-8 8s-8-3.58-8-8 3.58-8 8-8z" />
              <text x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="currentColor">{skipForwardSeconds}</text>
            </svg>
          </button>

          {/* next episode */}
          {episodes && episodes.length > 0 && onEpisodeChange && (() => {
            const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
            const next = idx >= 0 && idx < episodes.length - 1 ? episodes[idx + 1] : null;
            if (!next) return null;
            return (
              <button
                onClick={() => onEpisodeChange(next.id)}
                aria-label={t("player.controls.nextEpisode")}
                className="text-white hover:text-[var(--accent)] transition-colors p-1"
                title={next.title ? `E${next.indexNumber} - ${next.title}` : undefined}
              >
                <SkipForward size={22} fill="currentColor" />
              </button>
            );
          })()}

          {/* time */}
          <span className="text-white text-xs tabular-nums">
            {formatTime(absoluteTime)} / {formatTime(effectiveDuration)}
          </span>

          {/* volume */}
          <div className="flex items-center gap-1">
            <button
              onClick={toggleMute}
              aria-label={muted || volume === 0 ? t("player.controls.unmute") : t("player.controls.mute")}
              title={muted || volume === 0 ? `${t("player.controls.unmute")} (M)` : `${t("player.controls.mute")} (M)`}
              className="text-white hover:text-[var(--accent)] transition-colors p-1"
            >
              {muted || volume === 0 ? (
                <VolumeX size={18} />
              ) : volume < 0.5 ? (
                <Volume1 size={18} />
              ) : (
                <Volume2 size={18} />
              )}
            </button>
            <input
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={muted ? 0 : volume}
              onChange={handleVolume}
              className="w-20 accent-[var(--accent)] cursor-pointer hidden sm:block"
              aria-label={t("player.controls.volume")}
            />
          </div>

          <div className="flex-1" />

          {/* Chapters panel */}
          {chapters.length > 0 && (
            <ChaptersPanel
              chapters={chapters}
              currentTimeSec={absoluteTime}
              onSeek={(t) => onSeekRequest ? onSeekRequest(t) : (videoRef.current && (videoRef.current.currentTime = t))}
              open={chaptersOpen}
              onToggle={() => setOpenPanel(chaptersOpen ? null : "chapters")}
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
              open={episodesOpen}
              onToggle={() => setOpenPanel(episodesOpen ? null : "episodes")}
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
            playbackSpeed={playbackSpeed}
            onSpeedChange={onSpeedChange}
            open={settingsOpen}
            onToggle={() => setOpenPanel(settingsOpen ? null : "settings")}
            autoQuality={autoQuality}
            onAutoQualityToggle={onAutoQualityToggle}
            autoQualityLabel={autoQualityLabel}
            onExternalSubtitle={onExternalSubtitle}
            subtitleStyle={subtitleStyle}
            onSubtitleStyleChange={onSubtitleStyleChange}
          />

          {/* Picture-in-Picture */}
          {typeof document !== "undefined" && document.pictureInPictureEnabled && (
            <button
              onClick={togglePip}
              aria-label={isPip ? t("player.controls.exitPip") : t("player.controls.pip")}
              title={isPip ? `${t("player.controls.exitPip")} (P)` : `${t("player.controls.pip")} (P)`}
              className="text-white hover:text-[var(--accent)] transition-colors p-1"
            >
              {isPip ? (
                <PictureInPicture2 size={20} />
              ) : (
                <PictureInPicture size={20} />
              )}
            </button>
          )}

          {/* fullscreen */}
          <button
            onClick={toggleFullscreen}
            aria-label={fullscreen ? t("player.controls.exitFullscreen") : t("player.controls.fullscreen")}
            title={fullscreen ? `${t("player.controls.exitFullscreen")} (F)` : `${t("player.controls.fullscreen")} (F)`}
            className="text-white hover:text-[var(--accent)] transition-colors p-1"
          >
            {fullscreen ? (
              <Minimize size={20} />
            ) : (
              <Maximize size={20} />
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
