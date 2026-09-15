import { View, Text, ScrollView, RefreshControl, Pressable, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useState, useCallback, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { LineChart } from "react-native-gifted-charts";
import { useQuery } from "@tanstack/react-query";
import {
  useAdminSystemServiceGetDashboard,
  useAdminSystemServiceGetSystemInfo,
  eveyFetch,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { formatBytes } from "@evey/api";

const HISTORY_SIZE = 30;

// /api/v1/admin/system/metrics: the finer facts under the two percentages
// (mirror web system-health LoadCard/ProcessCard/ForksCard).
interface SystemMetrics {
  load: { avg1: number; avg5: number; avg15: number; known: boolean; cpus: number; cpuBudget: number };
  memory: { totalBytes: number; availableBytes: number; budgetBytes: number };
  process: { rssBytes: number; heapInUseBytes: number; heapSysBytes: number; goroutines: number; gcCount: number; gcPauseTotalNs: number };
  forks: { count: number; rssBytes: number; cpuPercent: number; byClass: { class: string; count: number; rssBytes: number; cpuPercent: number }[] };
}

function formatUptime(seconds?: string): string {
  if (!seconds) return "-";
  const s = parseInt(seconds, 10);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s % 60}s`;
  return `${s}s`;
}

function formatBps(bps: number): string {
  if (bps >= 1e9) return `${(bps / 1e9).toFixed(2)} Gbps`;
  if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps >= 1e3) return `${(bps / 1e3).toFixed(0)} kbps`;
  return `${Math.max(0, bps)} bps`;
}

function getUsageColor(pct: number, theme: { colors: { error: string; warning: string; success: string } }): string {
  if (pct > 85) return theme.colors.error;
  if (pct > 70) return theme.colors.warning;
  return theme.colors.success;
}

function ProgressBar({ pct, color }: { pct: number; color: string }) {
  return (
    <View style={[styles.progressTrack, { backgroundColor: color + "33" }]}>
      <View style={[styles.progressFill, { width: `${Math.min(100, pct)}%`, backgroundColor: color }]} />
    </View>
  );
}

export default function SystemHealthScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [refreshing, setRefreshing] = useState(false);
  const [serverInfoOpen, setServerInfoOpen] = useState(false);

  const { data: dashboard, refetch: refetchDash } = useAdminSystemServiceGetDashboard({
    query: { refetchInterval: 5_000, refetchIntervalInBackground: false },
  });
  const { data: sysInfo, refetch: refetchSys } = useAdminSystemServiceGetSystemInfo();
  const { data: metrics, refetch: refetchMetrics } = useQuery<SystemMetrics>({
    queryKey: ["/api/v1/admin/system/metrics"],
    queryFn: () => eveyFetch<SystemMetrics>({ url: "/api/v1/admin/system/metrics", method: "GET" }),
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([refetchDash(), refetchSys(), refetchMetrics()]);
    setRefreshing(false);
  }, [refetchDash, refetchSys, refetchMetrics]);

  const cpuPct = Math.min(100, Math.max(0, dashboard?.cpuPercent ?? 0));
  const memPct = Math.min(100, Math.max(0, dashboard?.memoryPercent ?? 0));

  const cpuHistory = useRef<number[]>([]);
  const memHistory = useRef<number[]>([]);
  const bwHistory = useRef<number[]>([]);
  const streamingBps = Number(dashboard?.streamingBps ?? -1);
  useEffect(() => {
    cpuHistory.current = [...cpuHistory.current.slice(-(HISTORY_SIZE - 1)), cpuPct];
    memHistory.current = [...memHistory.current.slice(-(HISTORY_SIZE - 1)), memPct];
    bwHistory.current = [...bwHistory.current.slice(-(HISTORY_SIZE - 1)), Math.max(0, streamingBps)];
  }, [cpuPct, memPct, streamingBps]);

  const cpuData = cpuHistory.current.map((v) => ({ value: v }));
  const memData = memHistory.current.map((v) => ({ value: v }));
  const bwData = bwHistory.current.map((v) => ({ value: v }));

  const serverInfoRows = [
    { label: t("admin.dashboard.version", "Version"), value: sysInfo?.version },
    { label: t("admin.dashboard.os", "OS"), value: sysInfo?.os ? `${sysInfo.os}/${sysInfo.arch}` : undefined },
    { label: "Go", value: sysInfo?.goVersion },
    { label: t("admin.dashboard.ffmpeg", "FFmpeg"), value: sysInfo?.ffmpegVersion },
    { label: t("admin.dashboard.uptime", "Uptime"), value: formatUptime(sysInfo?.uptimeSeconds) },
    { label: t("admin.systemHealth.serverId", "Server ID"), value: sysInfo?.serverId },
  ];

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.systemHealth.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {/* Metrics */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("admin.systemHealth.cpuUsage")} / {t("admin.systemHealth.memoryUsage")}
      </Text>
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        <View style={styles.metricRow}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.systemHealth.cpuUsage")}</Text>
          <Text style={{ color: getUsageColor(cpuPct, theme), fontSize: 13, fontWeight: "700" }}>{cpuPct.toFixed(1)}%</Text>
        </View>
        <ProgressBar pct={cpuPct} color={getUsageColor(cpuPct, theme)} />
        {cpuData.length > 1 && (
          <View style={{ marginTop: 8 }}>
            <LineChart
              data={cpuData}
              width={260}
              height={50}
              hideDataPoints
              hideYAxisText
              hideAxesAndRules
              color={getUsageColor(cpuPct, theme)}
              thickness={2}
              curved
              maxValue={100}
              areaChart
              startFillColor={getUsageColor(cpuPct, theme)}
              endFillColor="transparent"
              startOpacity={0.3}
              endOpacity={0}
              isAnimated={false}
            />
          </View>
        )}

        <View style={[styles.metricRow, { marginTop: 16 }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.systemHealth.memoryUsage")}</Text>
          <Text style={{ color: getUsageColor(memPct, theme), fontSize: 13, fontWeight: "700" }}>{memPct.toFixed(1)}%</Text>
        </View>
        <ProgressBar pct={memPct} color={getUsageColor(memPct, theme)} />
        {memData.length > 1 && (
          <View style={{ marginTop: 8 }}>
            <LineChart
              data={memData}
              width={260}
              height={50}
              hideDataPoints
              hideYAxisText
              hideAxesAndRules
              color={getUsageColor(memPct, theme)}
              thickness={2}
              curved
              maxValue={100}
              areaChart
              startFillColor={getUsageColor(memPct, theme)}
              endFillColor="transparent"
              startOpacity={0.3}
              endOpacity={0}
              isAnimated={false}
            />
          </View>
        )}
      </View>

      {/* Bandwidth */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("admin.systemHealth.bandwidth")}
      </Text>
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        <View style={styles.metricRow}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.systemHealth.bandwidth")}</Text>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "700" }}>
            {streamingBps >= 0 ? formatBps(streamingBps) : "-"}
          </Text>
        </View>
        {bwData.length > 1 && (
          <View style={{ marginTop: 8 }}>
            <LineChart
              data={bwData}
              width={260}
              height={50}
              hideDataPoints
              hideYAxisText
              hideAxesAndRules
              color={theme.colors.accent}
              thickness={2}
              curved
              areaChart
              startFillColor={theme.colors.accent}
              endFillColor="transparent"
              startOpacity={0.3}
              endOpacity={0}
              isAnimated={false}
            />
          </View>
        )}
        <View style={[styles.metricRow, { marginTop: 8 }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
            {t("admin.systemHealth.bandwidthTheoretical", { rate: formatBps(Number(dashboard?.activeSessionsBps ?? 0)) })}
          </Text>
        </View>
        <View style={styles.metricRow}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
            {t("admin.systemHealth.bandwidthServed", { size: formatBytes(dashboard?.streamingBytesTotal) })}
          </Text>
        </View>
      </View>

      {/* System metrics: the finer facts under the two percentages (mirror web
          LoadCard/ProcessCard/ForksCard). Replaces the old activity grid, which
          duplicated the dashboard. */}
      {metrics && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
            {t("admin.systemHealth.metricsSection", "System")}
          </Text>
          <View style={{ marginHorizontal: 16, gap: 8 }}>
            {(() => {
              const budget = metrics.load.cpuBudget > 0 ? metrics.load.cpuBudget : metrics.load.cpus;
              const loadPct = budget > 0 ? (metrics.load.avg1 / budget) * 100 : 0;
              return (
                <View style={[styles.fineCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <View style={styles.fineHead}>
                    <Text style={styles.fineTitle}>{t("admin.systemHealth.load")}</Text>
                    <Text style={{ fontSize: 18, fontWeight: "700", color: metrics.load.known ? getUsageColor(loadPct, theme) : theme.colors.textMuted }}>
                      {metrics.load.known ? `${metrics.load.avg1.toFixed(2)} / ${metrics.load.avg5.toFixed(2)} / ${metrics.load.avg15.toFixed(2)}` : "-"}
                    </Text>
                  </View>
                  <Text style={styles.fineLine}>{t("admin.systemHealth.loadCpus", { budget })}</Text>
                </View>
              );
            })()}

            <View style={[styles.fineCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <View style={styles.fineHead}>
                <Text style={styles.fineTitle}>{t("admin.systemHealth.process")}</Text>
                <Text style={{ fontSize: 18, fontWeight: "700", color: theme.colors.text }}>{formatBytes(metrics.process.rssBytes)}</Text>
              </View>
              <Text style={styles.fineLine}>
                {t("admin.systemHealth.heap", { size: formatBytes(metrics.process.heapInUseBytes) })} · {t("admin.systemHealth.goroutines", { count: metrics.process.goroutines })}
              </Text>
              <Text style={styles.fineLine}>{t("admin.systemHealth.gcCount", { count: metrics.process.gcCount })}</Text>
            </View>

            <View style={[styles.fineCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <View style={styles.fineHead}>
                <Text style={styles.fineTitle}>{t("admin.systemHealth.forks")}</Text>
                <Text style={{ fontSize: 18, fontWeight: "700", color: theme.colors.text }}>
                  {metrics.forks.count === 0 ? "0" : t("admin.systemHealth.forksLine", { count: metrics.forks.count, memory: formatBytes(metrics.forks.rssBytes), cpu: Math.round(metrics.forks.cpuPercent) })}
                </Text>
              </View>
              {metrics.forks.count === 0 ? (
                <Text style={styles.fineLine}>{t("admin.systemHealth.forksNone")}</Text>
              ) : (
                metrics.forks.byClass.map((g) => (
                  <Text key={g.class} style={styles.fineLine}>
                    {t(`admin.systemHealth.forkClass.${g.class}`, g.class)}: {t("admin.systemHealth.forksLine", { count: g.count, memory: formatBytes(g.rssBytes), cpu: Math.round(g.cpuPercent) })}
                  </Text>
                ))
              )}
            </View>
          </View>
        </>
      )}

      {/* Storage */}
      {dashboard?.storage && dashboard.storage.length > 0 && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
            {t("admin.systemHealth.storage")}
          </Text>
          <View style={{ marginHorizontal: 16, gap: 8 }}>
            {dashboard.storage.map((s, i) => {
              const used = parseInt(s.usedBytes ?? "0");
              const total = parseInt(s.totalBytes ?? "1");
              const pct = total > 0 ? Math.round((used / total) * 100) : 0;
              const color = getUsageColor(pct, theme);
              // One card per physical disk. When several libraries live on the same
              // drive the backend groups them, so list every path sharing this disk.
              const displayPaths = s.paths && s.paths.length > 0 ? s.paths : [s.path ?? ""];
              return (
                <View key={i} style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <View style={styles.metricRow}>
                    <View style={{ flex: 1, gap: 2 }}>
                      {displayPaths.map((p) => (
                        <Text key={p} style={{ color: theme.colors.text, fontSize: 13, fontFamily: "monospace" }} numberOfLines={1}>{p}</Text>
                      ))}
                    </View>
                    <Text style={{ color, fontSize: 13, fontWeight: "700" }}>{pct}%</Text>
                  </View>
                  <ProgressBar pct={pct} color={color} />
                  <View style={[styles.metricRow, { marginTop: 6 }]}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                      {t("admin.systemHealth.used", { size: formatBytes(s.usedBytes) })}
                    </Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                      {t("admin.systemHealth.free", { size: formatBytes(s.freeBytes) })}
                    </Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                      {t("admin.systemHealth.total", { size: formatBytes(s.totalBytes) })}
                    </Text>
                  </View>
                </View>
              );
            })}
          </View>
        </>
      )}

      {/* Video Transcode Capacity */}
      {sysInfo?.videoSessions && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
            {t("admin.systemHealth.videoCapacitySection")}
          </Text>
          <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16, gap: 6 }]}>
            <View style={styles.metricRow}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.systemHealth.videoCapacityTitle")}</Text>
              <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "700" }}>
                {t("admin.systemHealth.videoCapacityRatio", {
                  active: sysInfo.videoSessions.active ?? 0,
                  limit: sysInfo.videoSessions.effectiveLimit ?? 0,
                })}
              </Text>
            </View>
            {(sysInfo.videoSessions.effectiveLimit ?? 0) < (sysInfo.videoSessions.autoCapacity ?? 0) && (
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {t("admin.systemHealth.videoCapacityAutoNote", { auto: sysInfo.videoSessions.autoCapacity ?? 0 })}
              </Text>
            )}
            {sysInfo.videoSessions.evidence?.derived === false ? (
              <Text style={{ color: theme.colors.warning, fontSize: 11 }}>
                {t("admin.systemHealth.videoCapacityNotMeasured")}
              </Text>
            ) : sysInfo.videoSessions.evidence ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {t("admin.systemHealth.videoCapacityEvidence", {
                  height: sysInfo.videoSessions.evidence.height ?? 0,
                  fps: (sysInfo.videoSessions.evidence.fps ?? 0).toFixed(1),
                  requiredFps: (sysInfo.videoSessions.evidence.requiredFps ?? 0).toFixed(1),
                  cpuBudget: sysInfo.videoSessions.evidence.cpuBudget ?? 0,
                })}
              </Text>
            ) : null}
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {t("admin.systemHealth.videoCapacityHint")}
            </Text>
          </View>
        </>
      )}

      {/* Server Info (collapsible) */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("admin.systemHealth.serverInfo")}
      </Text>
      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        <Pressable onPress={() => setServerInfoOpen((o) => !o)} style={styles.collapseHeader}>
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600", flex: 1 }}>
            {t("admin.systemHealth.serverInfo")}
          </Text>
          <Ionicons name={serverInfoOpen ? "chevron-up" : "chevron-down"} size={18} color={theme.colors.textMuted} />
        </Pressable>
        {serverInfoOpen && (
          <View>
            {serverInfoRows.map((row) =>
              row.value ? (
                <View key={row.label} style={[styles.infoRow, { borderTopColor: theme.colors.border }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{row.label}</Text>
                  <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1, textAlign: "right" }} numberOfLines={1}>{row.value}</Text>
                </View>
              ) : null
            )}
            {sysInfo?.hwDevices && sysInfo.hwDevices.length > 0 && (
              <View style={[styles.infoRow, { borderTopColor: theme.colors.border, flexDirection: "column", alignItems: "flex-start", gap: 6 }]}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 11, textTransform: "uppercase" }}>
                  {t("admin.systemHealth.hwAcceleration")}
                </Text>
                <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6 }}>
                  {sysInfo.hwDevices.map((d, i) => (
                    <View key={i} style={[styles.hwBadge, { backgroundColor: theme.colors.bg }]}>
                      <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>
                        {(d.type?.replace("HW_ACCEL_TYPE_", "") ?? "?")} - {d.name ?? "unknown"}
                      </Text>
                    </View>
                  ))}
                </View>
              </View>
            )}
          </View>
        )}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  card: { padding: 16, overflow: "hidden" },
  metricRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  progressTrack: { height: 8, borderRadius: 4, marginTop: 6 },
  progressFill: { height: 8, borderRadius: 4 },
  fineCard: { padding: 14, gap: 4 },
  fineHead: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 8 },
  fineTitle: { fontSize: 11, fontWeight: "600", textTransform: "uppercase", color: "#888" },
  fineLine: { fontSize: 12, color: "#888" },
  collapseHeader: { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  infoRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 10, borderTopWidth: StyleSheet.hairlineWidth },
  hwBadge: { paddingVertical: 4, paddingHorizontal: 8, borderRadius: 6 },
});
