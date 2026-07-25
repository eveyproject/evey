import { useCallback, useRef, useState } from "react";
import { View, Text, Pressable, ScrollView, StyleSheet, useWindowDimensions, type StyleProp, type ViewStyle } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

export interface AudioStream {
  index: number;
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  channels?: number;
}

export interface SubtitleStream {
  index: number;
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  isForced?: boolean;
  codec?: string;
  burnIn?: boolean;
}

export const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;

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
  style?: StyleProp<ViewStyle>;
}

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

// Barre horizontale minimaliste pour la luminosité (mêmes mécanismes tactiles
// que PlayerVolumeControl, pleine largeur du panneau).
function HSlider({ value, min, max, onChange, accessibilityLabel }: {
  value: number;
  min: number;
  max: number;
  onChange: (v: number) => void;
  accessibilityLabel: string;
}) {
  const theme = useTheme();
  const widthRef = useRef(0);
  const valueFromX = useCallback((x: number) => {
    const w = widthRef.current;
    if (w <= 0) return min;
    return Math.max(min, Math.min(max, min + (x / w) * (max - min)));
  }, [min, max]);
  const fillPct = ((value - min) / (max - min)) * 100;
  return (
    <View
      style={sliderStyles.touch}
      onLayout={(e) => { widthRef.current = e.nativeEvent.layout.width; }}
      onStartShouldSetResponder={() => true}
      onMoveShouldSetResponder={() => true}
      onResponderGrant={(e) => onChange(valueFromX(e.nativeEvent.locationX))}
      onResponderMove={(e) => onChange(valueFromX(e.nativeEvent.locationX))}
      accessible={true}
      accessibilityRole="adjustable"
      accessibilityLabel={accessibilityLabel}
      accessibilityValue={{ min: Math.round(min * 100), max: Math.round(max * 100), now: Math.round(value * 100) }}
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

// Copie du SettingsPanel web : menu principal (Qualité en liste directe, puis
// lignes Audio / Sous-titres / Vitesse ouvrant un sous-menu, et la luminosité
// en bas), sous-menus avec entête retour. S'ancre AU-DESSUS de l'engrenage,
// aligné à droite, comme la popup web.
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
  style,
}: PlayerSettingsPanelProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { height: screenH } = useWindowDimensions();
  const QUALITY_OPTIONS = useQualityOptions();
  const [menu, setMenu] = useState<"main" | "audio" | "subtitles" | "speed">("main");

  const select = (fn: () => void) => {
    fn();
    onClose();
  };

  const currentAudioLabel =
    audioStreams.find((s) => s.index === currentAudioIndex)?.title
    ?? audioStreams.find((s) => s.index === currentAudioIndex)?.language
    ?? (audioStreams.length > 0 ? t("media.track", { number: currentAudioIndex }) : "-");

  const currentSubLabel = currentSubtitleIndex === null
    ? t("player.settings.off")
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
      style={[
        styles.panel,
        { backgroundColor: theme.colors.surface, borderColor: theme.colors.border },
        style,
      ]}
    >
      {/* La contrainte de hauteur vit sur le ScrollView, pas sur le panneau :
          un maxHeight sur le parent clippe le contenu sans borner la viewport
          de scroll (le ScrollView croit avoir la hauteur du contenu et ne
          défile jamais). */}
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
            {subtitleStreams.length > 0 && (
              <>
                <Divider />
                <NavRow label={t("player.settings.subtitles")} value={currentSubLabel} onPress={() => setMenu("subtitles")} />
              </>
            )}
            <Divider />
            <NavRow label={t("player.settings.speed")} value={currentSpeedLabel} onPress={() => setMenu("speed")} />
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
              />
            </View>
          </>
        )}

        {menu === "audio" && (
          <>
            <SubMenuHeader label={t("player.settings.audio")} />
            {audioStreams.map((s) => {
              const label = s.title ?? s.language ?? t("media.track", { number: s.index });
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
              const label = s.title ?? s.language ?? t("media.track", { number: s.index });
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
      </ScrollView>
    </View>
  );
}

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
