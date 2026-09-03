import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Pause, Play, Square } from "lucide-react";
import { eveyFetch, formatBytes, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { ICON } from "@/constants/iconSizes";

// BackgroundTaskView mirrors backgroundTaskView in the Go HTTP handler: the
// registry entry plus the live cost of the processes forked for the task.
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

// Running first (they are what costs something now), then the ones an admin
// stopped (they need a decision), then the idle rest.
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

function TaskRow({ task }: { task: BackgroundTaskView }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // Confirm-to-act, like the dashboard's Restart: one click arms, a second
  // within a few seconds stops. A stop kills the ffmpeg in flight, which is
  // not something a stray click should do.
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const id = setTimeout(() => setArmed(false), 4000);
    return () => clearTimeout(id);
  }, [armed]);

  const control = useMutation({
    mutationFn: (action: "stop" | "resume") =>
      eveyFetch<void>({ url: `/api/v1/admin/background-tasks/${encodeURIComponent(task.name)}/${action}`, method: "POST" }),
    onSuccess: (_data, action) => {
      queryClient.invalidateQueries({ queryKey: BACKGROUND_TASKS_QUERY_KEY });
      toast.success(t(action === "stop" ? "admin.backgroundTasks.stopped" : "admin.backgroundTasks.resumed", { name: label }));
    },
    onError: (err: Error, action) =>
      toast.error(apiErrorMessage(err, t, t(action === "stop" ? "admin.backgroundTasks.stopFailed" : "admin.backgroundTasks.resumeFailed"))),
  });

  const label = t(`admin.activity.bgTask.${task.name}`, task.name);
  const running = task.state === "running";
  const since = running ? sinceLabel(t, task.startedAt) : null;
  const hasCost = running && task.processes > 0;

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
        </span>
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
            loading={control.isPending}
            onClick={() => control.mutate("resume")}
          >
            <Play size={ICON.xs} aria-hidden="true" />
            {t("admin.backgroundTasks.resume")}
          </Button>
        ) : (
          <Button
            variant={armed ? "danger" : "secondary"}
            loading={control.isPending}
            onClick={() => {
              if (!armed) {
                setArmed(true);
                return;
              }
              setArmed(false);
              control.mutate("stop");
            }}
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
  const { data, isError } = useQuery<BackgroundTaskView[]>({
    queryKey: BACKGROUND_TASKS_QUERY_KEY,
    queryFn: () => eveyFetch<BackgroundTaskView[]>({ url: "/api/v1/admin/background-tasks", method: "GET" }),
    refetchInterval: REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
  });
  const tasks = [...(data ?? [])].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
  const runningCount = tasks.filter((x) => x.state === "running").length;

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
      {isError && !data ? (
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
