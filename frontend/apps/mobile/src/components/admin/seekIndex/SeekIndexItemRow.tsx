import { View, Text, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import type { V1SeekIndexItem } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { audioDelayed, formatBytes } from "@evey/api";

type Theme = ReturnType<typeof useTheme>;

// One badge that can carry a longer explanation. RN has no hover tooltip, so
// pressing the badge surfaces the same text the web title="..." attribute
// shows on hover.
function HintBadge({ label, hint, color }: { label: string; hint?: string; color: string }) {
  const badge = (
    <View style={[styles.badge, { backgroundColor: color + "22" }]}>
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

export function SeekIndexItemRow({
  item,
  theme,
  busy,
  onRepair,
  onRepairResync,
  onRollback,
  onRemuxAudio,
  onDeleteBackup,
  repairPending,
  repairResyncPending,
  rollbackPending,
  remuxAudioPending,
  deleteBackupPending,
}: {
  item: V1SeekIndexItem;
  theme: Theme;
  busy: boolean;
  onRepair: (mediaId: string) => void;
  onRepairResync: (mediaId: string) => void;
  onRollback: (mediaId: string) => void;
  onRemuxAudio: (mediaId: string) => void;
  onDeleteBackup: (mediaId: string) => void;
  repairPending: boolean;
  repairResyncPending: boolean;
  rollbackPending: boolean;
  remuxAudioPending: boolean;
  deleteBackupPending: boolean;
}) {
  const { t } = useTranslation();
  const delayed = audioDelayed(item);
  const damagedBytes = Number(item.damagedBytes ?? 0);
  const missingTailBytes = Number(item.missingTailBytes ?? 0);
  const audioShortfallSec = item.audioShortfallSec ?? 0;
  // No repair exists for a truncated file (the tail is simply absent) nor for
  // a coarse one (its index was already rebuilt and cannot beat the encode's
  // keyframes) - offering one would promise a fix nothing can deliver.
  const canRepair = !item.hasIndex && !item.truncated && !item.seekCoarse;

  return (
    <View style={[stylesCard.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
        {item.title || item.path}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={1}>
        {item.path}
      </Text>
      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: 6 }}>
        <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{(item.container ?? "").toUpperCase()}</Text>
        </View>
        <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{formatBytes(Number(item.sizeBytes ?? 0))}</Text>
        </View>

        {item.hasIndex ? (
          <HintBadge label={t("admin.seekIndex.statusIndexed")} color={theme.colors.success} />
        ) : item.seekCoarse ? (
          <HintBadge
            label={t("admin.seekIndex.seekCoarseBadge")}
            hint={t("admin.seekIndex.seekCoarseHint")}
            color={theme.colors.info}
          />
        ) : (
          <HintBadge label={t("admin.seekIndex.statusMissing")} color={theme.colors.warning} />
        )}

        {item.hasBackup && (
          <HintBadge label={t("admin.seekIndex.backupBadge")} color={theme.colors.info} />
        )}

        {delayed && (
          <HintBadge
            label={t("admin.seekIndex.audioDelayBadge", { sec: (item.audioDelaySec ?? 0).toFixed(1) })}
            color={theme.colors.warning}
          />
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
              color={theme.colors.warning}
            />
          )
        )}

        {item.wrongContainer && (
          <HintBadge
            label={t("admin.seekIndex.wrongContainerBadge")}
            hint={t("admin.seekIndex.wrongContainerHint")}
            color={theme.colors.error}
          />
        )}

        {item.trailingJunk && (
          <HintBadge
            label={t("admin.seekIndex.trailingJunkBadge")}
            hint={t("admin.seekIndex.trailingJunkHint")}
            color={theme.colors.info}
          />
        )}

        {audioShortfallSec > 0 && (
          <HintBadge
            label={t("admin.seekIndex.audioShortBadge", { sec: Math.round(audioShortfallSec) })}
            hint={t("admin.seekIndex.audioShortHint", { sec: Math.round(audioShortfallSec) })}
            color={theme.colors.warning}
          />
        )}
      </View>

      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 }}>
        {delayed && (
          <Pressable
            disabled={busy}
            onPress={() => item.mediaId && onRemuxAudio(item.mediaId)}
            style={[stylesCard.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}
          >
            {remuxAudioPending ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.remuxAudioBtn")}</Text>
            )}
          </Pressable>
        )}
        {canRepair && (
          <Pressable
            disabled={busy}
            onPress={() => item.mediaId && onRepair(item.mediaId)}
            onLongPress={() => Alert.alert(t("admin.seekIndex.repair"), t("admin.seekIndex.repairHint"))}
            style={[stylesCard.smallBtn, { backgroundColor: theme.colors.accent, opacity: busy ? 0.6 : 1 }]}
          >
            {repairPending ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.repair")}</Text>
            )}
          </Pressable>
        )}
        {canRepair && (
          <Pressable
            disabled={busy}
            onPress={() => item.mediaId && onRepairResync(item.mediaId)}
            onLongPress={() => Alert.alert(t("admin.seekIndex.repairResync"), t("admin.seekIndex.repairResyncHint"))}
            style={[stylesCard.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}
          >
            {repairResyncPending ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.repairResync")}</Text>
            )}
          </Pressable>
        )}
        {item.hasBackup && (
          <Pressable
            disabled={busy}
            onPress={() => item.mediaId && onRollback(item.mediaId)}
            style={[stylesCard.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}
          >
            {rollbackPending ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.rollback")}</Text>
            )}
          </Pressable>
        )}
        {item.hasBackup && (
          <Pressable
            disabled={deleteBackupPending}
            onPress={() => item.mediaId && onDeleteBackup(item.mediaId)}
            style={[stylesCard.smallBtn, { backgroundColor: theme.colors.bg, opacity: deleteBackupPending ? 0.6 : 1 }]}
          >
            {deleteBackupPending ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.deleteBackup")}</Text>
            )}
          </Pressable>
        )}
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
