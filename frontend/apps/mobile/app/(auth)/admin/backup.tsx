import { View, Text, FlatList, Pressable, TextInput, Switch, Alert, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast, useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { formatBytes } from "@evey/api";
import * as DocumentPicker from "expo-document-picker";
import { downloadBackupFile, restoreExistingBackup, restoreFromFile } from "../../../src/components/admin/backup/backupFileOps";
import { isBackupRestorable } from "../../../src/components/admin/backup/backupTransfer";

interface Backup {
  id: string;
  createdAt: string;
  version: string;
  size: number;
}

interface BackupSchedule {
  cronExpression: string;
  enabled: boolean;
  retentionDays: number;
  lastRunAt?: string;
  nextRunAt?: string;
  lastStatus?: string;
  lastError?: string;
}

const BACKUPS_KEY = ["admin", "backups"];
const SCHEDULE_KEY = ["admin", "backup-schedule"];

export function isValidCron(expr: string): boolean {
  return expr.trim().split(/\s+/).length === 5;
}

function useBackups() {
  return useQuery({
    queryKey: BACKUPS_KEY,
    queryFn: () => eveyFetch<{ backups: Backup[] }>({ url: "/api/v1/admin/backups", method: "GET" }),
    select: (d) => d.backups ?? [],
  });
}

// ScheduleSection mirrors the web ScheduleSection (routes/_auth/admin/backup.lazy.tsx):
// cron + retention + enabled toggle, plus a manual "Run now" trigger.
function ScheduleSection() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: schedule, isLoading } = useQuery<BackupSchedule>({
    queryKey: SCHEDULE_KEY,
    queryFn: () => eveyFetch<BackupSchedule>({ url: "/api/v1/admin/backup-schedule", method: "GET" }),
  });

  const [cron, setCron] = useState<string | undefined>(undefined);
  const [retention, setRetention] = useState<number | undefined>(undefined);
  const [enabled, setEnabled] = useState<boolean | undefined>(undefined);

  const effectiveCron = cron ?? schedule?.cronExpression ?? "";
  const effectiveRetention = retention ?? schedule?.retentionDays ?? 7;
  const effectiveEnabled = enabled ?? schedule?.enabled ?? false;
  const dirty = cron !== undefined || retention !== undefined || enabled !== undefined;
  const cronError = effectiveCron && !isValidCron(effectiveCron)
    ? t("admin.backup.schedule.cronInvalid", "Doit contenir 5 champs (minute heure jour mois jour_semaine)")
    : undefined;

  const saveMutation = useMutation({
    mutationFn: () =>
      eveyFetch<BackupSchedule>({
        url: "/api/v1/admin/backup-schedule",
        method: "PATCH",
        data: { cronExpression: effectiveCron, retentionDays: effectiveRetention, enabled: effectiveEnabled },
      }),
    onSuccess: (data) => {
      qc.setQueryData(SCHEDULE_KEY, data);
      setCron(undefined);
      setRetention(undefined);
      setEnabled(undefined);
      toast.success(t("admin.backup.schedule.saved", "Planification enregistrée"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.schedule.saveFailed", "Échec de l'enregistrement"))),
  });

  const runMutation = useMutation({
    mutationFn: () => eveyFetch<void>({ url: "/api/v1/admin/backup-schedule/run", method: "POST" }),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: BACKUPS_KEY }),
        qc.invalidateQueries({ queryKey: SCHEDULE_KEY }),
      ]);
      toast.success(t("admin.backup.schedule.runStarted", "Sauvegarde lancée"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.schedule.runFailed", "Échec du lancement"))),
  });

  const lastStatus = schedule?.lastStatus ?? "";
  const statusColor = lastStatus === "success" ? theme.colors.success : lastStatus === "failed" ? theme.colors.error : theme.colors.textMuted;
  const statusLabel = lastStatus === "success"
    ? t("admin.backup.schedule.statusSuccess", "Succès")
    : lastStatus === "failed"
      ? t("admin.backup.schedule.statusFailed", "Échec")
      : t("admin.backup.schedule.statusUnknown", "Inconnu");

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, flexDirection: "column", alignItems: "stretch", gap: 12 }]}>
      <View>
        <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>
          {t("admin.backup.schedule.title", "Planification")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
          {t("admin.backup.schedule.description", "Sauvegarde automatique récurrente de la base de données")}
        </Text>
      </View>

      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} />
      ) : (
        <>
          <View style={{ gap: 4 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
              {t("admin.backup.schedule.cronLabel", "Expression cron")}
            </Text>
            <TextInput
              value={effectiveCron}
              onChangeText={setCron}
              placeholder="0 3 * * *"
              placeholderTextColor={theme.colors.textMuted}
              autoCapitalize="none"
              style={[styles.input, { color: theme.colors.text, borderColor: cronError ? theme.colors.error : theme.colors.border, backgroundColor: theme.colors.bg }]}
            />
            {cronError ? (
              <Text style={{ color: theme.colors.error, fontSize: 11 }}>{cronError}</Text>
            ) : (
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {t("admin.backup.schedule.cronHelp", "Format cron standard (minute heure jour mois jour_semaine)")}
              </Text>
            )}
          </View>

          <View style={{ gap: 4 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
              {t("admin.backup.schedule.retentionLabel", "Rétention (jours)")}
            </Text>
            <TextInput
              accessibilityLabel={t("admin.backup.schedule.retentionLabel", "Rétention (jours)")}
              value={String(effectiveRetention)}
              onChangeText={(text) => {
                const v = Number(text);
                setRetention(Number.isFinite(v) && text !== "" ? Math.max(1, v) : effectiveRetention);
              }}
              keyboardType="numeric"
              style={[styles.input, { color: theme.colors.text, borderColor: theme.colors.border, backgroundColor: theme.colors.bg, width: 80 }]}
            />
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {t("admin.backup.schedule.retentionHelp", "Les sauvegardes plus anciennes sont supprimées automatiquement")}
            </Text>
          </View>

          <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between" }}>
            <Text style={{ color: theme.colors.text, fontSize: 13 }}>
              {t("admin.backup.schedule.enabledLabel", "Activer la planification")}
            </Text>
            <Switch
              value={effectiveEnabled}
              onValueChange={setEnabled}
              trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
            />
          </View>

          <View style={{ gap: 2 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {schedule?.lastRunAt
                ? t("admin.backup.schedule.lastRun", { time: new Date(schedule.lastRunAt).toLocaleString(), defaultValue: `Dernière exécution : ${new Date(schedule.lastRunAt).toLocaleString()}` })
                : t("admin.backup.schedule.lastRunNever", "Jamais exécutée")}
            </Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {schedule?.nextRunAt
                ? t("admin.backup.schedule.nextRun", { time: new Date(schedule.nextRunAt).toLocaleString(), defaultValue: `Prochaine exécution : ${new Date(schedule.nextRunAt).toLocaleString()}` })
                : t("admin.backup.schedule.nextRunNotScheduled", "Non planifiée")}
            </Text>
            <Text style={{ color: statusColor, fontSize: 11, fontWeight: "600" }}>{statusLabel}</Text>
          </View>

          <View style={{ flexDirection: "row", gap: 8 }}>
            <Pressable
              onPress={() => saveMutation.mutate()}
              disabled={!dirty || !!cronError || saveMutation.isPending}
              style={[styles.smallBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm, opacity: (!dirty || !!cronError) ? 0.5 : 1 }]}
            >
              {saveMutation.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.onAccent} />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 13 }}>{t("admin.backup.schedule.save", "Enregistrer")}</Text>
              )}
            </Pressable>
            <Pressable
              onPress={() => runMutation.mutate()}
              disabled={runMutation.isPending}
              style={[styles.smallBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm, opacity: runMutation.isPending ? 0.5 : 1 }]}
            >
              {runMutation.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.text} />
              ) : (
                <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 13 }}>{t("admin.backup.schedule.runNow", "Lancer maintenant")}</Text>
              )}
            </Pressable>
          </View>
        </>
      )}
    </View>
  );
}

export default function AdminBackupScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);

  const { data: backups, isLoading, isError, refetch } = useBackups();

  const createBackup = useMutation({
    mutationFn: () => eveyFetch<Backup>({ url: "/api/v1/admin/backups", method: "POST" }),
    onSuccess: async () => {
      toast.success(t("admin.backup.backupCreated"));
      await qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.failedCreate"))),
  });

  const deleteBackup = useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/admin/backups/${id}`, method: "DELETE" }),
    onSuccess: async () => {
      toast.success(t("admin.backup.backupDeleted"));
      await qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.failedDeleteBackup"))),
  });

  const downloadBackup = useMutation({
    mutationFn: (backup: Backup) => downloadBackupFile(backup, accessToken),
    onSuccess: () => toast.success(t("media.downloadComplete")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("media.downloadFailed"))),
  });

  const restoreBackup = useMutation({
    mutationFn: (backup: Backup) => restoreExistingBackup(backup, accessToken),
    onSuccess: () => toast.success(t("admin.backup.restoreCompleted")),
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.restoreFailed"))),
  });

  // Restore from an external archive picked off the device (mirror web RestoreUploadModal).
  const restoreFromFileMut = useMutation({
    mutationFn: (uri: string) => restoreFromFile(uri, accessToken),
    onSuccess: () => { toast.success(t("admin.backup.restoreCompleted")); void qc.invalidateQueries({ queryKey: BACKUPS_KEY }); },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.restoreFailed"))),
  });

  const handleRestoreFromFile = useCallback(async () => {
    const res = await DocumentPicker.getDocumentAsync({
      type: ["application/gzip", "application/x-gzip", "application/x-tar", "application/octet-stream"],
      copyToCacheDirectory: true,
    });
    if (res.canceled || !res.assets?.[0]?.uri) return;
    const uri = res.assets[0].uri;
    Alert.alert(
      t("admin.backup.restoreFromFileTitle", "Restore from file"),
      t("admin.backup.restoreFromFileDesc", "This overwrites the current database with the selected archive. Continue?"),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("common.restore", "Restore"), style: "destructive", onPress: () => restoreFromFileMut.mutate(uri) },
      ],
    );
  }, [restoreFromFileMut, t]);

  const handleDelete = useCallback((backup: Backup) => {
    Alert.alert(
      t("admin.backup.deleteBackupTitle"),
      t("admin.backup.deleteConfirm", { date: new Date(backup.createdAt).toLocaleString() }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("common.delete"), style: "destructive", onPress: () => deleteBackup.mutate(backup.id) },
      ],
    );
  }, [deleteBackup, t]);

  // Restoring overwrites the whole live database - same destructive
  // confirmation contract as the web app's BackupRow.handleRestore modal.
  const handleRestore = useCallback((backup: Backup) => {
    Alert.alert(
      t("admin.backup.restoreBackupTitle"),
      t("admin.backup.restoreConfirm", { date: new Date(backup.createdAt).toLocaleString() }),
      [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("common.restore"), style: "destructive", onPress: () => restoreBackup.mutate(backup) },
      ],
    );
  }, [restoreBackup, t]);

  const renderBackup = useCallback(({ item }: { item: Backup }) => {
    const restorable = isBackupRestorable(item);
    const downloading = downloadBackup.isPending && downloadBackup.variables?.id === item.id;
    const restoring = restoreBackup.isPending && restoreBackup.variables?.id === item.id;
    return (
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
        <View style={{ flex: 1, gap: 2 }}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", fontFamily: "monospace" }} numberOfLines={1}>
            {item.id}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {new Date(item.createdAt).toLocaleString()} · {formatBytes(item.size)}
          </Text>
          {item.version && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>v{item.version}</Text>
          )}
        </View>
        <View style={{ flexDirection: "row", gap: 16 }}>
          <Pressable onPress={() => downloadBackup.mutate(item)} disabled={!restorable || downloading} hitSlop={8} accessibilityRole="button" accessibilityLabel={t("common.download")}>
            {downloading ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : (
              <Ionicons name="download-outline" size={20} color={restorable ? theme.colors.text : theme.colors.textMuted} />
            )}
          </Pressable>
          <Pressable onPress={() => handleRestore(item)} disabled={!restorable || restoring} hitSlop={8} accessibilityRole="button" accessibilityLabel={t("common.restore")}>
            {restoring ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : (
              <Ionicons name="refresh-outline" size={20} color={restorable ? theme.colors.text : theme.colors.textMuted} />
            )}
          </Pressable>
          <Pressable onPress={() => handleDelete(item)} hitSlop={8} accessibilityRole="button" accessibilityLabel={t("common.delete")}>
            <Ionicons name="trash-outline" size={20} color={theme.colors.error} />
          </Pressable>
        </View>
      </View>
    );
  }, [theme, handleDelete, handleRestore, downloadBackup, restoreBackup]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.backup.title") || "Database Backup",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <Pressable
        onPress={() => createBackup.mutate()}
        disabled={createBackup.isPending}
        style={[styles.createBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: createBackup.isPending ? 0.6 : 1 }]}
      >
        {createBackup.isPending ? (
          <ActivityIndicator size="small" color={theme.colors.onAccent} />
        ) : (
          <Ionicons name="add-circle-outline" size={20} color={theme.colors.onAccent} />
        )}
        <Text style={{ color: theme.colors.onAccent, fontSize: 15, fontWeight: "600" }}>
          {t("admin.backup.createBackup")}
        </Text>
      </Pressable>

      <Pressable
        onPress={handleRestoreFromFile}
        disabled={restoreFromFileMut.isPending}
        style={[styles.createBtn, { marginTop: 0, borderWidth: 1, borderColor: theme.colors.border, borderRadius: theme.radii.md, opacity: restoreFromFileMut.isPending ? 0.6 : 1 }]}
      >
        {restoreFromFileMut.isPending ? (
          <ActivityIndicator size="small" color={theme.colors.text} />
        ) : (
          <Ionicons name="cloud-upload-outline" size={20} color={theme.colors.text} />
        )}
        <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>
          {t("admin.backup.restoreFromFile", "Restore from file")}
        </Text>
      </Pressable>

      <FlatList
        data={backups ?? []}
        keyExtractor={(b) => b.id}
        renderItem={renderBackup}
        contentContainerStyle={{ paddingHorizontal: 16, gap: 8, paddingBottom: 32 }}
        refreshing={isLoading}
        onRefresh={refetch}
        ListHeaderComponent={
          <View style={{ marginBottom: 8 }}>
            <ScheduleSection />
          </View>
        }
        ListEmptyComponent={
          isLoading ? null : isError ? (
            <LoadFailed onRetry={() => void refetch()} />
          ) : (
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="cloud-download-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>
                {t("admin.backup.noBackups")}
              </Text>
            </View>
          )
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  createBtn: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, marginHorizontal: 16, marginVertical: 12, paddingVertical: 12 },
  input: {
    fontSize: 14,
    paddingVertical: 8,
    paddingHorizontal: 10,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 6,
  },
  smallBtn: { flex: 1, paddingVertical: 10, alignItems: "center", justifyContent: "center" },
});
