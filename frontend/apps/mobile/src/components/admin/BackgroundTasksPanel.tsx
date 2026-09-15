import { useEffect, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, formatBytes, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";

// BackgroundTasksPanel mirrors the web features/admin/BackgroundTasksPanel.tsx:
// the one place an admin sees what Evey is doing on its own (backfills,
// detections, sweeps), what it costs right now, and where the stop button is.

export interface BackgroundTaskView {
  name: string;
  state: "running" | "idle";
  detail?: string;
  processed: number;
  lastActiveAt?: string;
  startedAt?: string;
  controllable: boolean;
  paused: boolean;
  processes: number;
  rssBytes: number;
  cpuPercent: number;
  cpuSecondsTotal: number;
  forks: number;
}

export const BACKGROUND_TASKS_QUERY_KEY = ["/api/v1/admin/background-tasks"];
const REFRESH_INTERVAL = 5_000;

// Running first (they cost something now), then admin-stopped (need a
// decision), then the idle rest.
function rank(t: BackgroundTaskView): number {
  if (t.state === "running") return 0;
  if (t.paused) return 1;
  return 2;
}

function sinceLabel(t: (k: string, o?: Record<string, unknown>) => string, startedAt?: string): string | null {
  if (!startedAt) return null;
  const mins = Math.round((Date.now() - new Date(startedAt).getTime()) / 60_000);
  if (!Number.isFinite(mins) || mins < 1) return t("admin.backgroundTasks.sinceNow");
  if (mins < 60) return t("admin.backgroundTasks.sinceMinutes", { count: mins });
  return t("admin.backgroundTasks.sinceHours", { count: Math.round(mins / 60) });
}

function TaskRow({ task, theme }: { task: BackgroundTaskView; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  // Confirm-to-act: one tap arms, a second within a few seconds stops (a stop
  // kills the ffmpeg in flight - not something a stray tap should do).
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const id = setTimeout(() => setArmed(false), 4000);
    return () => clearTimeout(id);
  }, [armed]);

  const label = t(`admin.activity.bgTask.${task.name}`, task.name);
  const control = useMutation({
    mutationFn: (action: "stop" | "resume") =>
      eveyFetch<void>({ url: `/api/v1/admin/background-tasks/${encodeURIComponent(task.name)}/${action}`, method: "POST" }),
    onSuccess: (_data, action) => {
      qc.invalidateQueries({ queryKey: BACKGROUND_TASKS_QUERY_KEY });
      toast.success(t(action === "stop" ? "admin.backgroundTasks.stopped" : "admin.backgroundTasks.resumed", { name: label }));
    },
    onError: (err: Error, action) =>
      toast.error(apiErrorMessage(err, t, t(action === "stop" ? "admin.backgroundTasks.stopFailed" : "admin.backgroundTasks.resumeFailed"))),
  });

  const running = task.state === "running";
  const since = running ? sinceLabel(t, task.startedAt) : null;
  const hasCost = running && task.processes > 0;
  const dotColor = running ? theme.colors.success : task.paused ? theme.colors.warning : theme.colors.border;

  return (
    <View style={[styles.row, { borderBottomColor: theme.colors.border }]}>
      <View style={[styles.dot, { backgroundColor: dotColor }]} />
      <View style={{ flex: 1, minWidth: 0 }}>
        <View style={{ flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{label}</Text>
          <View style={[styles.badge, { backgroundColor: `${(task.paused ? theme.colors.warning : running ? theme.colors.success : theme.colors.textMuted)}20` }]}>
            <Text style={{ fontSize: 9, fontWeight: "600", color: task.paused ? theme.colors.warning : running ? theme.colors.success : theme.colors.textMuted }}>
              {task.paused
                ? (running ? t("admin.backgroundTasks.stopping") : t("admin.backgroundTasks.paused"))
                : running ? t("admin.activity.bgRunning") : t("admin.activity.bgIdle")}
            </Text>
          </View>
          {since ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{since}</Text> : null}
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 1 }} numberOfLines={1}>
          {running && task.detail ? task.detail : t("admin.activity.bgProcessed", { n: task.processed })}
          {task.cpuSecondsTotal > 0 ? ` · ${t("admin.backgroundTasks.cpuTotal", { seconds: Math.round(task.cpuSecondsTotal) })}` : ""}
        </Text>
        {hasCost ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 1 }}>
            {t("admin.backgroundTasks.impact", { processes: task.processes, memory: formatBytes(task.rssBytes), cpu: Math.round(task.cpuPercent) })}
          </Text>
        ) : null}
      </View>
      {task.controllable ? (
        task.paused ? (
          <Pressable onPress={() => control.mutate("resume")} disabled={control.isPending} style={[styles.ctrlBtn, { borderColor: theme.colors.border }]}>
            {control.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : (
              <>
                <Ionicons name="play" size={13} color={theme.colors.text} />
                <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.backgroundTasks.resume")}</Text>
              </>
            )}
          </Pressable>
        ) : (
          <Pressable
            onPress={() => { if (!armed) { setArmed(true); return; } setArmed(false); control.mutate("stop"); }}
            disabled={control.isPending}
            style={[styles.ctrlBtn, { borderColor: armed ? theme.colors.danger : theme.colors.border, backgroundColor: armed ? `${theme.colors.danger}20` : "transparent" }]}
          >
            {control.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.danger} />
            ) : (
              <>
                <Ionicons name="stop" size={13} color={armed ? theme.colors.danger : theme.colors.text} />
                <Text style={{ color: armed ? theme.colors.danger : theme.colors.text, fontSize: 12, fontWeight: "600" }}>
                  {armed ? t("admin.backgroundTasks.confirmStop") : t("admin.backgroundTasks.stop")}
                </Text>
              </>
            )}
          </Pressable>
        )
      ) : null}
    </View>
  );
}

export function BackgroundTasksPanel({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data, isError } = useQuery<BackgroundTaskView[]>({
    queryKey: BACKGROUND_TASKS_QUERY_KEY,
    queryFn: () => eveyFetch<BackgroundTaskView[]>({ url: "/api/v1/admin/background-tasks", method: "GET" }),
    refetchInterval: REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
  });
  const tasks = [...(data ?? [])].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
  const runningCount = tasks.filter((x) => x.state === "running").length;

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={styles.headerRow}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Ionicons name="pulse-outline" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.backgroundTasks.title")}</Text>
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
          {runningCount > 0 ? t("admin.backgroundTasks.runningCount", { count: runningCount }) : t("admin.backgroundTasks.allIdle")}
        </Text>
      </View>
      {isError && !data ? (
        <Text style={styles.empty}>{t("admin.backgroundTasks.unavailable")}</Text>
      ) : tasks.length === 0 ? (
        <Text style={styles.empty}>{t("admin.backgroundTasks.empty")}</Text>
      ) : (
        tasks.map((task) => <TaskRow key={task.name} task={task} theme={theme} />)
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 14 },
  headerRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 6 },
  row: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth },
  dot: { width: 8, height: 8, borderRadius: 4 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  ctrlBtn: { flexDirection: "row", alignItems: "center", gap: 4, borderWidth: 1, borderRadius: 8, paddingHorizontal: 10, paddingVertical: 6 },
  empty: { color: "#888", fontSize: 13, textAlign: "center", paddingVertical: 16 },
});
