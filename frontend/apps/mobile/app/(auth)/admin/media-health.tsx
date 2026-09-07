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
  useAdminMediaServiceDeleteSeekIndexBackup,
  useAdminMediaServicePurgeSeekIndexBackups,
  useAdminMediaServiceAckSeekIndexMedia,
  useAdminMediaServiceAckSeekIndexGroup,
  useAdminMediaServiceFixContainerMedia,
} from "@evey/api";
import type { V1GetSeekIndexJobResponse, SeekView, SeekScanPace } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import {
  audioTracksDisagree,
  baseName,
  errText,
  needsResync,
  triageOf,
  triageRows,
  verdictOf,
  viewCounts,
  volumeBackupsRisky,
  SEEK_INDEX_BACKUP_WINDOW,
  SEEK_INDEX_SCAN_QUERY_BASE,
  SEEK_INDEX_KIND_KEYS,
  SEEK_VIEWS,
  SEEK_SCAN_PACE_DEFAULT,
  scanRequestBody,
} from "@evey/api";
import { SeekIndexItemRow } from "../../../src/components/admin/seekIndex/SeekIndexItemRow";
import { RepairConfirmModal, type BatchKind } from "../../../src/components/admin/seekIndex/RepairConfirmModal";

// One card per verdict (same triage as the web grid), and there are two: what
// Evey repairs here, and what it merely noticed. The number, what it is made
// of, and - for the remarks - the button that waves them through.
function StatChip({
  label,
  value,
  detail,
  action,
  theme,
}: {
  label: string;
  value: number;
  detail?: string;
  action?: { label: string; onPress: () => void; disabled: boolean };
  theme: ReturnType<typeof useTheme>;
}) {
  return (
    <View style={[styles.statChip, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
      {detail ? <Text style={[styles.statDetail, { color: theme.colors.textMuted }]}>{detail}</Text> : null}
      {action && value > 0 ? (
        <Pressable disabled={action.disabled} onPress={action.onPress} style={{ marginTop: 6, opacity: action.disabled ? 0.6 : 1 }}>
          <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>{action.label}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const VIEW_KEYS: Record<SeekView, string> = {
  attention: "admin.seekIndex.viewAttention",
  repairable: "admin.seekIndex.viewRepairable",
  informational: "admin.seekIndex.viewInformational",
  acked: "admin.seekIndex.viewAcked",
  backups: "admin.seekIndex.viewBackups",
  all: "admin.seekIndex.viewAll",
};

function JobProgressCard({ job, theme }: { job: V1GetSeekIndexJobResponse; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const done = job.done ?? 0;
  const total = job.total ?? 0;
  const failed = job.failed ?? 0;
  const kind = t(SEEK_INDEX_KIND_KEYS[job.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
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
  const kind = t(SEEK_INDEX_KIND_KEYS[last.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
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
  // The displayed rows: one view (what awaits attention by default), sorted by
  // priority - the same pipeline as the web page (triageRows).
  const [view, setView] = useState<SeekView>("attention");
  // How much of the disks the next scan may take. Gentle by default, same as
  // the web page: nobody watches a scan, everybody watches their films.
  const [pace, setPace] = useState<SeekScanPace>(SEEK_SCAN_PACE_DEFAULT);
  const [confirmBatch, setConfirmBatch] = useState<BatchKind | null>(null);
  // keepBackups/resyncDamaged mirror the web modal: strategy is always "auto",
  // "repair all" repairs every repairable verdict, and the operator only picks
  // whether backups survive the run and whether the lossy resync of damaged
  // files is part of it. Pre-set when the dialog opens (see openRepairAll).
  const [keepBackups, setKeepBackups] = useState(true);
  const [resyncDamaged, setResyncDamaged] = useState(true);

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
  const withBackup = result?.withBackup ?? 0;
  const containerFixAvailable = result?.containerFixAvailable ?? false;
  // The triage is derived from the very rows the list shows, so the cards, the
  // button and the rows can never disagree.
  const triage = triageOf(items);
  const repairable = triage.repairable;
  const counts = viewCounts(items);
  const repairableRows = items.filter((it) => verdictOf(it) === "repairable");
  // The resync bucket: the corruption the scan MEASURED, plus the files a
  // repair already walked and whose source refused to parse. The byte total
  // covers only the first kind - how much the second loses is not knowable
  // before the rewrite - so the two are counted together and stated apart.
  const measuredDamagedRows = repairableRows.filter((it) => it.hasIndex && Number(it.damagedBytes ?? 0) > 0);
  const resyncRequiredRows = repairableRows.filter((it) => it.resyncRequired);
  const damagedRows = repairableRows.filter((it) => needsResync(it));
  const damagedBytes = measuredDamagedRows.reduce((sum, it) => sum + Number(it.damagedBytes ?? 0), 0);
  const detail = (parts: [number, string][]) =>
    parts.filter(([n]) => n > 0).map(([n, key]) => t(key, { count: n })).join(" · ") || undefined;
  const repairableDetail = detail([
    [repairableRows.filter((it) => !it.hasIndex).length, "admin.seekIndex.partMissing"],
    [measuredDamagedRows.length, "admin.seekIndex.partDamaged"],
    [items.filter((it) => it.wrongContainer).length, "admin.seekIndex.partWrongContainer"],
    [resyncRequiredRows.length, "admin.seekIndex.partResyncRequired"],
  ]);
  // Everything Evey cannot fix here is ONE pile: a remark, never a chore.
  const infoDetail = detail([
    [items.filter((it) => it.truncated && !it.ackedAtMs).length, "admin.seekIndex.partTruncated"],
    [items.filter((it) => it.seekCoarse && !it.truncated && !it.ackedAtMs).length, "admin.seekIndex.partSeekCoarse"],
    [items.filter((it) => audioTracksDisagree(it) && !it.ackedAtMs).length, "admin.seekIndex.partTracksDisagree"],
    [items.filter((it) => verdictOf(it) === "informational" && (it.audioDelaySec ?? 0) > 0.1 && !it.ackedAtMs).length, "admin.seekIndex.partAudioDelay"],
    [items.filter((it) => verdictOf(it) === "informational" && (it.audioShortfallSec ?? 0) > 0 && !it.ackedAtMs).length, "admin.seekIndex.partAudioShort"],
    [items.filter((it) => verdictOf(it) === "informational" && it.trailingJunk && !it.ackedAtMs).length, "admin.seekIndex.partTrailingJunk"],
  ]);
  const repairVolumes = (result?.volumes ?? []).filter((v) => (v.missingFiles ?? 0) > 0);
  const backupsRisky = repairVolumes.some(volumeBackupsRisky);

  const prevRunning = useRef(false);
  useEffect(() => {
    if (prevRunning.current && !running) {
      void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      const last = job?.last;
      if (last) {
        const kind = t(SEEK_INDEX_KIND_KEYS[last.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
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
  // Synchronous (an unlink server-side): refresh the cached scan directly
  // instead of the job-completion path the batch mutations rely on.
  const deleteBackup = useAdminMediaServiceDeleteSeekIndexBackup({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.seekIndex.backupDeleted"));
        void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      },
      onError: onStartError,
    },
  });
  const purgeLib = useAdminMediaServicePurgeSeekIndexBackups({
    mutation: { onSuccess: onStarted("admin.seekIndex.purgeStarted"), onError: onStartError },
  });
  const fixContainerOne = useAdminMediaServiceFixContainerMedia({
    mutation: { onSuccess: onStarted("admin.seekIndex.fixContainerStarted"), onError: onStartError },
  });
  // Acknowledging is the operator saying "seen, nothing to do here": the row
  // leaves the attention list until the file's bytes change.
  const ackOne = useAdminMediaServiceAckSeekIndexMedia({
    mutation: {
      onSuccess: (_res, vars) => {
        toast.success(t(vars.data.acked ? "admin.seekIndex.ackDone" : "admin.seekIndex.unackDone"));
        void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      },
      onError: onStartError,
    },
  });
  const ackGroup = useAdminMediaServiceAckSeekIndexGroup({
    mutation: {
      onSuccess: (res) => {
        toast.success(t("admin.seekIndex.ackGroupDone", { count: res.acked ?? 0 }));
        void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      },
      onError: onStartError,
    },
  });
  const ackAll = () => ackGroup.mutate({ data: { ...(libraryId ? { libraryId } : {}), verdict: "informational" } });

  const visible = triageRows(items, view, "", "priority");
  const busy = running || startScan.isPending;

  function openRepairAll() {
    // Pre-set the safe choices; the operator can still flip them in the modal.
    setKeepBackups(!backupsRisky);
    setResyncDamaged(true);
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
          deepVerify: !keepBackups,
          ...(keepBackups ? {} : { backupWindow: SEEK_INDEX_BACKUP_WINDOW }),
          ...(damagedRows.length > 0 && resyncDamaged ? { resyncDamaged: true } : {}),
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
            containerFixAvailable={containerFixAvailable}
            onRepair={(mediaId) => repairOne.mutate({ mediaId, data: { mode: "auto" } })}
            onRepairResync={(mediaId) => repairOne.mutate({ mediaId, data: { resync: true } })}
            onRollback={(mediaId) => rollbackOne.mutate({ mediaId })}
            onFixContainer={(mediaId) => fixContainerOne.mutate({ mediaId })}
            onAck={(mediaId, acked) => ackOne.mutate({ data: { mediaId, acked } })}
            onDeleteBackup={(mediaId) => deleteBackup.mutate({ data: { mediaId } })}
            repairPending={repairOne.isPending && repairOne.variables?.mediaId === item.mediaId && !repairOne.variables?.data?.resync}
            repairResyncPending={repairOne.isPending && repairOne.variables?.mediaId === item.mediaId && repairOne.variables?.data?.resync === true}
            rollbackPending={rollbackOne.isPending && rollbackOne.variables?.mediaId === item.mediaId}
            fixContainerPending={fixContainerOne.isPending && fixContainerOne.variables?.mediaId === item.mediaId}
            ackPending={ackOne.isPending && ackOne.variables?.data?.mediaId === item.mediaId}
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

            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>{t("admin.seekIndex.pace")}</Text>
            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
              {(["gentle", "fast"] as const).map((p) => (
                <Pressable
                  key={p}
                  disabled={busy}
                  onPress={() => setPace(p)}
                  style={[styles.filterChip, { backgroundColor: pace === p ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md, opacity: busy ? 0.6 : 1 }]}
                >
                  <Text style={{ color: pace === p ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
                    {p === "fast" ? t("admin.seekIndex.paceFast") : t("admin.seekIndex.paceGentle")}
                  </Text>
                </Pressable>
              ))}
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
              {pace === "fast" ? t("admin.seekIndex.paceFastHint") : t("admin.seekIndex.paceGentleHint")}
            </Text>

            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
              <Pressable
                disabled={busy}
                onPress={() => startScan.mutate({ data: scanRequestBody(libraryId, pace) })}
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
              {result && repairable > 0 && (
                <Pressable disabled={busy} onPress={openRepairAll} style={[styles.smallBtn, { backgroundColor: theme.colors.bg, opacity: busy ? 0.6 : 1 }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                    {t("admin.seekIndex.repairAll", { count: repairable })}
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
                  <StatChip label={t("admin.seekIndex.statsRepairable")} value={triage.repairable} detail={repairableDetail} theme={theme} />
                  <StatChip
                    label={t("admin.seekIndex.statsInformational")}
                    value={triage.informational}
                    detail={infoDetail}
                    action={{ label: t("admin.seekIndex.ackGroupBtn", { count: triage.informational }), onPress: ackAll, disabled: busy || ackGroup.isPending }}
                    theme={theme}
                  />
                  <StatChip
                    label={t("admin.seekIndex.statsBackup")}
                    value={withBackup}
                    detail={triage.acked > 0 ? t("admin.seekIndex.partAcked", { count: triage.acked }) : undefined}
                    theme={theme}
                  />
                </ScrollView>

                {/* The views: one chip per verdict with its live count, plus
                    acknowledged / backups / all - what the single "attention"
                    checkbox could not say it was hiding. */}
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                  {SEEK_VIEWS.map((v) => (
                    <Pressable
                      key={v}
                      onPress={() => setView(v)}
                      accessibilityRole="button"
                      accessibilityState={{ selected: view === v }}
                      style={[styles.filterChip, { backgroundColor: view === v ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
                    >
                      <Text style={{ color: view === v ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
                        {t(VIEW_KEYS[v], { count: counts[v] })}
                      </Text>
                    </Pressable>
                  ))}
                </ScrollView>
                <Text style={{ color: theme.colors.textMuted, fontSize: 11, textAlign: "right" }}>
                  {t("admin.seekIndex.scannedAt", { date: new Date(Number(result.scannedAtMs)).toLocaleString() })}
                </Text>
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
                {items.length === 0 ? t("admin.seekIndex.noEbml") : view === "attention" ? t("admin.seekIndex.allGood") : t("admin.seekIndex.noneInView")}
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
        damagedCount={damagedRows.length}
        damagedBytes={damagedBytes}
        volumes={repairVolumes}
        keepBackups={keepBackups}
        setKeepBackups={setKeepBackups}
        resyncDamaged={resyncDamaged}
        setResyncDamaged={setResyncDamaged}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 12 },
  statChip: { padding: 12, minWidth: 96, maxWidth: 180, alignItems: "center" },
  statValue: { fontSize: 20, fontWeight: "700" },
  statLabel: { fontSize: 11, textAlign: "center" },
  statDetail: { fontSize: 10, textAlign: "center", marginTop: 2 },
  filterChip: { paddingHorizontal: 12, paddingVertical: 8 },
  smallBtn: { paddingHorizontal: 14, paddingVertical: 8, borderRadius: 8, alignItems: "center", justifyContent: "center", minWidth: 70 },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4 },
});
