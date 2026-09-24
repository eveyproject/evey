import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  CPU_BUDGET_GLOBAL_KEY,
  CPU_BUDGET_TASKS,
  cpuBudgetTaskKey,
  readCpuBudget,
  useCpuBudgetForm,
} from "./useCpuBudgetForm";

// The form must show what the server applies: the stored keys first, the
// legacy toggles when they are unset, the defaults otherwise.
describe("readCpuBudget", () => {
  it("defaults to 25 % with every task following the global share", () => {
    const s = readCpuBudget(undefined);
    expect(s.globalPercent).toBe(25);
    for (const { task } of CPU_BUDGET_TASKS) expect(s.taskPercent[task]).toBeNull();
  });

  it("reads the stored shares and ignores out-of-range values like the server", () => {
    const s = readCpuBudget([
      { key: CPU_BUDGET_GLOBAL_KEY, value: "50" },
      { key: cpuBudgetTaskKey("trickplay_generation"), value: "80" },
      { key: cpuBudgetTaskKey("library_scan"), value: "25 %" },
      { key: cpuBudgetTaskKey("intro_detection"), value: "95" },
    ]);
    expect(s.globalPercent).toBe(50);
    expect(s.taskPercent.trickplay_generation).toBe(80);
    expect(s.taskPercent.library_scan).toBe(25);
    expect(s.taskPercent.intro_detection).toBeNull();
  });

  it("caps a global value above the boost back to the default", () => {
    expect(readCpuBudget([{ key: CPU_BUDGET_GLOBAL_KEY, value: "80" }]).globalPercent).toBe(25);
  });

  it("mirrors the legacy toggles until the new keys are set", () => {
    const legacy = readCpuBudget([
      { key: "trickplay.adaptive_cpu", value: "false" },
      { key: "intro_detection.use_all_cores", value: "true" },
    ]);
    expect(legacy.globalPercent).toBe(50);
    expect(legacy.taskPercent.intro_detection).toBe(80);

    const overridden = readCpuBudget([
      { key: "trickplay.adaptive_cpu", value: "false" },
      { key: "intro_detection.use_all_cores", value: "true" },
      { key: CPU_BUDGET_GLOBAL_KEY, value: "25" },
      { key: cpuBudgetTaskKey("intro_detection"), value: "50" },
    ]);
    expect(overridden.globalPercent).toBe(25);
    expect(overridden.taskPercent.intro_detection).toBe(50);
  });
});

describe("useCpuBudgetForm", () => {
  it("writes one key per change and deletes a task key to follow the global share", () => {
    const save = vi.fn();
    const remove = vi.fn();
    const { result } = renderHook(() => useCpuBudgetForm([], { save, remove }));
    result.current.setGlobal(50);
    expect(save).toHaveBeenLastCalledWith(CPU_BUDGET_GLOBAL_KEY, "50");
    result.current.setTask("trickplay_generation", 80);
    expect(save).toHaveBeenLastCalledWith("background.cpu_percent.trickplay_generation", "80");
    result.current.setTask("trickplay_generation", null);
    expect(remove).toHaveBeenLastCalledWith("background.cpu_percent.trickplay_generation");
  });
});
