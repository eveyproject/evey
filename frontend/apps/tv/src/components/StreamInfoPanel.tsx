import { Text, Modal, Pressable, View, ScrollView, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import type { V1StreamInfo } from "@evey/api";
import { buildStreamInfoModel, decisionBadgeKey, type ServedVariant } from "@evey/client";
import { FONT_SIZE } from "@evey/theme";

interface StreamInfoPanelProps {
  visible: boolean;
  onClose: () => void;
  streamInfo: V1StreamInfo | null;
  /** ABSOLUTE index of the selected audio stream, so the panel describes IT. */
  audioStreamIndex?: number | null;
  /** The variant playing right now: the panel follows every quality change. */
  served?: ServedVariant | null;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  const theme = useTheme();
  return (
    <View style={[styles.row, { borderBottomColor: theme.colors.border }]}>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>{label}</Text>
      <Text style={[styles.value, { color: theme.colors.text }]}>{value}</Text>
    </View>
  );
}

// The decision belongs to its stream: colour-coded so copied (untouched, good) is
// visually distinct from transcoded (re-encoded, costly) and wasm (decoded
// client-side, so not transcoded at all) - mirrors web/mobile's badge.
function DecisionBadge({ decision }: { decision?: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  if (!decision) return null;
  const isCopy = decision === "copy" || decision === "direct";
  const isWasm = decision === "wasm";
  // bg switches between accent/success/warning, so the label colour must
  // follow: onAccent painted on a warning fill (or vice versa) is the exact
  // WCAG AA regression this badge already caused once before.
  const bg = isWasm ? theme.colors.accent : isCopy ? theme.colors.success : theme.colors.warning;
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

export function StreamInfoPanel({ visible, onClose, streamInfo, audioStreamIndex, served }: StreamInfoPanelProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const model = buildStreamInfoModel(streamInfo, { audio: { absolute: audioStreamIndex }, served });

  // A hidden RN Modal keeps its children mounted, so this picker re-rendered
  // on every ~2/s currentTime tick of the player while closed. Render nothing
  // until opened.
  if (!visible) return null;

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} focusable={false}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface }]}
          onPress={(e) => e.stopPropagation()}
          focusable={false}
        >
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.streamInfo.title", "Stream Info")}
          </Text>
          {model ? (
            <ScrollView style={styles.body}>
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
            </ScrollView>
          ) : null}

          <FocusableCard
            hasTVPreferredFocus={true}
            onPress={onClose}
            accessibilityLabel={t("common.close", "Close")}
            style={[styles.closeRow, { borderColor: theme.colors.border }]}
          >
            <Text style={[styles.closeLabel, { color: theme.colors.accent }]}>{t("common.close", "Close")}</Text>
          </FocusableCard>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.7)",
  },
  sheet: {
    borderRadius: 16,
    padding: 24,
    minWidth: 420,
    maxWidth: 480,
    maxHeight: 600,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
    textAlign: "center",
    marginBottom: 12,
  },
  body: {
    marginBottom: 12,
  },
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
  closeRow: {
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingVertical: 9,
    alignItems: "center",
    borderRadius: 8,
  },
  closeLabel: {
    fontSize: FONT_SIZE.base,
    fontWeight: "600",
  },
});
