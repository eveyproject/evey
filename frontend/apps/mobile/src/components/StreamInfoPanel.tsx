import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { BottomSheet } from "./BottomSheet";
import type { V1StreamInfo } from "@evey/api";
import { buildStreamInfoModel, decisionBadgeKey } from "@evey/client";
import { FONT_SIZE } from "@evey/theme";

interface StreamInfoPanelProps {
  visible: boolean;
  onClose: () => void;
  streamInfo: V1StreamInfo | null;
  /** ABSOLUTE index of the selected audio stream, so the panel describes IT. */
  audioStreamIndex?: number | null;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  const theme = useTheme();
  return (
    <View style={[styles.row, { borderBottomColor: theme.colors.divider }]}>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.value, { color: theme.colors.text }]}>{value}</Text>
    </View>
  );
}

// The decision belongs to its stream, not to a lonely row at the bottom of
// the section: colour-coded so copied (untouched, good) is visually distinct
// from transcoded (re-encoded, costly) and wasm (decoded client-side, not
// transcoded at all).
function DecisionBadge({ decision }: { decision?: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  if (!decision) return null;
  const isCopy = decision === "copy" || decision === "direct";
  const isWasm = decision === "wasm";
  const bg = isWasm ? theme.colors.accent : isCopy ? theme.colors.success : theme.colors.warning;
  // The label follows the fill: bg painted on a warning fill is the WCAG AA
  // failure the television's badge was already fixed for.
  const onBg = isWasm ? theme.colors.onAccent : isCopy ? theme.colors.onSuccess : theme.colors.onWarning;
  const badge = decisionBadgeKey(decision);
  const label = t(badge.key, badge.defaultValue);
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <Text style={[styles.badgeText, { color: onBg }]}>{label}</Text>
    </View>
  );
}

function SectionHeader({ title, decision }: { title: string; decision?: string }) {
  const theme = useTheme();
  return (
    <View style={styles.sectionHeader}>
      <Text style={[styles.section, { color: theme.colors.accent }]}>{title}</Text>
      <DecisionBadge decision={decision} />
    </View>
  );
}

export function StreamInfoPanel({ visible, onClose, streamInfo, audioStreamIndex }: StreamInfoPanelProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const model = buildStreamInfoModel(streamInfo, { audio: { absolute: audioStreamIndex } });
  if (!model) return null;

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.streamInfo.title", "Stream Info")}>
      {model.sections.map((section) => (
        <View key={section.kind}>
          <SectionHeader title={t(section.titleKey, section.title)} decision={section.decision} />
          {section.rows.map((row) => (
            <InfoRow
              key={row.labelKey}
              label={t(row.labelKey, row.label)}
              value={row.valueKey ? t(row.valueKey) : (row.value ?? "")}
            />
          ))}
        </View>
      ))}

      {model.reasons.length > 0 && (
        <>
          <SectionHeader title={t(model.reasonsHeadingKey)} />
          {model.reasons.map((reason) => (
            <View key={reason} style={styles.reasonRow}>
              <Text style={[styles.reasonBullet, { color: theme.colors.warning }]}>{"•"}</Text>
              <Text style={[styles.reasons, { color: theme.colors.textMuted }]}>
                {t(`player.streamInfo.reason.${reason}`, reason.replace(/_/g, " "))}
              </Text>
            </View>
          ))}
        </>
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  section: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  sectionHeader: {
    flexDirection: "row",
    alignItems: "center",
    marginTop: 16,
    marginBottom: 6,
  },
  badge: {
    marginLeft: 8,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeText: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "700",
    textTransform: "uppercase",
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: {
    fontSize: FONT_SIZE.sm,
  },
  value: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "500",
    textAlign: "right",
    flexShrink: 1,
    marginLeft: 12,
  },
  reasonRow: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 6,
    paddingVertical: 4,
  },
  reasonBullet: {
    fontSize: FONT_SIZE.xs,
  },
  reasons: {
    fontSize: FONT_SIZE.xs,
    flexShrink: 1,
  },
});
