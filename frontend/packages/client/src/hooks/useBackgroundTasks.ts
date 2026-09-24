import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import type { TFunction } from "i18next";

// The admin's background tasks (backfills, detections, sweeps): what runs, what
// it costs, and the stop button. The web and the phone drew the same list from
// two copies of this logic; they now only draw it.

// Mirrors backgroundTaskView in the Go HTTP handler: the registry entry plus the
// live cost of the processes forked for the task.
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

export function useBackgroundTasks() {
  const query = useQuery<BackgroundTaskView[]>({
    queryKey: BACKGROUND_TASKS_QUERY_KEY,
    queryFn: () => eveyFetch<BackgroundTaskView[]>({ url: "/api/v1/admin/background-tasks", method: "GET" }),
    refetchInterval: REFRESH_INTERVAL,
    refetchIntervalInBackground: false,
  });
  const tasks = [...(query.data ?? [])].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name));
  return {
    tasks,
    runningCount: tasks.filter((x) => x.state === "running").length,
    isError: query.isError,
    // A failed refresh keeps the last list on screen; only no list at all is an error state.
    loaded: query.data !== undefined,
  };
}

export function backgroundTaskSince(t: TFunction, startedAt?: string): string | null {
  if (!startedAt) return null;
  const mins = Math.round((Date.now() - new Date(startedAt).getTime()) / 60_000);
  if (!Number.isFinite(mins) || mins < 1) return t("admin.backgroundTasks.sinceNow");
  if (mins < 60) return t("admin.backgroundTasks.sinceMinutes", { count: mins });
  return t("admin.backgroundTasks.sinceHours", { count: Math.round(mins / 60) });
}

// One task's controls. Confirm-to-act: the first press arms, a second within a
// few seconds stops. A stop kills the ffmpeg in flight, which is not something
// a stray press should do.
export function useBackgroundTaskControl(task: BackgroundTaskView, t: TFunction) {
  const queryClient = useQueryClient();
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
      queryClient.invalidateQueries({ queryKey: BACKGROUND_TASKS_QUERY_KEY });
      toast.success(t(action === "stop" ? "admin.backgroundTasks.stopped" : "admin.backgroundTasks.resumed", { name: label }));
    },
    onError: (err: Error, action) =>
      toast.error(apiErrorMessage(err, t, t(action === "stop" ? "admin.backgroundTasks.stopFailed" : "admin.backgroundTasks.resumeFailed"))),
  });

  const running = task.state === "running";
  return {
    label,
    running,
    since: running ? backgroundTaskSince(t, task.startedAt) : null,
    hasCost: running && task.processes > 0,
    armed,
    pending: control.isPending,
    resume: () => control.mutate("resume"),
    // First press arms, the second stops.
    pressStop: () => {
      if (!armed) { setArmed(true); return; }
      setArmed(false);
      control.mutate("stop");
    },
  };
}
