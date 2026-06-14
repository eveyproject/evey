import { useRef, useState } from "react";
import { Check, ChevronLeft, ChevronRight, Settings, Upload } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import { SPEED_OPTIONS } from "./constants";

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

const CheckIcon = () => <Check size={14} aria-hidden="true" />;

const ChevronRightIcon = () => <ChevronRight size={14} className="text-[var(--text-muted)]" aria-hidden="true" />;

function SubMenuHeader({ label, onBack }: { label: string; onBack: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      onClick={onBack}
      aria-label={t("common.back")}
      title={t("common.back")}
      className="flex items-center gap-2 w-full px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
    >
      <ChevronLeft size={14} aria-hidden="true" />
      {label}
    </button>
  );
}

export interface SettingsPanelProps {
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
  brightness?: number;
  onBrightnessChange?: (value: number) => void;
  availableQualities?: number[];
}

export function SettingsPanel({
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
  brightness = 1,
  onBrightnessChange,
  availableQualities,
}: SettingsPanelProps) {
  const { t } = useTranslation();
  const allQualityOptions = useQualityOptions();
  const QUALITY_OPTIONS = availableQualities
    ? allQualityOptions.filter((q) => q.value === 0 || availableQualities.includes(q.value))
    : allQualityOptions;
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
        className="text-white hover:text-[var(--accent)] transition-colors-fast p-1"
      >
        <Settings size={20} aria-hidden="true" />
      </button>

      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-56 rounded-[var(--radius-md)] shadow-xl overflow-y-auto z-badge max-h-[80vh] bg-[var(--surface)] border border-[var(--border)]"
          onClick={(e) => e.stopPropagation()}
        >
          {menu === "main" && (
            <>
              <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                {t("player.settings.quality")}
              </div>
              {onAutoQualityToggle && (
                <button
                  onClick={() => select(() => onAutoQualityToggle(true))}
                  className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${autoQuality ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
                >
                  <span>{t("player.settings.auto")}{autoQuality && autoQualityLabel ? ` (${autoQualityLabel})` : ""}</span>
                  {autoQuality && <CheckIcon />}
                </button>
              )}
              {QUALITY_OPTIONS.map((q) => (
                <button
                  key={q.value}
                  onClick={() => select(() => { onAutoQualityToggle?.(false); onQualityChange(q.value); })}
                  className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${!autoQuality && q.value === currentQuality ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
                >
                  <span>{q.label}</span>
                  {!autoQuality && q.value === currentQuality && <CheckIcon />}
                </button>
              ))}

              {audioStreams.length > 0 && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => setMenu("audio")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.audio")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs text-[var(--text-muted)]">{currentAudioLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {subtitleStreams.length > 0 && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => setMenu("subtitles")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.subtitles")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs text-[var(--text-muted)]">{currentSubLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {onSpeedChange && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => setMenu("speed")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.speed")}</span>
                    <span className="flex items-center gap-1">
                      <span className="text-xs text-[var(--text-muted)]">{currentSpeedLabel}</span>
                      <ChevronRightIcon />
                    </span>
                  </button>
                </>
              )}

              {onSubtitleStyleChange && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => setMenu("subtitle-style")}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.subtitleStyle")}</span>
                    <ChevronRightIcon />
                  </button>
                </>
              )}

              {onBrightnessChange && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <div className="px-4 py-2">
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">{t("player.settings.brightness")}</span>
                      <span className="text-xs text-[var(--text-muted)]">{Math.round(brightness * 100)}%</span>
                    </div>
                    <input
                      type="range"
                      min="20"
                      max="150"
                      value={Math.round(brightness * 100)}
                      onChange={(e) => onBrightnessChange(Number(e.target.value) / 100)}
                      className="w-full h-1 rounded-full appearance-none cursor-pointer accent-[var(--accent)] bg-white/20"
                      aria-label={t("player.settings.brightness")}
                    />
                  </div>
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
                    className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${s.index === currentAudioIndex ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
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
                className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${currentSubtitleTrackIndex === null ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
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
                    className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${s.trackIndex === currentSubtitleTrackIndex ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
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
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <input
                    ref={extSubInputRef}
                    type="file"
                    accept=".srt,.vtt,.ass,.ssa"
                    className="hidden"
                    onChange={async (e) => {
                      const file = e.target.files?.[0];
                      if (!file) return;
                      const name = file.name;
                      try {
                        const url = await loadSubtitleFile(file);
                        onExternalSubtitle(url, name);
                        setMenu("main");
                        if (open) onToggle();
                      } catch (err) { console.warn("subtitle load failed", err); }
                      e.target.value = "";
                    }}
                  />
                  <button
                    onClick={() => extSubInputRef.current?.click()}
                    className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <Upload size={16} className="flex-shrink-0" aria-hidden="true" />
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
                  className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${s === playbackSpeed ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
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
                  <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                    {t("player.subtitleStyle.size")}
                  </span>
                  <span className="text-xs text-[var(--text-muted)]">{subtitleFontSize}%</span>
                </div>
                <input
                  type="range"
                  min={50}
                  max={200}
                  step={10}
                  value={subtitleFontSize}
                  onChange={(e) => onSubtitleStyleChange({ fontSize: Number(e.target.value) })}
                  className="w-full accent-[var(--accent)]"
                  aria-label={t("player.subtitleStyle.size")}
                />
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2 text-[var(--text-muted)]">
                  {t("player.subtitleStyle.textColor")}
                </span>
                <div className="flex gap-2">
                  {SUBTITLE_COLORS.map((c) => (
                    <button
                      key={c.value}
                      title={t(c.labelKey)}
                      aria-label={t(c.labelKey)}
                      onClick={() => onSubtitleStyleChange({ color: c.value })}
                      className="w-5 h-5 rounded-full transition-transform-fast hover:scale-110"
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
                  <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                    {t("player.subtitleStyle.background")}
                  </span>
                  <span className="text-xs text-[var(--text-muted)]">{subtitleBgOpacity}%</span>
                </div>
                <div className="flex gap-2 mb-2">
                  {SUBTITLE_BG_COLORS.map((c) => (
                    <button
                      key={c.labelKey}
                      title={t(c.labelKey)}
                      aria-label={t(c.labelKey)}
                      onClick={() => onSubtitleStyleChange({ bgColor: c.value, bgOpacity: c.opacity !== undefined ? c.opacity : subtitleBgOpacity })}
                      className="w-5 h-5 rounded transition-transform-fast hover:scale-110 border border-[var(--border)]"
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
                  aria-label={t("player.subtitleStyle.background")}
                />
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2 text-[var(--text-muted)]">
                  {t("player.subtitleStyle.font")}
                </span>
                <select
                  value={subtitleFontFamily}
                  aria-label={t("player.subtitleStyle.font")}
                  onChange={(e) => onSubtitleStyleChange({ fontFamily: e.target.value })}
                  className="w-full text-sm px-2 py-1 rounded bg-[var(--surface-raised)] text-[var(--text)] border border-[var(--border)]"
                >
                  {SUBTITLE_FONTS.map((f) => (
                    <option key={f.value} value={f.value}>{t(f.labelKey)}</option>
                  ))}
                </select>
              </div>
              <div className="px-4 py-2">
                <span className="text-xs font-semibold uppercase tracking-wider block mb-2 text-[var(--text-muted)]">
                  {t("player.subtitleStyle.position")}
                </span>
                <div className="flex gap-2">
                  {(["bottom", "top"] as const).map((pos) => (
                    <button
                      key={pos}
                      onClick={() => onSubtitleStyleChange({ position: pos })}
                      className={`flex-1 text-xs py-1 rounded transition-colors-fast border border-[var(--border)] ${subtitlePosition === pos ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface-raised)] text-[var(--text)]"}`}
                    >
                      {t(`player.subtitleStyle.${pos}`)}
                    </button>
                  ))}
                </div>
              </div>
              <div className="px-4 py-2">
                <button
                  onClick={() => onSubtitleStyleChange({ shadow: !subtitleShadow })}
                  className="flex items-center gap-2 text-xs w-full text-[var(--text)]"
                >
                  <span
                    className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${subtitleShadow ? "bg-[var(--accent)] border-[var(--accent)]" : "bg-transparent border-[var(--border)]"}`}
                  >
                    {subtitleShadow && (
                      <svg width="10" height="10" viewBox="0 0 24 24" fill="white">
                        <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
                      </svg>
                    )}
                  </span>
                  <span className="font-semibold uppercase tracking-wider text-[var(--text-muted)]">{t("player.subtitleStyle.textShadow")}</span>
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
