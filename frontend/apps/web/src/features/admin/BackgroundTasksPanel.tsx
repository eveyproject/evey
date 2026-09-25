import { useTranslation } from "react-i18next";
import { Activity, Pause, Play, Square } from "lucide-react";
import { formatBytes } from "@evey/api";
import { useBackgroundTaskControl, useBackgroundTasks, type BackgroundTaskView } from "@evey/client";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { ICON } from "@/constants/iconSizes";

// The list, its order and the controls are @evey/client's (useBackgroundTasks).
export { BACKGROUND_TASKS_QUERY_KEY, type BackgroundTaskView } from "@evey/client";

function TaskRow({ task }: { task: BackgroundTaskView }) {
  const { t } = useTranslation();
  const { label, running, since, hasCost, progress, armed, pending, resume, pressStop } = useBackgroundTaskControl(task, t);

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-[var(--border)] py-2 text-sm last:border-b-0">
      <span
        className={`inline-block h-2 w-2 shrink-0 rounded-full ${running ? "animate-pulse bg-[var(--success,#16a34a)]" : task.paused ? "bg-[var(--color-warning)]" : "bg-[var(--border)]"}`}
        aria-hidden="true"
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-[var(--text)]">{label}</span>
          {task.paused ? (
            <Badge variant="warning">{running ? t("admin.backgroundTasks.stopping") : t("admin.backgroundTasks.paused")}</Badge>
          ) : running ? (
            <Badge variant="success">{t("admin.activity.bgRunning")}</Badge>
          ) : (
            <Badge>{t("admin.activity.bgIdle")}</Badge>
          )}
          {since && <span className="text-xs text-[var(--text-muted)]">{since}</span>}
        </span>
        <span className="truncate text-xs text-[var(--text-muted)]">
          {running && task.detail ? task.detail : t("admin.activity.bgProcessed", { n: task.processed })}
          {task.cpuSecondsTotal > 0 && ` · ${t("admin.backgroundTasks.cpuTotal", { seconds: Math.round(task.cpuSecondsTotal) })}`}
          {(task.cpuSlots ?? 0) > 0 && ` · ${t("admin.backgroundTasks.cpuSlots", { held: task.cpuSlotsHeld ?? 0, slots: task.cpuSlots })}`}
        </span>
        {progress && (
          <span className="mt-1 flex items-center gap-2" data-testid={`task-progress-${task.name}`}>
            <span
              className="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--border)]"
              role="progressbar"
              aria-label={t("admin.backgroundTasks.batchProgress", { done: progress.done, total: progress.total })}
              aria-valuemin={0}
              aria-valuemax={progress.total}
              aria-valuenow={progress.done}
            >
              <span className="block h-full rounded-full bg-[var(--accent)] transition-[width] duration-500" style={{ width: `${Math.round(progress.ratio * 100)}%` }} />
            </span>
            <span className="shrink-0 text-xs tabular-nums text-[var(--text-muted)]">
              {t("admin.backgroundTasks.batchProgress", { done: progress.done, total: progress.total })}
            </span>
          </span>
        )}
      </div>
      {hasCost && (
        <span className="shrink-0 text-xs tabular-nums text-[var(--text)]" data-testid={`task-cost-${task.name}`}>
          {t("admin.backgroundTasks.impact", {
            processes: task.processes,
            memory: formatBytes(task.rssBytes),
            cpu: Math.round(task.cpuPercent),
          })}
        </span>
      )}
      {task.controllable && (
        task.paused ? (
          <Button
            variant="secondary"
            loading={pending}
            onClick={resume}
          >
            <Play size={ICON.xs} aria-hidden="true" />
            {t("admin.backgroundTasks.resume")}
          </Button>
        ) : (
          <Button
            variant={armed ? "danger" : "secondary"}
            loading={pending}
            onClick={pressStop}
          >
            <Square size={ICON.xs} aria-hidden="true" />
            {armed ? t("admin.backgroundTasks.confirmStop") : t("admin.backgroundTasks.stop")}
          </Button>
        )
      )}
    </li>
  );
}

// BackgroundTasksPanel is the one place an admin sees what Evey is doing on
// its own (backfills, detections, sweeps), what it costs right now, and where
// the stop button is. Shared by the dashboard and the health page.
export function BackgroundTasksPanel() {
  const { t } = useTranslation();
  const { tasks, runningCount, isError, loaded } = useBackgroundTasks();

  return (
    <Card className="flex flex-col" data-testid="background-tasks-panel">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text)]">
          <Activity size={ICON.md} className="text-[var(--accent)]" aria-hidden="true" />
          {t("admin.backgroundTasks.title")}
        </h3>
        <span className="flex items-center gap-1 text-xs text-[var(--text-muted)]">
          {runningCount > 0 ? (
            t("admin.backgroundTasks.runningCount", { count: runningCount })
          ) : (
            <>
              <Pause size={ICON.xs} aria-hidden="true" />
              {t("admin.backgroundTasks.allIdle")}
            </>
          )}
        </span>
      </div>
      {isError && !loaded ? (
        <p className="py-4 text-center text-sm text-[var(--text-muted)]">{t("admin.backgroundTasks.unavailable")}</p>
      ) : tasks.length === 0 ? (
        <p className="py-4 text-center text-sm text-[var(--text-muted)]">{t("admin.backgroundTasks.empty")}</p>
      ) : (
        <ul className="flex flex-col">
          {tasks.map((task) => (
            <TaskRow key={task.name} task={task} />
          ))}
        </ul>
      )}
    </Card>
  );
}
