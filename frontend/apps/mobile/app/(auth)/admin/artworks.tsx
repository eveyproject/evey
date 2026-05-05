import { useState, useEffect, useRef } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const STATS_QUERY_KEY = ["admin", "artworks", "stats"];

interface ArtworkStats {
  totalRecords: number;
  withLocalPath: number;
  missingLocalPath: number;
  totalSizeBytes: number;
}

interface JobStatus {
  status: "running" | "done" | "failed";
  processed: number;
  total: number;
  errors: string[];
}

function useArtworkStats() {
  return useQuery<ArtworkStats>({
    queryKey: STATS_QUERY_KEY,
    queryFn: () =>
      eveyFetch<ArtworkStats>({ url: "/api/v1/admin/artworks/stats", method: "GET" }),
    staleTime: 30_000,
  });
}

function useStartDownload() {
  return useMutation<{ jobId: string; totalToDownload: number }, Error>({
    mutationFn: () =>
      eveyFetch<{ jobId: string; totalToDownload: number }>({
        url: "/api/v1/admin/artworks/download-all",
        method: "POST",
      }),
  });
}

function useJobStatus(jobId: string | null) {
  return useQuery<JobStatus>({
    queryKey: ["admin", "artworks", "job", jobId],
    queryFn: () =>
      eveyFetch<JobStatus>({
        url: `/api/v1/admin/artworks/download-all/${jobId}`,
        method: "GET",
      }),
    enabled: !!jobId,
    refetchInterval: (query) => {
      if (query.state.data?.status === "done") return false;
      return 2000;
    },
    staleTime: 0,
  });
}

function formatPct(n: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((n / total) * 100);
}

export default function ArtworksAdminScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(null);
  const prevDoneRef = useRef(false);

  const { data: stats, isLoading: statsLoading } = useArtworkStats();
  const startDownload = useStartDownload();
  const { data: job } = useJobStatus(jobId);

  useEffect(() => {
    if (!job) return;
    if (job.status === "done" && !prevDoneRef.current) {
      prevDoneRef.current = true;
      qc.invalidateQueries({ queryKey: STATS_QUERY_KEY });
      toast.success(t("admin.artworks.downloadComplete", "Artwork download complete"));
    }
  }, [job, qc, t]);

  function handleStartDownload() {
    prevDoneRef.current = false;
    startDownload.mutate(undefined, {
      onSuccess: (data) => setJobId(data.jobId),
      onError: () => toast.error(t("admin.artworks.downloadFailed", "Download failed")),
    });
  }

  const isRunning = !!jobId && job?.status === "running";
  const isDone = job?.status === "done";
  const s = makeStyles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.artworks.title", "Artworks"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      <Text style={s.subtitle}>
        {t("admin.artworks.subtitle", "Local artwork cache lets posters and backdrops load offline.")}
      </Text>

      {/* Stats card */}
      <View style={s.card}>
        <Text style={s.cardTitle}>{t("admin.artworks.cacheStatus", "Cache status")}</Text>

        {statsLoading ? (
          <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: theme.spacing.md }} />
        ) : stats ? (
          <View style={{ gap: theme.spacing.md }}>
            <View>
              <View style={s.rowBetween}>
                <Text style={s.muted}>
                  {stats.withLocalPath} / {stats.totalRecords} {t("admin.artworks.storedLocally", "cached locally")}
                </Text>
                <Text style={s.muted}>{formatPct(stats.withLocalPath, stats.totalRecords)}%</Text>
              </View>
              <ProgressBar percent={formatPct(stats.withLocalPath, stats.totalRecords)} color={theme.colors.accent} bg={theme.colors.surfaceRaised} />
            </View>

            <View style={s.statsGrid}>
              <View style={s.statTile}>
                <Text style={s.statLabel}>{t("admin.artworks.totalRecords", "Total")}</Text>
                <Text style={s.statValue}>{stats.totalRecords}</Text>
              </View>
              <View style={s.statTile}>
                <Text style={s.statLabel}>{t("admin.artworks.missing", "Missing")}</Text>
                <Text style={[s.statValue, { color: theme.colors.warning }]}>{stats.missingLocalPath}</Text>
              </View>
            </View>
          </View>
        ) : (
          <Text style={s.muted}>{t("admin.artworks.statsUnavailable", "Stats unavailable")}</Text>
        )}
      </View>

      {/* Download section */}
      <View style={s.card}>
        <Text style={s.cardTitle}>{t("admin.artworks.downloadSection", "Bulk download")}</Text>
        <Text style={[s.muted, { marginBottom: theme.spacing.md }]}>
          {t(
            "admin.artworks.downloadDesc",
            "Fetch every missing artwork from the metadata providers and store it locally.",
          )}
        </Text>

        <Pressable
          onPress={handleStartDownload}
          disabled={isRunning || startDownload.isPending}
          style={[s.btn, (isRunning || startDownload.isPending) && s.btnDisabled]}
          accessibilityRole="button"
          accessibilityLabel={t("admin.artworks.downloadMissing", "Download missing")}
        >
          {isRunning ? (
            <ActivityIndicator color={theme.colors.onAccent} size="small" />
          ) : (
            <Ionicons
              name={isDone ? "checkmark-circle-outline" : "cloud-download-outline"}
              size={18}
              color={theme.colors.onAccent}
            />
          )}
          <Text style={s.btnText}>
            {isRunning
              ? t("admin.artworks.downloading", "Downloading…")
              : isDone
                ? t("admin.artworks.downloadAgain", "Download again")
                : t("admin.artworks.downloadMissing", "Download missing")}
          </Text>
        </Pressable>

        {jobId && job ? (
          <View style={{ marginTop: theme.spacing.md }}>
            <View style={s.rowBetween}>
              <Text style={s.tiny}>
                {job.processed} / {job.total}
              </Text>
              <Text style={s.tiny}>
                {job.status === "done"
                  ? t("admin.artworks.done", "Done")
                  : `${formatPct(job.processed, job.total)}%`}
              </Text>
            </View>
            <ProgressBar
              percent={job.total > 0 ? formatPct(job.processed, job.total) : 0}
              color={job.status === "done" ? theme.colors.success : theme.colors.accent}
              bg={theme.colors.surfaceRaised}
            />
            {job.errors.length > 0 ? (
              <View style={{ marginTop: theme.spacing.sm }}>
                <Text style={[s.tiny, { color: theme.colors.warning }]}>
                  {job.errors.length} {t("admin.artworks.errorsPartial", "errors")}
                </Text>
                {job.errors.slice(0, 20).map((e, i) => (
                  <Text key={i} style={s.errorLine} numberOfLines={1}>
                    {e}
                  </Text>
                ))}
              </View>
            ) : null}
          </View>
        ) : null}
      </View>
    </ScrollView>
  );
}

function ProgressBar({ percent, color, bg }: { percent: number; color: string; bg: string }) {
  return (
    <View style={[barStyles.bg, { backgroundColor: bg }]}>
      <View style={[barStyles.fill, { backgroundColor: color, width: `${Math.min(100, Math.max(0, percent))}%` }]} />
    </View>
  );
}

const barStyles = StyleSheet.create({
  bg: { height: 6, borderRadius: 3, overflow: "hidden" },
  fill: { height: "100%", borderRadius: 3 },
});

function makeStyles(t: ReturnType<typeof useTheme>) {
  return StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    content: { padding: t.spacing.md, gap: t.spacing.md, paddingBottom: t.spacing.xl },
    subtitle: { color: t.colors.textMuted, fontSize: 13 },
    card: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      padding: t.spacing.md,
      gap: t.spacing.sm,
    },
    cardTitle: { color: t.colors.text, fontSize: 16, fontWeight: "600" },
    muted: { color: t.colors.textMuted, fontSize: 13 },
    tiny: { color: t.colors.textMuted, fontSize: 11 },
    rowBetween: {
      flexDirection: "row",
      justifyContent: "space-between",
      alignItems: "center",
      marginBottom: 4,
    },
    statsGrid: { flexDirection: "row", gap: t.spacing.sm },
    statTile: {
      flex: 1,
      backgroundColor: t.colors.surfaceRaised,
      borderRadius: t.radii.sm,
      padding: t.spacing.sm + 2,
      gap: 2,
    },
    statLabel: { color: t.colors.textMuted, fontSize: 11 },
    statValue: { color: t.colors.text, fontSize: 16, fontWeight: "700" },
    btn: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "center",
      gap: t.spacing.sm,
      backgroundColor: t.colors.accent,
      paddingVertical: t.spacing.sm + 2,
      paddingHorizontal: t.spacing.md,
      borderRadius: t.radii.md,
    },
    btnDisabled: { opacity: 0.5 },
    btnText: { color: t.colors.onAccent, fontSize: 14, fontWeight: "600" },
    errorLine: { color: t.colors.textMuted, fontFamily: "monospace", fontSize: 10 },
  });
}
