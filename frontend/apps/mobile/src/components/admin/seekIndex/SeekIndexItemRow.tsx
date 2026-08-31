import { View, Text, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import type { V1SeekIndexItem } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { audioDelayed, formatBytes, isAckable, isAcked, verdictOf } from "@evey/api";

type Theme = ReturnType<typeof useTheme>;

// One badge that can carry a longer explanation. RN has no hover tooltip, so
// pressing the badge surfaces the same text the web title="..." attribute
// shows on hover.
function HintBadge({ label, hint, color, testID }: { label: string; hint?: string; color: string; testID?: string }) {
  const badge = (
    <View style={[styles.badge, { backgroundColor: color + "22" }]} testID={testID}>
      <Text style={{ color, fontSize: 10, fontWeight: "600" }}>{label}</Text>
    </View>
  );
  if (!hint) return badge;
  return (
    <Pressable onPress={() => Alert.alert(label, hint)} accessibilityRole="button" accessibilityLabel={hint}>
      {badge}
    </Pressable>
  );
}

const VERDICT_KEYS = {
  healthy: "admin.seekIndex.verdictHealthy",
  repairable: "admin.seekIndex.verdictRepairable",
  manual: "admin.seekIndex.verdictManual",
  informational: "admin.seekIndex.verdictInformational",
} as const;

export function SeekIndexItemRow({
  item,
  theme,
  busy,
  containerFixAvailable,
  onRepair,
  onRepairResync,
  onRollback,
  onRemuxAudio,
  onFixContainer,
  onAck,
  onDeleteBackup,
  repairPending,
  repairResyncPending,
  rollbackPending,
  remuxAudioPending,
  fixContainerPending,
  ackPending,
  deleteBackupPending,
}: {
  item: V1SeekIndexItem;
  theme: Theme;
  busy: boolean;
  containerFixAvailable: boolean;
  onRepair: (mediaId: string) => void;
  onRepairResync: (mediaId: string) => void;
  onRollback: (mediaId: string) => void;
  onRemuxAudio: (mediaId: string) => void;
  onFixContainer: (mediaId: string) => void;
  onAck: (mediaId: string, acked: boolean) => void;
  onDeleteBackup: (mediaId: string) => void;
  repairPending: boolean;
  repairResyncPending: boolean;
  rollbackPending: boolean;
  remuxAudioPending: boolean;
  fixContainerPending: boolean;
  ackPending: boolean;
  deleteBackupPending: boolean;
}) {
  const { t } = useTranslation();
  const delayed = audioDelayed(item);
  const damagedBytes = Number(item.damagedBytes ?? 0);
  const missingTailBytes = Number(item.missingTailBytes ?? 0);
  const audioShortfallSec = item.audioShortfallSec ?? 0;
  const verdict = verdictOf(item);
  const acked = isAcked(item);
  // Same rules as the web table: the normal repair for a missing index, the
  // lossy resync for a damaged stretch (index fine or not), no repair at all
  // for a truncated or coarse file - the operator acknowledges those instead.
  const indexRepairable = !item.hasIndex && !item.truncated && !item.seekCoarse;
  const damagedOnly = Boolean(item.hasIndex) && damagedBytes > 0 && !item.truncated;
  const verdictColor =
    verdict === "healthy" ? theme.colors.success : verdict === "repairable" ? theme.colors.warning : verdict === "manual" ? theme.colors.error : theme.colors.info;

  const btn = (label: string, onPress: () => void, pending: boolean, accent = false, hint?: string, disabled = busy) => (
    <Pressable
      disabled={disabled}
      onPress={onPress}
      onLongPress={hint ? () => Alert.alert(label, hint) : undefined}
      style={[stylesCard.smallBtn, { backgroundColor: accent ? theme.colors.accent : theme.colors.bg, opacity: disabled ? 0.6 : 1 }]}
    >
      {pending ? (
        <ActivityIndicator color={accent ? theme.colors.onAccent : theme.colors.text} size="small" />
      ) : (
        <Text style={{ color: accent ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>{label}</Text>
      )}
    </Pressable>
  );

  return (
    <View style={[stylesCard.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
        {item.title || item.path}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={1}>
        {item.path}
      </Text>
      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: 6 }}>
        {/* The verdict first - the one answer - then the defects behind it. */}
        <HintBadge label={t(VERDICT_KEYS[verdict])} color={verdictColor} testID="seek-index-verdict" />
        {acked && <HintBadge label={t("admin.seekIndex.ackedBadge")} color={theme.colors.textMuted} />}
        <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{(item.container ?? "").toUpperCase()}</Text>
        </View>
        <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{formatBytes(Number(item.sizeBytes ?? 0))}</Text>
        </View>

        {!item.hasIndex && !item.seekCoarse && !item.truncated && !item.wrongContainer && (
          <HintBadge label={t("admin.seekIndex.statusMissing")} color={theme.colors.warning} />
        )}
        {item.seekCoarse && !item.truncated && (
          <HintBadge
            label={t("admin.seekIndex.seekCoarseBadge")}
            hint={t("admin.seekIndex.seekCoarseHint", { detail: item.seekCoarseDetail || t("admin.seekIndex.seekCoarseNoDetail") })}
            color={theme.colors.info}
          />
        )}
        {item.hasBackup && <HintBadge label={t("admin.seekIndex.backupBadge")} color={theme.colors.info} />}
        {delayed && (
          <HintBadge label={t("admin.seekIndex.audioDelayBadge", { sec: (item.audioDelaySec ?? 0).toFixed(1) })} color={theme.colors.warning} />
        )}
        {item.truncated ? (
          <HintBadge
            label={
              missingTailBytes > 0
                ? t("admin.seekIndex.truncatedSizeBadge", { size: formatBytes(missingTailBytes) })
                : t("admin.seekIndex.truncatedBadge")
            }
            hint={t("admin.seekIndex.truncatedHint")}
            color={theme.colors.error}
          />
        ) : (
          damagedBytes > 0 && (
            <HintBadge
              label={t("admin.seekIndex.damagedBadge", { size: formatBytes(damagedBytes) })}
              hint={t("admin.seekIndex.repairResyncHint")}
              color={theme.colors.warning}
            />
          )
        )}
        {item.wrongContainer && (
          <HintBadge label={t("admin.seekIndex.wrongContainerBadge")} hint={t("admin.seekIndex.wrongContainerHint")} color={theme.colors.error} />
        )}
        {item.trailingJunk && (
          <HintBadge label={t("admin.seekIndex.trailingJunkBadge")} hint={t("admin.seekIndex.trailingJunkHint")} color={theme.colors.info} />
        )}
        {audioShortfallSec > 0 && (
          <HintBadge
            label={t("admin.seekIndex.audioShortBadge", { sec: Math.round(audioShortfallSec) })}
            hint={t("admin.seekIndex.audioShortHint", { sec: Math.round(audioShortfallSec) })}
            color={theme.colors.info}
          />
        )}
      </View>

      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 }}>
        {delayed && btn(t("admin.seekIndex.remuxAudioBtn"), () => item.mediaId && onRemuxAudio(item.mediaId), remuxAudioPending)}
        {indexRepairable && !item.wrongContainer &&
          btn(t("admin.seekIndex.repair"), () => item.mediaId && onRepair(item.mediaId), repairPending, true, t("admin.seekIndex.repairHint"))}
        {(indexRepairable || damagedOnly) && !item.wrongContainer &&
          btn(t("admin.seekIndex.repairResync"), () => item.mediaId && onRepairResync(item.mediaId), repairResyncPending, false, t("admin.seekIndex.repairResyncHint"))}
        {item.wrongContainer && containerFixAvailable &&
          btn(t("admin.seekIndex.fixContainerBtn"), () => item.mediaId && onFixContainer(item.mediaId), fixContainerPending, true, t("admin.seekIndex.fixContainerHint"))}
        {isAckable(item) &&
          btn(acked ? t("admin.seekIndex.unackBtn") : t("admin.seekIndex.ackBtn"), () => item.mediaId && onAck(item.mediaId, !acked), ackPending, false, undefined, ackPending)}
        {item.hasBackup && btn(t("admin.seekIndex.rollback"), () => item.mediaId && onRollback(item.mediaId), rollbackPending)}
        {item.hasBackup &&
          btn(t("admin.seekIndex.deleteBackup"), () => item.mediaId && onDeleteBackup(item.mediaId), deleteBackupPending, false, undefined, deleteBackupPending)}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4 },
});

const stylesCard = StyleSheet.create({
  card: { padding: 12 },
  smallBtn: { paddingHorizontal: 14, paddingVertical: 8, borderRadius: 8, alignItems: "center", justifyContent: "center", minWidth: 70 },
});
