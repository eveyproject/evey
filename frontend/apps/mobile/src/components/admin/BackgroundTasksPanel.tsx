import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { formatBytes } from "@evey/api";
import { useBackgroundTaskControl, useBackgroundTasks, type BackgroundTaskView } from "@evey/client";
import { useTheme } from "../../providers/ThemeProvider";

// BackgroundTasksPanel draws what the web's does, from the same @evey/client
// hooks: what Evey is doing on its own (backfills, detections, sweeps), what it
// costs right now, and where the stop button is.
export { BACKGROUND_TASKS_QUERY_KEY, type BackgroundTaskView } from "@evey/client";

function TaskRow({ task, theme }: { task: BackgroundTaskView; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { label, running, since, hasCost, progress, armed, pending, resume, pressStop } = useBackgroundTaskControl(task, t);
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
        {progress ? (
          <View style={styles.progressRow} testID={`task-progress-${task.name}`}>
            <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]}>
              <View style={[styles.progressFill, { backgroundColor: theme.colors.accent, width: `${Math.round(progress.ratio * 100)}%` }]} />
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {t("admin.backgroundTasks.batchProgress", { done: progress.done, total: progress.total })}
            </Text>
          </View>
        ) : null}
        {hasCost ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 1 }}>
            {t("admin.backgroundTasks.impact", { processes: task.processes, memory: formatBytes(task.rssBytes), cpu: Math.round(task.cpuPercent) })}
          </Text>
        ) : null}
      </View>
      {task.controllable ? (
        task.paused ? (
          <Pressable onPress={resume} disabled={pending} style={[styles.ctrlBtn, { borderColor: theme.colors.border }]}>
            {pending ? (
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
            onPress={pressStop}
            disabled={pending}
            style={[styles.ctrlBtn, { borderColor: armed ? theme.colors.danger : theme.colors.border, backgroundColor: armed ? `${theme.colors.danger}20` : "transparent" }]}
          >
            {pending ? (
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
  const { tasks, runningCount, isError, loaded } = useBackgroundTasks();

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
      {isError && !loaded ? (
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
  progressRow: { flexDirection: "row", alignItems: "center", gap: 8, marginTop: 4 },
  progressTrack: { flex: 1, height: 5, borderRadius: 3, overflow: "hidden" },
  progressFill: { height: "100%", borderRadius: 3 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  ctrlBtn: { flexDirection: "row", alignItems: "center", gap: 4, borderWidth: 1, borderRadius: 8, paddingHorizontal: 10, paddingVertical: 6 },
  empty: { color: "#888", fontSize: 13, textAlign: "center", paddingVertical: 16 },
});
