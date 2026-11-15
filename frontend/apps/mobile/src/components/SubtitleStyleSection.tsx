import { View, Text, Pressable, Switch, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import {
  DEFAULT_SUBTITLE_STYLE,
  SUBTITLE_BG_COLORS,
  SUBTITLE_STYLE_PREF_KEY,
  SUBTITLE_TEXT_COLORS,
  parseSubtitleStylePref,
  subtitleBgRgba,
  type SubtitleStyle,
} from "@evey/client";
import { useTheme } from "../providers/ThemeProvider";

// The sizes a tap can pick; the player's own slider covers the same range.
const SIZES = [75, 100, 125, 150, 200] as const;

interface Props {
  /** The running profile's preferences. */
  preferences: Record<string, string> | undefined;
  /** Saves a preference patch on the profile; the screen owns the request. */
  onSave: (patch: Record<string, string>) => void;
  disabled?: boolean;
}

/**
 * The subtitle appearance as a profile setting: the same preference the
 * player's own menu writes, so it can be set without starting a title and
 * follows the viewer to every device.
 */
export function SubtitleStyleSection({ preferences, onSave, disabled }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const style = parseSubtitleStylePref(preferences) ?? DEFAULT_SUBTITLE_STYLE;
  const save = (patch: Partial<SubtitleStyle>) =>
    onSave({ [SUBTITLE_STYLE_PREF_KEY]: JSON.stringify({ ...style, ...patch }) });

  const title = (text: string) => (
    <Text style={[styles.groupTitle, { color: theme.colors.textMuted }]}>{text}</Text>
  );
  const chip = (key: string, label: string, selected: boolean, onPress: () => void, swatch?: string) => (
    <Pressable
      key={key}
      onPress={onPress}
      disabled={disabled}
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      accessibilityLabel={label}
      style={[
        styles.chip,
        {
          borderRadius: theme.radii.md,
          borderColor: selected ? theme.colors.accent : theme.colors.border,
          backgroundColor: theme.colors.surfaceRaised,
        },
      ]}
    >
      {swatch ? <View style={[styles.swatch, { backgroundColor: swatch, borderColor: theme.colors.border }]} /> : null}
      <Text style={{ color: theme.colors.text, fontSize: 14 }}>{label}</Text>
    </Pressable>
  );

  return (
    <View style={[styles.box, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: theme.spacing.md }]}>
      {/* A stand-in for a video frame: dark whatever the theme. */}
      <View style={[styles.preview, { borderRadius: theme.radii.md, justifyContent: style.position === "top" ? "flex-start" : "flex-end" }]}>
        <Text
          testID="subtitle-style-preview"
          style={{
            color: style.color,
            fontSize: 16 * (style.fontSize / 100),
            backgroundColor: subtitleBgRgba(style.bgColor, style.bgOpacity),
            paddingHorizontal: 8,
            paddingVertical: 2,
            borderRadius: 4,
            overflow: "hidden",
            textAlign: "center",
            ...(style.shadow ? { textShadowColor: "#000", textShadowRadius: 4, textShadowOffset: { width: 0, height: 1 } } : null),
          }}
        >
          {t("auth.settings.playback.subtitleStylePreview", "This is how subtitles will look.")}
        </Text>
      </View>

      {title(t("player.subtitleStyle.textColor", "Text Color"))}
      <View style={styles.chips}>
        {SUBTITLE_TEXT_COLORS.map((c) => chip(`c-${c.value}`, t(c.labelKey), style.color === c.value, () => save({ color: c.value }), c.value))}
      </View>

      {title(t("player.subtitleStyle.background", "Background"))}
      <View style={styles.chips}>
        {SUBTITLE_BG_COLORS.map((c) =>
          chip(
            `b-${c.labelKey}`,
            t(c.labelKey),
            c.opacity === 0 ? style.bgOpacity === 0 : style.bgColor === c.value && style.bgOpacity > 0,
            () => save({ bgColor: c.value, bgOpacity: c.opacity === 0 ? 0 : style.bgOpacity || 75 }),
            c.opacity === 0 ? "transparent" : c.value,
          ),
        )}
      </View>

      {title(t("player.subtitleStyle.size", "Size"))}
      <View style={styles.chips}>
        {SIZES.map((size) => chip(`s-${size}`, `${size} %`, style.fontSize === size, () => save({ fontSize: size })))}
      </View>

      {title(t("player.subtitleStyle.position", "Position"))}
      <View style={styles.chips}>
        {(["bottom", "top"] as const).map((pos) =>
          chip(`p-${pos}`, t(`player.subtitleStyle.${pos}`), style.position === pos, () => save({ position: pos })),
        )}
      </View>

      <View style={styles.shadowRow}>
        <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{t("player.subtitleStyle.textShadow", "Text shadow")}</Text>
        <Switch
          value={style.shadow}
          onValueChange={(v) => save({ shadow: v })}
          disabled={disabled}
          accessibilityLabel={t("player.subtitleStyle.textShadow", "Text shadow")}
          trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  box: { padding: 12, gap: 8 },
  preview: { minHeight: 96, padding: 12, alignItems: "center", backgroundColor: "#171717" },
  groupTitle: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 6 },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: { flexDirection: "row", alignItems: "center", gap: 6, paddingVertical: 8, paddingHorizontal: 12, borderWidth: 2 },
  swatch: { width: 14, height: 14, borderRadius: 7, borderWidth: 1 },
  shadowRow: { flexDirection: "row", alignItems: "center", marginTop: 6 },
});
