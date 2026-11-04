import type { ReactNode } from "react";
import { View, Text, Modal, Pressable, ScrollView, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import {
  SUBTITLE_TEXT_COLORS,
  SUBTITLE_BG_COLORS,
  type SubtitleStyle,
} from "../lib/subtitleStyle";

// Subtitle appearance, with a remote in hand. The phone offers the same
// choices behind sliders and swatches; here every value is a row the D-pad
// can land on - the owner's television had no way to change a colour, a size
// or a side (2026-09-19).
//
// Sizes and opacities are the discrete steps a remote can actually walk, not
// the phone's continuous sliders: the stored field keeps the same percent
// scale so a value set on either screen reads correctly on both.
const SIZES = [75, 100, 125, 150, 200] as const;
// A language-neutral cue, so the preview needs no translation of its own.
const SAMPLE_CUE = "Aa Bb Cc";

export interface SubtitleStylePickerProps {
  visible: boolean;
  style: SubtitleStyle;
  onChange: (style: SubtitleStyle) => void;
  onClose: () => void;
}

export function SubtitleStylePicker({ visible, style, onChange, onClose }: SubtitleStylePickerProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  if (!visible) return null;

  // One choice in a row of choices: the D-pad walks a row with left/right and
  // moves between rows with up/down. A swatch, when given, shows the colour
  // itself next to its name.
  const chip = (key: string, label: string, selected: boolean, onPress: () => void, isFirst: boolean, swatch?: string) => (
    <FocusableCard
      key={key}
      hasTVPreferredFocus={isFirst}
      onPress={onPress}
      accessibilityLabel={label}
      accessibilityState={{ selected }}
      style={[
        styles.chip,
        {
          borderColor: selected ? theme.colors.accent : theme.colors.border,
          backgroundColor: selected ? theme.colors.accent + "22" : "transparent",
          borderRadius: theme.radii.md,
        },
      ]}
      focusedStyle={{ backgroundColor: theme.colors.accent + "44" }}
    >
      {swatch !== undefined && (
        <View style={[styles.swatch, { backgroundColor: swatch, borderColor: theme.colors.border }]} />
      )}
      <Text style={[styles.chipLabel, { color: selected ? theme.colors.accent : theme.colors.text }]} numberOfLines={1}>
        {label}
      </Text>
    </FocusableCard>
  );

  const section = (title: string, chips: ReactNode) => (
    <View style={styles.sectionBlock}>
      <Text style={[styles.section, { color: theme.colors.textMuted }]}>{title}</Text>
      <View style={styles.chipRow}>{chips}</View>
    </View>
  );

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <View style={[styles.scrim, { backgroundColor: theme.colors.bg + "E6" }]}>
        <View style={[styles.panel, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg ?? theme.radii.md }]}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.settings.subtitleStyle")}
          </Text>

          {/* The cue as it will look, above the choices: on a television the
              preview is the only way to judge a colour without leaving. */}
          <View style={styles.previewBox}>
            <Text
              style={{
                color: style.color,
                backgroundColor: style.bgOpacity === 0 ? "transparent" : style.bgColor,
                fontSize: 24 * (style.fontSize / 100),
                paddingHorizontal: 12,
                paddingVertical: 6,
                borderRadius: 6,
                overflow: "hidden",
              }}
            >
              {SAMPLE_CUE}
            </Text>
          </View>

          {/* One row per setting, every choice of a row in sight: a long
              column hid everything below the first colours, and nothing on
              screen said there was more further down. */}
          <ScrollView style={styles.list}>
            {section(
              t("player.subtitleStyle.textColor"),
              SUBTITLE_TEXT_COLORS.map((c, i) =>
                chip(`c-${c.value}`, t(c.labelKey, c.value), style.color === c.value, () => onChange({ ...style, color: c.value }), i === 0, c.value),
              ),
            )}
            {section(
              t("player.subtitleStyle.background"),
              SUBTITLE_BG_COLORS.map((c) =>
                chip(
                  `b-${c.labelKey}`,
                  t(c.labelKey, c.value),
                  c.opacity === 0 ? style.bgOpacity === 0 : style.bgColor === c.value && style.bgOpacity > 0,
                  () => onChange({ ...style, bgColor: c.value, bgOpacity: c.opacity === 0 ? 0 : style.bgOpacity || 75 }),
                  false,
                  c.opacity === 0 ? "transparent" : c.value,
                ),
              ),
            )}
            {section(
              t("player.subtitleStyle.size"),
              SIZES.map((sz) => chip(`s-${sz}`, `${sz} %`, style.fontSize === sz, () => onChange({ ...style, fontSize: sz }), false)),
            )}
            {section(
              t("player.subtitleStyle.position"),
              (["bottom", "top"] as const).map((pos) =>
                chip(
                  `p-${pos}`,
                  t(pos === "bottom" ? "player.subtitleStyle.bottom" : "player.subtitleStyle.top"),
                  style.position === pos,
                  () => onChange({ ...style, position: pos }),
                  false,
                ),
              ),
            )}
          </ScrollView>

          <Pressable onPress={onClose} accessibilityLabel={t("common.close", "Close")} style={styles.close}>
            <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("common.close", "Close")}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  scrim: { flex: 1, justifyContent: "center", alignItems: "center" },
  panel: { width: 960, maxHeight: "90%", paddingVertical: 16, paddingHorizontal: 28 },
  title: { fontSize: 26, fontWeight: "700", textAlign: "center", marginBottom: 16 },
  previewBox: { alignItems: "center", marginBottom: 8 },
  list: { maxHeight: 600 },
  sectionBlock: { marginTop: 12 },
  section: { fontSize: 15, fontWeight: "700", letterSpacing: 1, marginBottom: 8 },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 12 },
  chip: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 8, paddingHorizontal: 16, borderWidth: 2 },
  swatch: { width: 18, height: 18, borderRadius: 9, borderWidth: 1 },
  chipLabel: { fontSize: 18 },
  close: { alignSelf: "center", paddingVertical: 10, paddingHorizontal: 32 },
});
