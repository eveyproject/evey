import { useEffect, useRef, useState } from "react";
import { View, Text, FlatList, Pressable, ScrollView, ActivityIndicator, Alert, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useAdminMediaServiceScanSeekIndex,
  useAdminMediaServiceStartSeekIndexScan,
  useAdminMediaServiceGetSeekIndexJob,
  useAdminMediaServiceReindexMediaSeekIndex,
  useAdminMediaServiceRollbackMediaSeekIndex,
  useAdminMediaServiceReindexLibrarySeekIndex,
  useAdminMediaServiceRollbackLibrarySeekIndex,
  useAdminMediaServiceRemuxAudioMedia,
  useAdminMediaServiceDeleteSeekIndexBackup,
  useAdminMediaServicePurgeSeekIndexBackups,
} from "@evey/api";
import type { V1GetSeekIndexJobResponse } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { audioDelayed, baseName, errText, repairableCount, volumeBackupsRisky, BACKUP_WINDOW } from "../../../src/components/admin/seekIndexHelpers";
import { SeekIndexItemRow } from "../../../src/components/admin/seekIndex/SeekIndexItemRow";
import { RepairConfirmModal, type BatchKind } from "../../../src/components/admin/seekIndex/RepairConfirmModal";

const SCAN_QUERY_BASE = "/api/v1/admin/media/seek-index";

const KIND_KEYS: Record<string, string> = {
  scan: "admin.seekIndex.jobScan",
  reindex: "admin.seekIndex.jobReindex",
  rollback: "admin.seekIndex.jobRollback",
  purge: "admin.seekIndex.jobPurge",
  remux_audio: "admin.seekIndex.jobRemuxAudio",
};

function StatChip({ label, value, theme }: { label: string; value: number; theme: ReturnType<typeof useTheme> }) {
  return (
    <View style={[styles.statChip, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

function JobProgressCard({ job, theme }: { job: V1GetSeekIndexJobResponse; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const done = job.done ?? 0;
  const total = job.total ?? 0;
  const failed = job.failed ?? 0;
  const kind = t(KIND_KEYS[job.kind ?? ""] ?? KIND_KEYS.scan);
  const phase = job.phase === "processing" ? t("admin.seekIndex.phaseProcessing") : t("admin.seekIndex.phaseScanning");

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <ActivityIndicator color={theme.colors.accent} size="small" />
        <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", flex: 1 }}>
          {t("admin.seekIndex.jobRunning", { kind, phase })}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {total > 0 ? `${done}/${total}` : "..."}
        </Text>
      </View>
      {failed > 0 && (
        <Text style={{ color: theme.colors.error, fontSize: 12, marginTop: 4 }}>
          {t("admin.seekIndex.jobFailedCount", { count: failed })}
        </Text>
      )}
      {job.currentPath ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 6 }} numberOfLines={1}>
          {baseName(job.currentPath)}
        </Text>
      ) : null}
    </View>
  );
}

function LastJobCard({ last, theme }: { last: NonNullable<V1GetSeekIndexJobResponse["last"]>; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const kind = t(KIND_KEYS[last.kind ?? ""] ?? KIND_KEYS.scan);
  const failed = last.failed ?? 0;
  const skipped = last.skipped ?? 0;
  const isSuccess = failed === 0 && !last.error;

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between" }}>
        <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.seekIndex.lastJob")}</Text>
        {isSuccess ? (
          <View style={[styles.badge, { backgroundColor: theme.colors.success + "22" }]}>
            <Text style={{ color: theme.colors.success, fontSize: 10, fontWeight: "600" }}>
              {t("admin.seekIndex.lastJobOk")}
            </Text>
          </View>
        ) : null}
      </View>
      <Text style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }}>
        {t("admin.seekIndex.lastJobLine", { kind, done: last.done ?? 0, total: last.total ?? 0, failed })}
      </Text>
      {skipped > 0 ? (
        <Pressable onPress={() => Alert.alert(t("admin.seekIndex.jobSkippedCount", { count: skipped }), t("admin.seekIndex.jobSkippedHint"))}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 4 }}>
            {t("admin.seekIndex.jobSkippedCount", { count: skipped })}
          </Text>
        </Pressable>
      ) : null}
      {last.error ? (
        <Text style={{ color: theme.colors.error, fontSize: 12, marginTop: 4 }}>
          {t("admin.seekIndex.lastJobError", { error: last.error })}
        </Text>
      ) : null}
      {last.readOnly ? (
        <Text style={{ color: theme.colors.warning, fontSize: 12, marginTop: 4 }}>
          {t("admin.seekIndex.jobReadOnlyHint")}
        </Text>
      ) : null}
    </View>
  );
}

export default function AdminSeekIndexScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [libraryId, setLibraryId] = useState("");
  const [onlyAttention, setOnlyAttention] = useState(true);
  const [confirmBatch, setConfirmBatch] = useState<BatchKind | null>(null);
  // keepBackups/alsoAudio mirror the web modal: strategy is always "auto", the
  // operator only picks whether backups survive the run and whether delayed
  // audio is folded into the same job. Pre-set from live scan data when the
  // dialog opens (see openRepairAll below).
  const [keepBackups, setKeepBackups] = useState(true);
  const [alsoAudio, setAlsoAudio] = useState(true);

  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];

  // The job endpoint is the live heartbeat: poll fast while a job runs so the
  // progress bar moves, slowly otherwise to pick up jobs started elsewhere.
  const jobQuery = useAdminMediaServiceGetSeekIndexJob({
    query: { refetchInterval: (q) => (q.state.data?.running ? 1500 : 10_000), refetchIntervalInBackground: false },
  });
  const job = jobQuery.data;
  const running = job?.running ?? false;

  const scan = useAdminMediaServiceScanSeekIndex(libraryId ? { libraryId } : undefined, { query: { retry: false } });
  const hasScan = Number(scan.data?.scannedAtMs ?? 0) > 0;
  const result = hasScan ? scan.data : undefined;
  const items = result?.items ?? [];
  const missing = result?.missingIndex ?? 0;
  const withBackup = result?.withBackup ?? 0;
  const delayedAudio = result?.audioDelayed ?? 0;
  const wrongContainer = result?.wrongContainer ?? 0;
  const trailingJunk = result?.trailingJunk ?? 0;
  const audioShort = result?.audioShort ?? 0;
  const seekCoarse = result?.seekCoarse ?? 0;
  // What "Repair all" actually acts on - see repairableCount's doc comment.
  const repairable = repairableCount(items);
  const repairVolumes = (result?.volumes ?? []).filter((v) => (v.missingFiles ?? 0) > 0);
  const backupsRisky = repairVolumes.some(volumeBackupsRisky);

  const prevRunning = useRef(false);
  useEffect(() => {
    if (prevRunning.current && !running) {
      void queryClient.invalidateQueries({ queryKey: [SCAN_QUERY_BASE] });
      const last = job?.last;
      if (last) {
        const kind = t(KIND_KEYS[last.kind ?? ""] ?? KIND_KEYS.scan);
        if (last.error) {
          toast.error(t("admin.seekIndex.lastJobError", { error: last.error }));
        } else if ((last.failed ?? 0) > 0) {
          toast.error(t("admin.seekIndex.jobCompletedFailures", { kind, done: last.done ?? 0, failed: last.failed ?? 0 }));
        } else if ((last.skipped ?? 0) > 0) {
          toast.success(
            t("admin.seekIndex.jobCompletedSkipped", { kind, done: (last.done ?? 0) - (last.skipped ?? 0), skipped: last.skipped ?? 0 }),
          );
        } else {
          toast.success(t("admin.seekIndex.jobCompleted", { kind, done: last.done ?? 0 }));
        }
      }
    }
    prevRunning.current = running;
  }, [running, job?.last, queryClient, t]);

  const onStarted = (msgKey: string) => () => {
    toast.info(t(msgKey));
    void queryClient.invalidateQueries({ queryKey: jobQuery.queryKey ?? [] });
  };
  const onStartError = (err: { message?: string } | null) => toast.error(errText(err, t("admin.seekIndex.startFailed")));

  const startScan = useAdminMediaServiceStartSeekIndexScan({
    mutation: { onSuccess: onStarted("admin.seekIndex.scanStarted"), onError: onStartError },
  });
  const repairOne = useAdminMediaServiceReindexMediaSeekIndex({
    mutation: { onSuccess: onStarted("admin.seekIndex.repairStarted"), onError: onStartError },
  });
  const rollbackOne = useAdminMediaServiceRollbackMediaSeekIndex({
    mutation: { onSuccess: onStarted("admin.seekIndex.rollbackStarted"), onError: onStartError },
  });
  const repairLib = useAdminMediaServiceReindexLibrarySeekIndex({
    mutation: { onSuccess: onStarted("admin.seekIndex.repairStarted"), onError: onStartError },
  });
  const rollbackLib = useAdminMediaServiceRollbackLibrarySeekIndex({
    mutation: { onSuccess: onStarted("admin.seekIndex.rollbackStarted"), onError: onStartError },
  });
  const remuxAudioOne = useAdminMediaServiceRemuxAudioMedia({
    mutation: { onSuccess: onStarted("admin.seekIndex.remuxAudioStarted"), onError: onStartError },
  });
  // Synchronous (an unlink server-side): refresh the cached scan directly
  // instead of the job-completion path the batch mutations rely on.
  const deleteBackup = useAdminMediaServiceDeleteSeekIndexBackup({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.seekIndex.backupDeleted"));
        void queryClient.invalidateQueries({ queryKey: [SCAN_QUERY_BASE] });
      },
      onError: onStartError,
    },
  });
  const purgeLib = useAdminMediaServicePurgeSeekIndexBackups({
    mutation: { onSuccess: onStarted("admin.seekIndex.purgeStarted"), onError: onStartError },
  });

  const visible = onlyAttention
    ? items.filter(
        (it) =>
          !it.hasIndex ||
          it.hasBackup ||
          audioDelayed(it) ||
          it.wrongContainer ||
          it.truncated ||
          Number(it.damagedBytes ?? 0) > 0 ||
          (it.audioShortfallSec ?? 0) > 0,
      )
    : items;
  const busy = running || startScan.isPending;

  function openRepairAll() {
    // Pre-set the safe choices; the operator can still flip them in the modal.
    setKeepBackups(!backupsRisky);
    setAlsoAudio(delayedAudio > 0);
    setConfirmBatch("repair");
  }

  function confirmBatchAction() {
    if (confirmBatch === "rollback") {
      rollbackLib.mutate({ data: libraryId ? { libraryId } : {} });
    } else if (confirmBatch === "purge") {
      purgeLib.mutate({ data: libraryId ? { libraryId } : {} });
    } else if (confirmBatch === "repair") {
      repairLib.mutate({
        data: {
          ...(libraryId ? { libraryId } : {}),
          mode: "auto",
          includeAudio: alsoAudio,
          deepVerify: !keepBackups,
          ...(keepBackups ? {} : { backupWindow: BACKUP_WINDOW }),
        },
      });
    }
    setConfirmBatch(null);
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.seekIndex.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <FlatList
        data={visible}
        keyExtractor={(it, index) => it.mediaId ?? String(index)}
        contentContainerStyle={{ padding: 16, gap: 8 }}
        renderItem={({ item }) => (
          <SeekIndexItemRow
            item={item}
            theme={theme}
            busy={busy}
            onRepair={(mediaId) => repairOne.mutate({ mediaId, data: { mode: "auto" } })}
            onRepairResync={(mediaId) => repairOne.mutate({ mediaId, data: { resync: true } })}
            onRollback={(mediaId) => rollbackOne.mutate({ mediaId })}
            onRemuxAudio={(mediaId) => remuxAudioOne.mutate({ mediaId })}
            onDeleteBackup={(mediaId) => deleteBackup.mutate({ data: { mediaId } })}
            repairPending={repairOne.isPending && repairOne.variables?.mediaId === item.mediaId && !repairOne.variables?.data?.resync}
            repairResyncPending={repairOne.isPending && repairOne.variables?.mediaId === item.mediaId && repairOne.variables?.data?.resync === true}
            rollbackPending={rollbackOne.isPending && rollbackOne.variables?.mediaId === item.mediaId}
            remuxAudioPending={remuxAudioOne.isPending && remuxAudioOne.variables?.mediaId === item.mediaId}
            deleteBackupPending={deleteBackup.isPending && deleteBackup.variables?.data?.mediaId === item.mediaId}
          />
        )}
        ListHeaderComponent={
          <View style={{ gap: 12, marginBottom: 12 }}>
            <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, gap: 6 }]}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.seekIndex.description")}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.seekIndex.readOnlyNote")}</Text>
            </View>

            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>{t("admin.seekIndex.library")}</Text>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
              <Pressable
                onPress={() => setLibraryId("")}
                style={[styles.filterChip, { backgroundColor: libraryId === "" ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
              >
                <Text style={{ color: libraryId === "" ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
                  {t("admin.seekIndex.allLibraries")}
                </Text>
              </Pressable>
              {libraries.map((lib) => (
                <Pressable
                  key={lib.id}
                  onPress={() => lib.id && setLibraryId(lib.id)}
                  style={[styles.filterChip, { backgroundColor: libraryId === lib.id ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
                >
                  <Text style={{ color: libraryId === lib.id ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
                    {lib.name ?? lib.id}
                  </Text>
                </Pressable>
              ))}
            </ScrollView>

            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
              <Pressable
                disabled={busy}
                onPress={() => startScan.mutate({ data: libraryId ? { libraryId } : {} })}
                style={[styles.smallBtn, { backgroundColor: theme.colors.accent, opacity: busy ? 0.6 : 1 }]}
              >
                {startScan.isPending ? (
                  <ActivityIndicator color={theme.colors.onAccent} size="small" />
                ) : (
                  <Text style={{ color: theme.colors.onAccent, fontSize: 13, fontWeight: "600" }}>
                    {result ? t("admin.seekIndex.scanAgain") : t("admin.seekIndex.scan")}
                  </Text>
                )}
              </Pressable>
              {result && (repairable > 0 || delayedAudio > 0) && (
                <Pressable disabled={busy} onPress={openRepairAll} style={[styles.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                    {t("admin.seekIndex.repairAll", { count: repairable + delayedAudio })}
                  </Text>
                </Pressable>
              )}
              {result && withBackup > 0 && (
                <Pressable disabled={busy} onPress={() => setConfirmBatch("rollback")} style={[styles.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                    {t("admin.seekIndex.rollbackAll", { count: withBackup })}
                  </Text>
                </Pressable>
              )}
              {result && withBackup > 0 && (
                <Pressable disabled={busy} onPress={() => setConfirmBatch("purge")} style={[styles.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                    {t("admin.seekIndex.purgeAll", { count: withBackup })}
                  </Text>
                </Pressable>
              )}
            </View>

            {running && job ? <JobProgressCard job={job} theme={theme} /> : null}
            {!running && job?.last ? <LastJobCard last={job.last} theme={theme} /> : null}

            {scan.isError ? (
              <Text style={{ color: theme.colors.error, fontSize: 13 }}>{errText(scan.error, t("admin.seekIndex.scanFailed"))}</Text>
            ) : null}

            {result && (
              <>
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                  <StatChip label={t("admin.seekIndex.statsInspected")} value={result.total ?? 0} theme={theme} />
                  <StatChip label={t("admin.seekIndex.statsMissing")} value={missing} theme={theme} />
                  <StatChip label={t("admin.seekIndex.statsBackup")} value={withBackup} theme={theme} />
                  <StatChip label={t("admin.seekIndex.statsAudioDelay")} value={delayedAudio} theme={theme} />
                  {wrongContainer > 0 && <StatChip label={t("admin.seekIndex.statsWrongContainer")} value={wrongContainer} theme={theme} />}
                  {trailingJunk > 0 && <StatChip label={t("admin.seekIndex.statsTrailingJunk")} value={trailingJunk} theme={theme} />}
                  {audioShort > 0 && <StatChip label={t("admin.seekIndex.statsAudioShort")} value={audioShort} theme={theme} />}
                  {seekCoarse > 0 && <StatChip label={t("admin.seekIndex.statsSeekCoarse")} value={seekCoarse} theme={theme} />}
                </ScrollView>

                <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                  <Pressable
                    onPress={() => setOnlyAttention((v) => !v)}
                    style={{ flexDirection: "row", alignItems: "center", gap: 8 }}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked: onlyAttention }}
                  >
                    <Ionicons name={onlyAttention ? "checkbox" : "square-outline"} size={20} color={onlyAttention ? theme.colors.accent : theme.colors.textMuted} />
                    <Text style={{ color: theme.colors.text, fontSize: 13 }}>{t("admin.seekIndex.onlyAttention")}</Text>
                  </Pressable>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {t("admin.seekIndex.scannedAt", { date: new Date(Number(result.scannedAtMs)).toLocaleString() })}
                  </Text>
                </View>
              </>
            )}
          </View>
        }
        ListEmptyComponent={
          !result ? (
            <View style={{ alignItems: "center", paddingTop: 40 }}>
              <Ionicons name="build-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600", marginTop: 12, textAlign: "center" }}>
                {t("admin.seekIndex.emptyTitle")}
              </Text>
              <Text style={{ color: theme.colors.textMuted, marginTop: 4, textAlign: "center" }}>
                {t("admin.seekIndex.emptyDescription")}
              </Text>
            </View>
          ) : (
            <View style={{ alignItems: "center", paddingTop: 40 }}>
              <Text style={{ color: theme.colors.textMuted }}>
                {items.length === 0 ? t("admin.seekIndex.noEbml") : t("admin.seekIndex.allGood")}
              </Text>
            </View>
          )
        }
      />

      <RepairConfirmModal
        theme={theme}
        batch={confirmBatch}
        onClose={() => setConfirmBatch(null)}
        onConfirm={confirmBatchAction}
        pending={repairLib.isPending || rollbackLib.isPending || purgeLib.isPending}
        repairableCount={repairable}
        withBackupCount={withBackup}
        delayedAudioCount={delayedAudio}
        volumes={repairVolumes}
        keepBackups={keepBackups}
        setKeepBackups={setKeepBackups}
        alsoAudio={alsoAudio}
        setAlsoAudio={setAlsoAudio}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 12 },
  statChip: { padding: 12, minWidth: 80, alignItems: "center" },
  statValue: { fontSize: 20, fontWeight: "700" },
  statLabel: { fontSize: 11, textAlign: "center" },
  filterChip: { paddingHorizontal: 12, paddingVertical: 8 },
  smallBtn: { paddingHorizontal: 14, paddingVertical: 8, borderRadius: 8, alignItems: "center", justifyContent: "center", minWidth: 70 },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4 },
});
