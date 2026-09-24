// The admin's background CPU budget (Server settings): one global share
// every background task draws from, and a share of its own for the heavy
// tasks. Mirrors internal/app/config/cpu_budget.go: the keys, the accepted
// values and the legacy toggles the server still honours until the new keys
// are set. The web and the phone render the same list from here, so a task
// added to one is visible on the other.

export const CPU_BUDGET_GLOBAL_KEY = "background.cpu_percent";
export const CPU_BUDGET_TASK_KEY_PREFIX = "background.cpu_percent.";

export const CPU_BUDGET_DEFAULT_GLOBAL = 25;
export const CPU_BUDGET_GLOBAL_CHOICES = [25, 50] as const;
export const CPU_BUDGET_TASK_CHOICES = [25, 50, 80] as const;

// The two toggles the budget replaced. The server maps them the same way when
// the new keys are unset; the form shows the admin what the server does.
const LEGACY_TRICKPLAY_ADAPTIVE_CPU_KEY = "trickplay.adaptive_cpu";
const LEGACY_INTRO_USE_ALL_CORES_KEY = "intro_detection.use_all_cores";

// The heavy tasks an admin may give a share of their own, by their bgtasks
// name (what the dashboard's background-tasks panel shows).
export const CPU_BUDGET_TASKS = [
  { task: "library_scan", labelKey: "admin.serverConfig.cpuBudget.tasks.library_scan.label", descKey: "admin.serverConfig.cpuBudget.tasks.library_scan.desc" },
  { task: "trickplay_generation", labelKey: "admin.serverConfig.cpuBudget.tasks.trickplay_generation.label", descKey: "admin.serverConfig.cpuBudget.tasks.trickplay_generation.desc" },
  { task: "intro_detection", labelKey: "admin.serverConfig.cpuBudget.tasks.intro_detection.label", descKey: "admin.serverConfig.cpuBudget.tasks.intro_detection.desc" },
  { task: "complexity_classifier", labelKey: "admin.serverConfig.cpuBudget.tasks.complexity_classifier.label", descKey: "admin.serverConfig.cpuBudget.tasks.complexity_classifier.desc" },
] as const;

export type CpuBudgetTask = (typeof CPU_BUDGET_TASKS)[number]["task"];

export function cpuBudgetTaskKey(task: string): string {
  return CPU_BUDGET_TASK_KEY_PREFIX + task;
}

type Entry = { key?: string; value?: string };

function parsePercent(value: string | undefined, max: number): number | null {
  if (value === undefined) return null;
  const n = Number.parseInt(value.replace("%", "").trim(), 10);
  if (!Number.isFinite(n) || n < 1 || n > max) return null;
  return n;
}

export interface CpuBudgetState {
  // The global share in force (the stored value, else the legacy trickplay
  // toggle's meaning, else the default).
  globalPercent: number;
  // A task's own share, null when it follows the global one.
  taskPercent: Record<string, number | null>;
}

// readCpuBudget derives the budget the server applies from the raw entries,
// legacy toggles included, so the form never shows "25 %" while the server
// runs at 50 % because of an old checkbox.
export function readCpuBudget(entries: Entry[] | undefined): CpuBudgetState {
  const values = new Map<string, string>();
  for (const e of entries ?? []) if (e.key) values.set(e.key, e.value ?? "");
  let globalPercent = parsePercent(values.get(CPU_BUDGET_GLOBAL_KEY), 50);
  if (globalPercent === null) {
    globalPercent = values.get(LEGACY_TRICKPLAY_ADAPTIVE_CPU_KEY) === "false" ? 50 : CPU_BUDGET_DEFAULT_GLOBAL;
  }
  const taskPercent: Record<string, number | null> = {};
  for (const { task } of CPU_BUDGET_TASKS) {
    let pct = parsePercent(values.get(cpuBudgetTaskKey(task)), 80);
    if (pct === null && task === "intro_detection" && !values.has(cpuBudgetTaskKey(task)) && values.get(LEGACY_INTRO_USE_ALL_CORES_KEY) === "true") {
      pct = 80;
    }
    taskPercent[task] = pct;
  }
  return { globalPercent, taskPercent };
}

export interface CpuBudgetWrites {
  save: (key: string, value: string) => void;
  remove: (key: string) => void;
}

// useCpuBudgetForm is the card's logic: read the state, write one key per
// change. Choosing "follow the global share" deletes the task key, so the
// raw entries table stays clean and the server falls back by itself.
export function useCpuBudgetForm(entries: Entry[] | undefined, writes: CpuBudgetWrites) {
  const state = readCpuBudget(entries);
  return {
    ...state,
    setGlobal: (percent: number) => writes.save(CPU_BUDGET_GLOBAL_KEY, String(percent)),
    setTask: (task: string, percent: number | null) =>
      percent === null ? writes.remove(cpuBudgetTaskKey(task)) : writes.save(cpuBudgetTaskKey(task), String(percent)),
  };
}
