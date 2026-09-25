import { useRef, useState } from "react";
import { Check, ChevronLeft, ChevronRight, Settings, Upload } from "lucide-react";
import { useTranslation } from "react-i18next";
import { formatAudioTrackLabel, type AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import type { SleepTimerState } from "@/hooks/useSleepTimer";
import { SPEED_OPTIONS } from "./constants";
import { qualityRungsFromHeights, trackDisplayName } from "@evey/api";
import { SLEEP_TIMER_PRESETS_MIN, isSleepPresetActive, SUBTITLE_BG_COLORS, srtToVtt, assToVtt } from "@evey/client";

// Mirrors the mobile/TV sleep timer presets (apps/mobile/src/components/SleepTimerPicker.tsx,
// apps/tv/src/components/SleepTimerPicker.tsx).

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


const SUBTITLE_FONTS = [
  { labelKey: "player.fonts.default", value: "" },
  { labelKey: "player.fonts.sansSerif", value: "Arial, sans-serif" },
  { labelKey: "player.fonts.serif", value: "Georgia, serif" },
  { labelKey: "player.fonts.monospace", value: "'Courier New', monospace" },
  { labelKey: "player.fonts.comicSans", value: "'Comic Sans MS', cursive" },
];

// Shared with the phone: packages/client/src/lib/subtitleParse.ts.
export { srtToVtt, assToVtt };

async function loadSubtitleFile(file: File): Promise<string> {
  const text = await file.text();
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  if (ext === "srt" || ext === "ass" || ext === "ssa") {
    const vtt = ext === "srt" ? srtToVtt(text) : assToVtt(text);
    const blob = new Blob([vtt], { type: "text/vtt" });
    return URL.createObjectURL(blob);
  }
  return URL.createObjectURL(file);
}

const CheckIcon = () => <Check size={14} aria-hidden="true" />;

const ChevronRightIcon = () => <ChevronRight size={14} className="text-[var(--text-muted)]" aria-hidden="true" />;

// OFFSET_STEP_MS is the resync granularity: fine enough to dial in lip sync,
// coarse enough that a few clicks cover typical mux offsets.
const OFFSET_STEP_MS = 100;
const OFFSET_MAX_MS = 5000;

// formatOffset renders a millisecond offset as a signed seconds label.
function formatOffset(ms: number): string {
  if (ms === 0) return "0 ms";
  const sign = ms > 0 ? "+" : "-";
  const abs = Math.abs(ms);
  return abs >= 1000 ? `${sign}${(abs / 1000).toFixed(abs % 1000 === 0 ? 0 : 1)} s` : `${sign}${abs} ms`;
}

// OffsetStepper is the shared +/- control for the subtitle and audio resync
// rows: hold-free discrete steps, clamped, with a quick reset.
function OffsetStepper({ label, valueMs, onChange, disabled, hint, testId }: {
  label: string;
  valueMs: number;
  onChange: (ms: number) => void;
  disabled?: boolean;
  hint?: string;
  testId: string;
}) {
  const step = (dir: 1 | -1) => {
    const next = Math.max(-OFFSET_MAX_MS, Math.min(OFFSET_MAX_MS, valueMs + dir * OFFSET_STEP_MS));
    onChange(next);
  };
  return (
    <div className="px-4 py-2" data-testid={testId}>
      <div className="flex items-center justify-between mb-1.5">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">{label}</span>
        <span
          aria-live="polite"
          className={`text-xs ${valueMs !== 0 ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}
        >
          {formatOffset(valueMs)}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={() => step(-1)}
          disabled={disabled}
          className="flex-1 py-1 rounded bg-[var(--surface-raised)] text-sm text-[var(--text)] hover:bg-[var(--border)] disabled:opacity-40 transition-colors-fast"
          aria-label={`${label} -`}
          data-testid={`${testId}-minus`}
        >
          -
        </button>
        <button
          onClick={() => onChange(0)}
          disabled={disabled || valueMs === 0}
          aria-label={`${label} 0`}
          className="px-2 py-1 rounded text-xs text-[var(--text-muted)] hover:bg-[var(--surface-raised)] disabled:opacity-40 transition-colors-fast"
          data-testid={`${testId}-reset`}
        >
          0
        </button>
        <button
          onClick={() => step(1)}
          disabled={disabled}
          className="flex-1 py-1 rounded bg-[var(--surface-raised)] text-sm text-[var(--text)] hover:bg-[var(--border)] disabled:opacity-40 transition-colors-fast"
          aria-label={`${label} +`}
          data-testid={`${testId}-plus`}
        >
          +
        </button>
      </div>
      {hint && <p className="mt-1 text-xs leading-tight text-[var(--text-muted)]">{hint}</p>}
    </div>
  );
}

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
  /** Live subtitle resync in ms (positive = subtitles later). */
  subtitleOffsetMs?: number;
  onSubtitleOffsetChange?: (ms: number) => void;
  /** Audio resync in ms (positive = audio later). Applying restarts the
   * transcode at the current position, so callers debounce it. */
  audioOffsetMs?: number;
  onAudioOffsetChange?: (ms: number) => void;
  /** Audio resync needs a re-encode: disabled (with a hint) in direct play. */
  audioOffsetAvailable?: boolean;
  brightness?: number;
  onBrightnessChange?: (value: number) => void;
  availableQualities?: number[];
  /** Current sleep timer state, or null/undefined when no timer is active. */
  /** Per-profile choice, persisted by the caller; the row only shows when
   *  the caller can persist it (a series context). */
  autoplayNext?: boolean;
  onAutoplayNextChange?: (enabled: boolean) => void;
  sleepTimerState?: SleepTimerState | null;
  onSleepTimerMinutes?: (minutes: number) => void;
  onSleepTimerEndOfEpisode?: () => void;
  onSleepTimerCancel?: () => void;
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
  subtitleOffsetMs = 0,
  onSubtitleOffsetChange,
  audioOffsetMs = 0,
  onAudioOffsetChange,
  audioOffsetAvailable = true,
  brightness = 1,
  onBrightnessChange,
  availableQualities,
  autoplayNext = true,
  onAutoplayNextChange,
  sleepTimerState = null,
  onSleepTimerMinutes,
  onSleepTimerEndOfEpisode,
  onSleepTimerCancel,
}: SettingsPanelProps) {
  const { t } = useTranslation();
  const allQualityOptions = useQualityOptions();
  // Build the manual quality list from the ACTUAL hls rung heights, each
  // labelled by its nearest standard tier - the static list only matched 16:9
  // heights, so a widescreen ladder (e.g. 532/798) collapsed to just "Original".
  // The option value stays the real height (rung selection keys off it); dedupe
  // by tier keeping the tallest rung. Fall back to the static list before the
  // manifest is parsed.
  const QUALITY_OPTIONS = (() => {
    const rungs = qualityRungsFromHeights(availableQualities ?? []);
    if (rungs.length === 0) return allQualityOptions;
    return [{ label: t("player.quality.original"), value: 0 }, ...rungs];
  })();
  const subtitleFontSize = subtitleStyle?.fontSize ?? 100;
  const subtitleColor = subtitleStyle?.color ?? "#ffffff";
  const subtitleFontFamily = subtitleStyle?.fontFamily ?? "";
  const subtitleBgColor = subtitleStyle?.bgColor ?? "#000000";
  const subtitleBgOpacity = subtitleStyle?.bgOpacity ?? 75;
  const subtitlePosition = subtitleStyle?.position ?? "bottom";
  const subtitleShadow = subtitleStyle?.shadow ?? false;
  const extSubInputRef = useRef<HTMLInputElement>(null);
  const [menu, setMenu] = useState<"main" | "audio" | "subtitles" | "speed" | "subtitle-style" | "sleep">("main");

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
    ?? (audioStreams.length > 0 ? t("media.track", { number: currentAudioIndex }) : "-");

  const currentSubLabel = currentSubtitleTrackIndex === null
    ? t("player.settings.off")
    : (subtitleStreams.find((s) => s.trackIndex === currentSubtitleTrackIndex)?.title
      ?? subtitleStreams.find((s) => s.trackIndex === currentSubtitleTrackIndex)?.language
      ?? t("media.track", { number: currentSubtitleTrackIndex }));

  const currentSpeedLabel = playbackSpeed === 1 ? t("player.settings.normal") : `${playbackSpeed}x`;

  const currentSleepLabel = sleepTimerState === null
    ? t("player.settings.off")
    : sleepTimerState.kind === "endOfEpisode"
      ? t("player.sleepTimer.endOfEpisode")
      : t("player.sleepTimer.minutes", { count: sleepTimerState.minutesLeft ?? 0 });

  return (
    <div className="relative">
      <button
        onClick={toggle}
        data-testid="player-settings"
        aria-label={t("player.controls.settings")}
        aria-expanded={open}
        aria-haspopup="true"
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
                    data-testid="player-settings-audio"
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

              {onAutoplayNextChange && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    role="switch"
                    aria-checked={autoplayNext}
                    onClick={() => onAutoplayNextChange(!autoplayNext)}
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.autoplayNext")}</span>
                    <span className={`text-xs ${autoplayNext ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}>
                      {autoplayNext ? t("common.enabled") : t("common.disabled")}
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

              {onSleepTimerMinutes && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => setMenu("sleep")}
                    data-testid="sleep-timer-trigger"
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.settings.sleepTimer")}</span>
                    <span className="flex items-center gap-1">
                      <span className={`text-xs ${sleepTimerState !== null ? "text-[var(--accent)]" : "text-[var(--text-muted)]"}`}>{currentSleepLabel}</span>
                      <ChevronRightIcon />
                    </span>
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
              {onAudioOffsetChange && (
                <>
                  <OffsetStepper
                    label={t("player.settings.audioSync")}
                    valueMs={audioOffsetMs}
                    onChange={onAudioOffsetChange}
                    disabled={!audioOffsetAvailable}
                    hint={audioOffsetAvailable ? t("player.settings.audioSyncHint") : t("player.settings.audioSyncUnavailable")}
                    testId="audio-offset"
                  />
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                </>
              )}
              {audioStreams.map((s) => {
                const label = formatAudioTrackLabel(s, t("media.track", { number: s.index }));
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
                const label = trackDisplayName(s, t("media.track", { number: s.index }));
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

          {menu === "sleep" && onSleepTimerMinutes && (
            <>
              <SubMenuHeader label={t("player.settings.sleepTimer")} onBack={() => setMenu("main")} />
              {SLEEP_TIMER_PRESETS_MIN.map((m) => {
                const active = isSleepPresetActive(sleepTimerState, m);
                return (
                  <button
                    key={m}
                    onClick={() => select(() => onSleepTimerMinutes(m))}
                    data-testid={`sleep-timer-${m}`}
                    className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${active ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
                  >
                    <span>{t("player.sleepTimer.minutes", { count: m })}</span>
                    {active && <CheckIcon />}
                  </button>
                );
              })}
              {onSleepTimerEndOfEpisode && (
                <button
                  onClick={() => select(() => onSleepTimerEndOfEpisode())}
                  data-testid="sleep-timer-end-of-episode"
                  className={`flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast ${sleepTimerState?.kind === "endOfEpisode" ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
                >
                  <span>{t("player.sleepTimer.endOfEpisode")}</span>
                  {sleepTimerState?.kind === "endOfEpisode" && <CheckIcon />}
                </button>
              )}
              {sleepTimerState !== null && onSleepTimerCancel && (
                <>
                  <div className="mx-3 my-1 border-t border-[var(--border)]" />
                  <button
                    onClick={() => select(() => onSleepTimerCancel())}
                    data-testid="sleep-timer-cancel"
                    className="flex items-center justify-between w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text)]"
                  >
                    <span>{t("player.sleepTimer.cancel")}</span>
                  </button>
                </>
              )}
            </>
          )}

          {menu === "subtitle-style" && onSubtitleStyleChange && (
            <>
              <SubMenuHeader label={t("player.settings.subtitleStyle")} onBack={() => setMenu("main")} />
              {onSubtitleOffsetChange && (
                <OffsetStepper
                  label={t("player.settings.subtitleSync")}
                  valueMs={subtitleOffsetMs}
                  onChange={onSubtitleOffsetChange}
                  hint={t("player.settings.subtitleSyncHint")}
                  testId="subtitle-offset"
                />
              )}
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
