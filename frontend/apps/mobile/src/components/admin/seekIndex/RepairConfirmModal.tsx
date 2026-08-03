import { View, Text, Pressable, ActivityIndicator, Modal, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import type { V1SeekIndexVolume } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { formatSize, volumeBackupsRisky, volumeTempRisky, BACKUP_WINDOW } from "../seekIndexHelpers";

type Theme = ReturnType<typeof useTheme>;
export type BatchKind = "repair" | "rollback" | "purge";

function CheckRow({
  label,
  hint,
  checked,
  onToggle,
  theme,
}: {
  label: string;
  hint: string;
  checked: boolean;
  onToggle: () => void;
  theme: Theme;
}) {
  return (
    <Pressable
      onPress={onToggle}
      style={{ marginBottom: 10 }}
      accessibilityRole="checkbox"
      accessibilityState={{ checked }}
    >
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <Ionicons name={checked ? "checkbox" : "square-outline"} size={20} color={checked ? theme.colors.accent : theme.colors.textMuted} />
        <Text style={{ color: theme.colors.text, fontSize: 14 }}>{label}</Text>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2, marginLeft: 28 }}>{hint}</Text>
    </Pressable>
  );
}

export function RepairConfirmModal({
  theme,
  batch,
  onClose,
  onConfirm,
  pending,
  repairableCount,
  withBackupCount,
  delayedAudioCount,
  volumes,
  keepBackups,
  setKeepBackups,
  alsoAudio,
  setAlsoAudio,
}: {
  theme: Theme;
  batch: BatchKind | null;
  onClose: () => void;
  onConfirm: () => void;
  pending: boolean;
  repairableCount: number;
  withBackupCount: number;
  delayedAudioCount: number;
  volumes: V1SeekIndexVolume[];
  keepBackups: boolean;
  setKeepBackups: (v: boolean) => void;
  alsoAudio: boolean;
  setAlsoAudio: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  if (!batch) return null;

  const title =
    batch === "rollback"
      ? t("admin.seekIndex.rollbackAllConfirmTitle")
      : batch === "purge"
        ? t("admin.seekIndex.purgeAllConfirmTitle")
        : t("admin.seekIndex.repairAllConfirmTitle");

  const body =
    batch === "rollback"
      ? t("admin.seekIndex.rollbackAllConfirm", { count: withBackupCount })
      : batch === "purge"
        ? t("admin.seekIndex.purgeAllConfirm", { count: withBackupCount })
        : t("admin.seekIndex.repairAllConfirm", { count: repairableCount });

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <Pressable style={StyleSheet.absoluteFillObject} onPress={onClose} />
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700", marginBottom: 10 }}>{title}</Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: 14 }}>{body}</Text>

          {batch === "repair" && volumes.length > 0 && (
            <View style={{ marginBottom: 14, gap: 4 }}>
              {volumes.map((v) => (
                <View key={v.root}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {t("admin.seekIndex.volumeLine", {
                      root: v.root,
                      free: formatSize(Number(v.freeBytes ?? 0)),
                      missing: formatSize(Number(v.missingBytes ?? 0)),
                      count: v.missingFiles ?? 0,
                    })}
                  </Text>
                  {volumeBackupsRisky(v) && (
                    <Text style={{ color: theme.colors.warning, fontSize: 11 }}>{t("admin.seekIndex.volumeBackupsRisky")}</Text>
                  )}
                  {volumeTempRisky(v) && (
                    <Text style={{ color: theme.colors.error, fontSize: 11 }}>
                      {t("admin.seekIndex.volumeTempRisky", { largest: formatSize(Number(v.largestMissingBytes ?? 0)) })}
                    </Text>
                  )}
                </View>
              ))}
            </View>
          )}

          {batch === "repair" && (
            <View style={{ marginBottom: 6 }}>
              {delayedAudioCount > 0 && (
                <CheckRow
                  label={t("admin.seekIndex.alsoAudio", { count: delayedAudioCount })}
                  hint={t("admin.seekIndex.alsoAudioHint")}
                  checked={alsoAudio}
                  onToggle={() => setAlsoAudio(!alsoAudio)}
                  theme={theme}
                />
              )}
              <CheckRow
                label={t("admin.seekIndex.keepBackups")}
                hint={keepBackups ? t("admin.seekIndex.keepBackupsHint") : t("admin.seekIndex.keepBackupsOffHint", { n: BACKUP_WINDOW })}
                checked={keepBackups}
                onToggle={() => setKeepBackups(!keepBackups)}
                theme={theme}
              />
            </View>
          )}

          <View style={{ flexDirection: "row", justifyContent: "flex-end", gap: 8, marginTop: 8 }}>
            <Pressable onPress={onClose} style={styles.btn}>
              <Text style={{ color: theme.colors.accent, fontSize: 14, fontWeight: "600" }}>{t("common.cancel")}</Text>
            </Pressable>
            <Pressable
              disabled={pending}
              onPress={onConfirm}
              style={[styles.btn, { backgroundColor: theme.colors.accent, opacity: pending ? 0.6 : 1 }]}
            >
              {pending ? (
                <ActivityIndicator color={theme.colors.onAccent} size="small" />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontSize: 14, fontWeight: "600" }}>{t("common.confirm")}</Text>
              )}
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { padding: 20, paddingBottom: 32 },
  btn: { paddingHorizontal: 16, paddingVertical: 10, borderRadius: 8, alignItems: "center", justifyContent: "center" },
});
