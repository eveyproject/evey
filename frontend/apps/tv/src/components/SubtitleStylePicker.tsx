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

  const row = (key: string, label: string, value: string, selected: boolean, onPress: () => void, isFirst: boolean) => (
    <FocusableCard
      key={key}
      hasTVPreferredFocus={isFirst}
      onPress={onPress}
      accessibilityLabel={label}
      accessibilityState={{ selected }}
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
      focusedStyle={{ backgroundColor: theme.colors.accent + "33" }}
    >
      <Text style={[styles.rowLabel, { color: theme.colors.text }]}>{label}</Text>
      <Text style={{ color: selected ? theme.colors.accent : theme.colors.textMuted, fontSize: 18 }}>
        {selected ? `✓ ${value}` : value}
      </Text>
    </FocusableCard>
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

          <ScrollView style={styles.list}>
            <Text style={[styles.section, { color: theme.colors.textMuted }]}>{t("player.subtitleStyle.textColor")}</Text>
            {SUBTITLE_TEXT_COLORS.map((c, i) =>
              row(`c-${c.value}`, t(c.labelKey, c.value), "", style.color === c.value, () => onChange({ ...style, color: c.value }), i === 0),
            )}

            <Text style={[styles.section, { color: theme.colors.textMuted }]}>{t("player.subtitleStyle.size")}</Text>
            {SIZES.map((s) =>
              row(`s-${s}`, `${s} %`, "", style.fontSize === s, () => onChange({ ...style, fontSize: s }), false),
            )}

            <Text style={[styles.section, { color: theme.colors.textMuted }]}>{t("player.subtitleStyle.background")}</Text>
            {SUBTITLE_BG_COLORS.map((c) =>
              row(
                `b-${c.labelKey}`,
                t(c.labelKey, c.value),
                "",
                c.opacity === 0 ? style.bgOpacity === 0 : style.bgColor === c.value && style.bgOpacity > 0,
                () => onChange({ ...style, bgColor: c.value, bgOpacity: c.opacity === 0 ? 0 : style.bgOpacity || 75 }),
                false,
              ),
            )}

            <Text style={[styles.section, { color: theme.colors.textMuted }]}>{t("player.subtitleStyle.position")}</Text>
            {(["bottom", "top"] as const).map((pos) =>
              row(
                `p-${pos}`,
                t(pos === "bottom" ? "player.subtitleStyle.bottom" : "player.subtitleStyle.top"),
                "",
                style.position === pos,
                () => onChange({ ...style, position: pos }),
                false,
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
  panel: { width: 760, maxHeight: "86%", paddingVertical: 16, paddingHorizontal: 28 },
  title: { fontSize: 26, fontWeight: "700", textAlign: "center", marginBottom: 16 },
  previewBox: { alignItems: "center", marginBottom: 16 },
  list: { maxHeight: 560 },
  section: { fontSize: 15, fontWeight: "700", letterSpacing: 1, marginTop: 16, marginBottom: 4 },
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingVertical: 10, paddingHorizontal: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  rowLabel: { fontSize: 18 },
  close: { alignSelf: "center", paddingVertical: 10, paddingHorizontal: 32 },
});
