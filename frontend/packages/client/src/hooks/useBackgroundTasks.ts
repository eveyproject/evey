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
  // The cores the task may hold under the admin's CPU budget (0: no budget)
  // and what it holds right now.
  cpuSlots?: number;
  cpuSlotsHeld?: number;
  // The batch in progress when the worker announced it: items planned and done.
  batchTotal?: number;
  batchDone?: number;
  // The whole library's progress for this task, when it counts it.
  coverage?: BackgroundTaskCoverage;
}

// Mirrors bgtasks.Coverage: items and media duration done out of the total,
// and the remaining time at the pace of the last 30 min (absent until known,
// and while nothing moves).
export interface BackgroundTaskCoverage {
  doneItems: number;
  totalItems: number;
  doneSeconds: number;
  totalSeconds: number;
  etaSeconds?: number;
}

export interface CoverageProgress {
  ratio: number;
  remainingItems: number;
  etaSeconds: number | null;
  complete: boolean;
}

// coverageProgress weighs by media duration when known (a 3 h film is more
// work than a 20 min episode), by items otherwise.
export function coverageProgress(c: BackgroundTaskCoverage | undefined): CoverageProgress | null {
  if (!c || c.totalItems <= 0) return null;
  const raw = c.totalSeconds > 0 ? c.doneSeconds / c.totalSeconds : c.doneItems / c.totalItems;
  const remainingItems = Math.max(0, c.totalItems - c.doneItems);
  return {
    ratio: Math.min(1, Math.max(0, raw)),
    remainingItems,
    etaSeconds: remainingItems > 0 ? (c.etaSeconds ?? null) : null,
    complete: remainingItems === 0,
  };
}

// overallCoverage is every counted task together. The tasks run side by side,
// so the whole is done when the slowest is: its ETA, unknown while any
// unfinished task has none yet.
export function overallCoverage(tasks: BackgroundTaskView[]): CoverageProgress | null {
  const counted = tasks.map((task) => task.coverage).filter((c): c is BackgroundTaskCoverage => !!c && c.totalItems > 0);
  if (counted.length === 0) return null;
  let done = 0;
  let total = 0;
  let remainingItems = 0;
  let eta: number | null = 0;
  for (const c of counted) {
    const p = coverageProgress(c)!;
    done += c.totalSeconds > 0 ? c.doneSeconds : 0;
    total += c.totalSeconds;
    remainingItems += p.remainingItems;
    if (!p.complete) eta = eta === null || p.etaSeconds === null ? null : Math.max(eta, p.etaSeconds);
  }
  return {
    ratio: total > 0 ? Math.min(1, done / total) : 0,
    remainingItems,
    etaSeconds: remainingItems > 0 ? eta : null,
    complete: remainingItems === 0,
  };
}

// formatEta is a rough duration: an estimate, not a countdown.
export function formatEta(t: TFunction, seconds: number): string {
  const mins = seconds / 60;
  if (mins < 1) return t("admin.backgroundTasks.etaUnderMinute");
  if (mins < 90) return t("admin.backgroundTasks.etaMinutes", { count: Math.round(mins) });
  const hours = mins / 60;
  if (hours < 48) return t("admin.backgroundTasks.etaHours", { count: Math.round(hours) });
  return t("admin.backgroundTasks.etaDays", { count: Math.round(hours / 24) });
}

// coverageLine is the one sentence both apps print under a task or the whole.
export function coverageLine(t: TFunction, p: CoverageProgress): string {
  if (p.complete) return t("admin.backgroundTasks.upToDate");
  const parts = [
    t("admin.backgroundTasks.libraryProgress", { percent: Math.floor(p.ratio * 100) }),
    t("admin.backgroundTasks.remaining", { count: p.remainingItems }),
    p.etaSeconds !== null ? formatEta(t, p.etaSeconds) : t("admin.backgroundTasks.etaUnknown"),
  ];
  return parts.join(" · ");
}

// batchProgress is the share of the announced batch that is done, null when
// the worker announced nothing (event-driven one-offs, idle).
export function batchProgress(task: BackgroundTaskView): { done: number; total: number; ratio: number } | null {
  const total = task.batchTotal ?? 0;
  if (total <= 0) return null;
  const done = Math.min(task.batchDone ?? 0, total);
  return { done, total, ratio: done / total };
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
    overall: overallCoverage(tasks),
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
    // Short forks finish between two polls: a task can cost CPU with no
    // process alive at the instant of the snapshot.
    hasCost: running && (task.processes > 0 || task.cpuPercent > 0),
    progress: batchProgress(task),
    coverage: coverageProgress(task.coverage),
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
