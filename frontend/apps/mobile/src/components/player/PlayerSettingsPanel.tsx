import { useState } from "react";
import { View, Text, Pressable, ScrollView, StyleSheet, Switch, useWindowDimensions, type StyleProp, type ViewStyle } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { formatOffsetMs, stepOffsetMs } from "./audioSync";
import { useBarGesture } from "../../hooks/useBarGesture";
import {
  DEFAULT_SUBTITLE_STYLE,
  SUBTITLE_BG_COLORS,
  SUBTITLE_TEXT_COLORS,
  type SubtitleStyle,
} from "../../lib/subtitleStyle";
import { formatAudioTrackLabel, qualityRungsFromHeights, trackDisplayName, type QualityLevel } from "@evey/api";

// The track shapes every player lists: one definition in @evey/api.
export type { PlayerAudioStream as AudioStream, PlayerSubtitleStream as SubtitleStream } from "@evey/api";
import type { PlayerAudioStream as AudioStream, PlayerSubtitleStream as SubtitleStream } from "@evey/api";

// The speed list every player offers lives in @evey/client.
export { SPEED_OPTIONS } from "@evey/client";
import { SPEED_OPTIONS } from "@evey/client";

interface PlayerSettingsPanelProps {
  audioStreams: AudioStream[];
  currentAudioIndex: number;
  onAudioChange: (index: number) => void;
  subtitleStreams: SubtitleStream[];
  currentSubtitleIndex: number | null;
  onSubtitleChange: (index: number | null) => void;
  currentQuality: number;
  onQualityChange: (quality: number) => void;
  playbackSpeed: number;
  onSpeedChange: (speed: number) => void;
  brightness: number;
  onBrightnessChange: (value: number) => void;
  onClose: () => void;
  // Audio resync (lip-sync): optional - a caller that doesn't pass
  // onAudioOffsetChange gets no offset row, same convention as the web panel.
  audioOffsetMs?: number;
  onAudioOffsetChange?: (ms: number) => void;
  // Only meaningful during transcoded playback - the server re-encodes the
  // audio with a shifted input, which direct play has no re-encode step for.
  audioOffsetAvailable?: boolean;
  // Client-rendered subtitle style/resync (SubtitleOverlay): optional, same
  // convention - a caller that doesn't pass onSubtitleStyleChange gets no
  // "Subtitle Style" entry, matching the web panel's onSubtitleStyleChange gate.
  subtitleStyle?: SubtitleStyle;
  onSubtitleStyleChange?: (patch: Partial<SubtitleStyle>) => void;
  subtitleOffsetMs?: number;
  onSubtitleOffsetChange?: (ms: number) => void;
  // Externally-picked subtitle file (SRT/VTT/ASS uploaded client-side,
  // converted to WebVTT and rendered by SubtitleOverlay - no server upload).
  // Optional, same convention as onSubtitleStyleChange: a caller that doesn't
  // pass onPickExternalSubtitle gets no "Upload" entry. externalSubtitleName
  // surfaces the loaded file as its own selectable row, independent of
  // whether it's currently active (externalSubtitleActive), mirroring how a
  // server track stays listed while unselected.
  externalSubtitleName?: string | null;
  externalSubtitleActive?: boolean;
  onPickExternalSubtitle?: () => void;
  onSelectExternalSubtitle?: () => void;
  // Heights declared by the manifest, tallest-first order irrelevant. Empty
  // (direct play, or not read yet) keeps the static ladder.
  availableQualities?: readonly QualityLevel[];
  style?: StyleProp<ViewStyle>;
  // Per-profile "auto-play next episode" toggle: optional, same convention
  // as onSubtitleStyleChange - a caller that doesn't pass
  // onAutoplayNextChange gets no row.
  autoplayNext?: boolean;
  onAutoplayNextChange?: (enabled: boolean) => void;
}

// Shared +/- control for a millisecond resync value: hold-free discrete
// steps, clamped by the caller's onChange, with a quick reset to zero. Mirrors
// the web's OffsetStepper (PlayerSettingsPanel.tsx:130-187 on the web app).
function OffsetStepper({ label, valueMs, onChange, disabled, hint }: {
  label: string;
  valueMs: number;
  onChange: (ms: number) => void;
  disabled?: boolean;
  hint?: string;
}) {
  const theme = useTheme();
  return (
    <View style={offsetStyles.block}>
      <View style={offsetStyles.header}>
        <Text style={[offsetStyles.label, { color: theme.colors.textMuted }]}>{label}</Text>
        <Text style={[offsetStyles.value, { color: valueMs !== 0 ? theme.colors.accent : theme.colors.textMuted }]}>
          {formatOffsetMs(valueMs)}
        </Text>
      </View>
      <View style={offsetStyles.row}>
        <Pressable
          onPress={() => onChange(stepOffsetMs(valueMs, -1))}
          disabled={disabled}
          style={[offsetStyles.btn, { backgroundColor: theme.colors.surfaceRaised, opacity: disabled ? 0.4 : 1 }]}
          accessible={true}
          accessibilityLabel={`${label} -`}
          accessibilityRole="button"
        >
          <Text style={[offsetStyles.btnText, { color: theme.colors.text }]}>-</Text>
        </Pressable>
        <Pressable
          onPress={() => onChange(0)}
          disabled={disabled || valueMs === 0}
          style={[offsetStyles.resetBtn, { opacity: disabled || valueMs === 0 ? 0.4 : 1 }]}
          accessible={true}
          accessibilityLabel={`${label} 0`}
          accessibilityRole="button"
        >
          <Text style={[offsetStyles.resetText, { color: theme.colors.textMuted }]}>0</Text>
        </Pressable>
        <Pressable
          onPress={() => onChange(stepOffsetMs(valueMs, 1))}
          disabled={disabled}
          style={[offsetStyles.btn, { backgroundColor: theme.colors.surfaceRaised, opacity: disabled ? 0.4 : 1 }]}
          accessible={true}
          accessibilityLabel={`${label} +`}
          accessibilityRole="button"
        >
          <Text style={[offsetStyles.btnText, { color: theme.colors.text }]}>+</Text>
        </Pressable>
      </View>
      {hint && <Text style={[offsetStyles.hint, { color: theme.colors.textMuted }]}>{hint}</Text>}
    </View>
  );
}

// The heights the quality menu offers, 0 meaning "do not force one" - which is
// what lets the native HLS player pick a rung by itself.
//
// Exported so the "no two rows do the same thing" guard can read the list
// rather than the rendered labels. The menu used to show BOTH "Auto" and
// "Original", each sending 0: two names, one behaviour, and "Original"
// described something the value never did. A label-based test would not have
// caught the duplicate coming back under a third name; a duplicate VALUE
// cannot hide.
export const QUALITY_HEIGHTS = [0, 1080, 720, 480, 360] as const;

// availableQualities is what the MANIFEST declares, when it has been read.
// Empty means "not known yet, or direct play": the static ladder stands, which
// is what this menu always showed.
//
// 🔻 Still missing next to web: an "Auto (720p)" label naming the rung that is
// actually playing. expo-video does not expose the level it picked, so that
// half needs native support - it is not a plumbing job like this one.
function useQualityOptions(availableQualities: readonly QualityLevel[]) {
  const { t } = useTranslation();
  const rungs = qualityRungsFromHeights(availableQualities);
  if (rungs.length === 0) {
    return QUALITY_HEIGHTS.map((height) => ({
      value: height,
      label: height === 0 ? t("player.quality.auto") : `${height}p`,
    }));
  }
  // The pick goes to the server as maxHeight (restartWithTracks), which caps
  // on the rung's box: the tier, not the encoded height. A scope 1080p rung is
  // encoded 1920x800, and asking for 800 served the 720p one.
  return [{ value: 0, label: t("player.quality.auto") }, ...rungs.map((r) => ({ value: r.tier, label: r.label }))];
}

// Minimalist horizontal bar for brightness (same touch mechanics as
// PlayerVolumeControl, full width of the panel).
export function HSlider({ value, min, max, onChange, accessibilityLabel, announceScale = 1 }: {
  value: number;
  min: number;
  max: number;
  onChange: (v: number) => void;
  accessibilityLabel: string;
  // Multiplier applied only to the announced accessibilityValue, for a slider
  // whose raw range is 0-1 (brightness) and should read as a 0-100 percent. The
  // already-scaled sliders (size 50-200, background 0-100) leave it at 1 - a
  // hardcoded x100 here announced 5000-20000 for those.
  announceScale?: number;
}) {
  const theme = useTheme();
  const { barProps } = useBarGesture((f) => onChange(min + f * (max - min)));
  const fillPct = ((value - min) / (max - min)) * 100;
  return (
    <View
      {...barProps}
      style={sliderStyles.touch}
      accessible={true}
      accessibilityRole="adjustable"
      accessibilityLabel={accessibilityLabel}
      accessibilityValue={{ min: Math.round(min * announceScale), max: Math.round(max * announceScale), now: Math.round(value * announceScale) }}
      onAccessibilityAction={(ev) => {
        const step = (max - min) / 10;
        if (ev.nativeEvent.actionName === "increment") onChange(Math.min(max, value + step));
        else onChange(Math.max(min, value - step));
      }}
      accessibilityActions={[{ name: "increment" }, { name: "decrement" }]}
    >
      <View style={sliderStyles.track}>
        <View style={{ width: `${fillPct}%`, backgroundColor: theme.colors.accent }} />
      </View>
      <View style={[sliderStyles.thumb, { left: `${fillPct}%` }]} />
    </View>
  );
}

// Copy of the web SettingsPanel: main menu (Quality as a direct list, then
// Audio / Subtitles / Speed rows opening a submenu, and brightness at the
// bottom), submenus with a back header. Anchors ABOVE the gear icon,
// right-aligned, like the web popup.
export function PlayerSettingsPanel({
  audioStreams,
  currentAudioIndex,
  onAudioChange,
  subtitleStreams,
  currentSubtitleIndex,
  onSubtitleChange,
  currentQuality,
  onQualityChange,
  playbackSpeed,
  onSpeedChange,
  brightness,
  onBrightnessChange,
  onClose,
  audioOffsetMs = 0,
  onAudioOffsetChange,
  audioOffsetAvailable = true,
  subtitleStyle = DEFAULT_SUBTITLE_STYLE,
  onSubtitleStyleChange,
  subtitleOffsetMs = 0,
  onSubtitleOffsetChange,
  externalSubtitleName = null,
  externalSubtitleActive = false,
  onPickExternalSubtitle,
  onSelectExternalSubtitle,
  availableQualities,
  style,
  autoplayNext,
  onAutoplayNextChange,
}: PlayerSettingsPanelProps) {
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const { height: screenH } = useWindowDimensions();
  const QUALITY_OPTIONS = useQualityOptions(availableQualities ?? []);
  const [menu, setMenu] = useState<"main" | "audio" | "subtitles" | "speed" | "subtitle-style">("main");

  const select = (fn: () => void) => {
    fn();
    onClose();
  };

  const currentAudioLabel =
    audioStreams.find((s) => s.index === currentAudioIndex)?.title
    ?? audioStreams.find((s) => s.index === currentAudioIndex)?.language
    ?? (audioStreams.length > 0 ? t("media.track", { number: currentAudioIndex }) : "-");

  const currentSubLabel = currentSubtitleIndex === null
    ? (externalSubtitleActive && externalSubtitleName ? externalSubtitleName : t("player.settings.off"))
    : (subtitleStreams.find((s) => s.index === currentSubtitleIndex)?.title
      ?? subtitleStreams.find((s) => s.index === currentSubtitleIndex)?.language
      ?? t("media.track", { number: currentSubtitleIndex }));

  const currentSpeedLabel = playbackSpeed === 1 ? t("player.settings.normal") : `${playbackSpeed}x`;

  function OptionRow({ label, active, onPress, suffix }: { label: string; active: boolean; onPress: () => void; suffix?: string }) {
    return (
      <Pressable
        style={styles.optionRow}
        onPress={onPress}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="button"
      >
        <Text style={[styles.optionLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
          {label}
          {suffix ? <Text style={styles.optionSuffix}> {suffix}</Text> : null}
        </Text>
        {active && <Ionicons name="checkmark" size={ICON_SIZE.md} color={theme.colors.accent} />}
      </Pressable>
    );
  }

  function NavRow({ label, value, onPress }: { label: string; value: string; onPress: () => void }) {
    return (
      <Pressable
        style={styles.optionRow}
        onPress={onPress}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="button"
      >
        <Text style={[styles.optionLabel, { color: theme.colors.text }]}>{label}</Text>
        <View style={styles.navValue}>
          <Text style={[styles.navValueText, { color: theme.colors.textMuted }]} numberOfLines={1}>{value}</Text>
          <Ionicons name="chevron-forward" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
        </View>
      </Pressable>
    );
  }

  // Same row as NavRow but with no "current value" preview - used for the
  // Subtitle Style entry, which is a settings screen, not a picker (mirrors
  // the web panel's plain chevron button for the same entry).
  function ChevronRow({ label, onPress }: { label: string; onPress: () => void }) {
    return (
      <Pressable
        style={styles.optionRow}
        onPress={onPress}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="button"
      >
        <Text style={[styles.optionLabel, { color: theme.colors.text }]}>{label}</Text>
        <Ionicons name="chevron-forward" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
      </Pressable>
    );
  }

  function SubMenuHeader({ label }: { label: string }) {
    return (
      <Pressable
        style={styles.subHeader}
        onPress={() => setMenu("main")}
        accessible={true}
        accessibilityLabel={t("common.back")}
        accessibilityRole="button"
      >
        <Ionicons name="chevron-back" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
        <Text style={[styles.subHeaderText, { color: theme.colors.textMuted }]}>{label}</Text>
      </Pressable>
    );
  }

  const Divider = () => <View style={[styles.divider, { backgroundColor: theme.colors.border }]} />;

  return (
    <View
      // Trap the screen reader inside the open settings overlay: without this
      // the reader can wander onto the player controls behind it.
      accessibilityViewIsModal={true}
      style={[
        styles.panel,
        { backgroundColor: theme.colors.surface, borderColor: theme.colors.border },
        style,
      ]}
    >
      {/* The height constraint lives on the ScrollView, not on the panel:
          a maxHeight on the parent clips the content without bounding the
          scroll viewport (the ScrollView thinks it has the content's height
          and never scrolls). */}
      <ScrollView style={{ maxHeight: screenH * 0.6 }}>
        {menu === "main" && (
          <>
            <Text style={[styles.sectionHeader, { color: theme.colors.textMuted }]}>
              {t("player.settings.quality")}
            </Text>
            {QUALITY_OPTIONS.map((q) => (
              <OptionRow
                key={q.value}
                label={q.label}
                active={q.value === currentQuality}
                onPress={() => select(() => onQualityChange(q.value))}
              />
            ))}
            {audioStreams.length > 0 && (
              <>
                <Divider />
                <NavRow label={t("player.settings.audio")} value={currentAudioLabel} onPress={() => setMenu("audio")} />
              </>
            )}
            {(subtitleStreams.length > 0 || !!onPickExternalSubtitle || !!externalSubtitleName) && (
              <>
                <Divider />
                <NavRow label={t("player.settings.subtitles")} value={currentSubLabel} onPress={() => setMenu("subtitles")} />
              </>
            )}
            <Divider />
            <NavRow label={t("player.settings.speed")} value={currentSpeedLabel} onPress={() => setMenu("speed")} />
            {onAutoplayNextChange && (
              <>
                <Divider />
                <View style={styles.optionRow} accessibilityLabel={t("player.settings.autoplayNext")}>
                  <Text style={[styles.optionLabel, { color: theme.colors.text }]}>
                    {t("player.settings.autoplayNext")}
                  </Text>
                  <Switch
                    value={autoplayNext ?? true}
                    onValueChange={onAutoplayNextChange}
                    accessibilityRole="switch"
                    trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
                  />
                </View>
              </>
            )}
            {onSubtitleStyleChange && (
              <>
                <Divider />
                <ChevronRow label={t("player.settings.subtitleStyle")} onPress={() => setMenu("subtitle-style")} />
              </>
            )}
            <Divider />
            <View style={styles.brightnessBlock}>
              <View style={styles.brightnessHeader}>
                <Text style={[styles.sectionHeaderInline, { color: theme.colors.textMuted }]}>
                  {t("player.settings.brightness")}
                </Text>
                <Text style={[styles.brightnessPct, { color: theme.colors.textMuted }]}>
                  {Math.round(brightness * 100)}%
                </Text>
              </View>
              <HSlider
                value={brightness}
                min={0.05}
                max={1}
                onChange={onBrightnessChange}
                accessibilityLabel={t("player.settings.brightness")}
                announceScale={100}
              />
            </View>
          </>
        )}

        {menu === "audio" && (
          <>
            <SubMenuHeader label={t("player.settings.audio")} />
            {onAudioOffsetChange && (
              <>
                <OffsetStepper
                  label={t("player.settings.audioSync")}
                  valueMs={audioOffsetMs}
                  onChange={onAudioOffsetChange}
                  disabled={!audioOffsetAvailable}
                  hint={audioOffsetAvailable ? t("player.settings.audioSyncHint") : t("player.settings.audioSyncUnavailable")}
                />
                <Divider />
              </>
            )}
            {audioStreams.map((s) => {
              const label = formatAudioTrackLabel(s, t("media.track", { number: s.index }), i18n.language);
              return (
                <OptionRow
                  key={s.index}
                  label={label}
                  active={s.index === currentAudioIndex}
                  onPress={() => select(() => onAudioChange(s.index))}
                />
              );
            })}
          </>
        )}

        {menu === "subtitles" && (
          <>
            <SubMenuHeader label={t("player.settings.subtitles")} />
            <OptionRow
              label={t("player.settings.off")}
              active={currentSubtitleIndex === null}
              onPress={() => select(() => onSubtitleChange(null))}
            />
            {subtitleStreams.map((s) => {
              const label = trackDisplayName(s, t("media.track", { number: s.index }), i18n.language);
              return (
                <OptionRow
                  key={s.index}
                  label={label}
                  suffix={s.burnIn ? t("media.burnIn") : undefined}
                  active={s.index === currentSubtitleIndex}
                  onPress={() => select(() => onSubtitleChange(s.index))}
                />
              );
            })}
            {externalSubtitleName && (
              <OptionRow
                label={externalSubtitleName}
                active={externalSubtitleActive}
                onPress={() => select(() => onSelectExternalSubtitle?.())}
              />
            )}
            {onPickExternalSubtitle && (
              <>
                <Divider />
                <Pressable
                  style={styles.optionRow}
                  onPress={onPickExternalSubtitle}
                  accessible={true}
                  accessibilityLabel={t("player.settings.uploadSubtitle")}
                  accessibilityRole="button"
                >
                  <View style={styles.uploadRow}>
                    <Ionicons name="cloud-upload-outline" size={ICON_SIZE.sm} color={theme.colors.text} />
                    <Text style={[styles.optionLabel, { color: theme.colors.text }]}>
                      {t("player.settings.uploadSubtitle")}
                    </Text>
                  </View>
                </Pressable>
              </>
            )}
          </>
        )}

        {menu === "speed" && (
          <>
            <SubMenuHeader label={t("player.settings.speed")} />
            {SPEED_OPTIONS.map((s) => (
              <OptionRow
                key={s}
                label={s === 1 ? t("player.settings.normal") : `${s}x`}
                active={s === playbackSpeed}
                onPress={() => select(() => onSpeedChange(s))}
              />
            ))}
          </>
        )}

        {menu === "subtitle-style" && onSubtitleStyleChange && (
          <>
            <SubMenuHeader label={t("player.settings.subtitleStyle")} />
            {onSubtitleOffsetChange && (
              <OffsetStepper
                label={t("player.settings.subtitleSync")}
                valueMs={subtitleOffsetMs}
                onChange={onSubtitleOffsetChange}
                hint={t("player.settings.subtitleSyncHint")}
              />
            )}
            <View style={subtitleStyleStyles.block}>
              <View style={subtitleStyleStyles.header}>
                <Text style={[subtitleStyleStyles.label, { color: theme.colors.textMuted }]}>
                  {t("player.subtitleStyle.size")}
                </Text>
                <Text style={[subtitleStyleStyles.value, { color: theme.colors.textMuted }]}>
                  {subtitleStyle.fontSize}%
                </Text>
              </View>
              <HSlider
                value={subtitleStyle.fontSize}
                min={50}
                max={200}
                onChange={(v) => onSubtitleStyleChange({ fontSize: Math.round(v / 10) * 10 })}
                accessibilityLabel={t("player.subtitleStyle.size")}
              />
            </View>
            <View style={subtitleStyleStyles.block}>
              <Text style={[subtitleStyleStyles.label, { color: theme.colors.textMuted }]}>
                {t("player.subtitleStyle.textColor")}
              </Text>
              <View style={subtitleStyleStyles.swatchRow}>
                {SUBTITLE_TEXT_COLORS.map((c) => (
                  <Pressable
                    key={c.value}
                    onPress={() => onSubtitleStyleChange({ color: c.value })}
                    accessible={true}
                    accessibilityLabel={t(c.labelKey)}
                    accessibilityRole="button"
                    style={[
                      subtitleStyleStyles.swatch,
                      {
                        backgroundColor: c.value,
                        borderColor: subtitleStyle.color === c.value ? theme.colors.accent : "transparent",
                      },
                    ]}
                  />
                ))}
              </View>
            </View>
            <View style={subtitleStyleStyles.block}>
              <View style={subtitleStyleStyles.header}>
                <Text style={[subtitleStyleStyles.label, { color: theme.colors.textMuted }]}>
                  {t("player.subtitleStyle.background")}
                </Text>
                <Text style={[subtitleStyleStyles.value, { color: theme.colors.textMuted }]}>
                  {subtitleStyle.bgOpacity}%
                </Text>
              </View>
              <View style={subtitleStyleStyles.swatchRow}>
                {SUBTITLE_BG_COLORS.map((c) => (
                  <Pressable
                    key={c.labelKey}
                    onPress={() => onSubtitleStyleChange({
                      bgColor: c.value,
                      bgOpacity: c.opacity !== undefined ? c.opacity : subtitleStyle.bgOpacity,
                    })}
                    accessible={true}
                    accessibilityLabel={t(c.labelKey)}
                    accessibilityRole="button"
                    style={[
                      subtitleStyleStyles.swatch,
                      subtitleStyleStyles.swatchBordered,
                      {
                        backgroundColor: c.value,
                        borderColor: subtitleStyle.bgColor === c.value ? theme.colors.accent : theme.colors.border,
                      },
                    ]}
                  />
                ))}
              </View>
              <HSlider
                value={subtitleStyle.bgOpacity}
                min={0}
                max={100}
                onChange={(v) => onSubtitleStyleChange({ bgOpacity: Math.round(v / 5) * 5 })}
                accessibilityLabel={t("player.subtitleStyle.background")}
              />
            </View>
            <View style={subtitleStyleStyles.block}>
              <Text style={[subtitleStyleStyles.label, { color: theme.colors.textMuted }]}>
                {t("player.subtitleStyle.position")}
              </Text>
              <View style={subtitleStyleStyles.posRow}>
                {(["bottom", "top"] as const).map((pos) => (
                  <Pressable
                    key={pos}
                    onPress={() => onSubtitleStyleChange({ position: pos })}
                    accessible={true}
                    accessibilityLabel={t(`player.subtitleStyle.${pos}`)}
                    accessibilityRole="button"
                    style={[
                      subtitleStyleStyles.posBtn,
                      {
                        backgroundColor: subtitleStyle.position === pos ? theme.colors.accent : theme.colors.surfaceRaised,
                        borderColor: theme.colors.border,
                      },
                    ]}
                  >
                    <Text
                      style={{
                        fontSize: FONT_SIZE.xs,
                        color: subtitleStyle.position === pos ? theme.colors.onAccent : theme.colors.text,
                      }}
                    >
                      {t(`player.subtitleStyle.${pos}`)}
                    </Text>
                  </Pressable>
                ))}
              </View>
            </View>
            <Pressable
              onPress={() => onSubtitleStyleChange({ shadow: !subtitleStyle.shadow })}
              style={subtitleStyleStyles.shadowRow}
              accessible={true}
              accessibilityLabel={t("player.subtitleStyle.textShadow")}
              accessibilityRole="button"
            >
              <View
                style={[
                  subtitleStyleStyles.checkbox,
                  {
                    borderColor: theme.colors.border,
                    backgroundColor: subtitleStyle.shadow ? theme.colors.accent : "transparent",
                  },
                ]}
              >
                {subtitleStyle.shadow && <Ionicons name="checkmark" size={10} color={theme.colors.onAccent} />}
              </View>
              <Text style={[subtitleStyleStyles.label, { color: theme.colors.textMuted }]}>
                {t("player.subtitleStyle.textShadow")}
              </Text>
            </Pressable>
          </>
        )}
      </ScrollView>
    </View>
  );
}

const subtitleStyleStyles = StyleSheet.create({
  block: {
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 6,
  },
  label: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  value: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
  },
  swatchRow: {
    flexDirection: "row",
    gap: 10,
    marginTop: 4,
    marginBottom: 6,
  },
  swatch: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
  },
  swatchBordered: {
    borderRadius: 4,
    borderWidth: 1,
  },
  posRow: {
    flexDirection: "row",
    gap: 8,
    marginTop: 4,
  },
  posBtn: {
    flex: 1,
    paddingVertical: 6,
    borderRadius: 6,
    borderWidth: StyleSheet.hairlineWidth,
    alignItems: "center",
  },
  shadowRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  checkbox: {
    width: 16,
    height: 16,
    borderRadius: 4,
    borderWidth: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});

const offsetStyles = StyleSheet.create({
  block: {
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 6,
  },
  label: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  value: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  btn: {
    flex: 1,
    paddingVertical: 6,
    borderRadius: 6,
    alignItems: "center",
  },
  btnText: {
    fontSize: FONT_SIZE.sm,
  },
  resetBtn: {
    paddingHorizontal: 8,
    paddingVertical: 6,
  },
  resetText: {
    fontSize: FONT_SIZE.xs,
  },
  hint: {
    marginTop: 4,
    fontSize: FONT_SIZE.xs,
    lineHeight: 15,
  },
});

const sliderStyles = StyleSheet.create({
  touch: {
    height: 28,
    justifyContent: "center",
  },
  track: {
    height: 4,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.2)",
    flexDirection: "row",
    overflow: "hidden",
  },
  thumb: {
    position: "absolute",
    top: 8,
    width: 12,
    height: 12,
    borderRadius: 6,
    marginLeft: -6,
    backgroundColor: "#fff",
  },
});

const styles = StyleSheet.create({
  panel: {
    width: 240,
    borderRadius: 8,
    borderWidth: StyleSheet.hairlineWidth,
    overflow: "hidden",
    elevation: 8,
    shadowColor: "#000",
    shadowOpacity: 0.4,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 4 },
  },
  sectionHeader: {
    paddingHorizontal: 12,
    paddingTop: 10,
    paddingBottom: 4,
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  sectionHeaderInline: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  optionRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  optionLabel: {
    fontSize: FONT_SIZE.sm,
    flexShrink: 1,
  },
  uploadRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  optionSuffix: {
    fontSize: FONT_SIZE.xs,
    opacity: 0.6,
  },
  navValue: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
    marginLeft: 12,
    flexShrink: 1,
  },
  navValueText: {
    fontSize: FONT_SIZE.xs,
    maxWidth: 110,
  },
  subHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingHorizontal: 12,
    paddingTop: 10,
    paddingBottom: 6,
  },
  subHeaderText: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    marginHorizontal: 12,
    marginVertical: 4,
  },
  brightnessBlock: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    paddingBottom: 14,
  },
  brightnessHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 4,
  },
  brightnessPct: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
  },
});
