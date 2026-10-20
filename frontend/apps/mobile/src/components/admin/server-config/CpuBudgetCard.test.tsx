import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";
import { CPU_BUDGET_TASKS } from "@evey/client";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: unknown) => {
      if (o && typeof o === "object" && "label" in o) return `${key}:${String((o as { label: unknown }).label)}`;
      if (o && typeof o === "object" && "percent" in o) return `${key}:${String((o as { percent: unknown }).percent)}`;
      return key;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import { CpuBudgetCard } from "./CpuBudgetCard";

const saveEntry = vi.fn();
const deleteEntry = vi.fn();
beforeEach(() => vi.clearAllMocks());

function renderCard(entries: { key: string; value: string }[] = [], saving = false) {
  return render(<CpuBudgetCard entries={entries} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={saving} />);
}

describe("CpuBudgetCard", () => {
  it("saves background.cpu_percent when a global share is picked", () => {
    renderCard();
    const group = screen.getByTestId("cpu-budget-global");
    fireEvent.click(within(group).getByText("admin.serverConfig.cpuBudget.global50"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "background.cpu_percent", value: "50" });
  });

  it("saves a task's own key when a share is picked for it", () => {
    renderCard();
    const task = CPU_BUDGET_TASKS[0];
    const row = screen.getByTestId(`cpu-budget-${task.task}`);
    fireEvent.click(within(row).getByText("admin.serverConfig.cpuBudget.share:50"));
    expect(saveEntry).toHaveBeenCalledTimes(1);
    expect(saveEntry.mock.calls[0][0].value).toBe("50");
    expect(saveEntry.mock.calls[0][0].key).not.toBe("background.cpu_percent");
  });

  it("deletes the task key when it is set back to follow the global share", () => {
    const task = CPU_BUDGET_TASKS[0];
    renderCard();
    const row = screen.getByTestId(`cpu-budget-${task.task}`);
    fireEvent.click(within(row).getByText("admin.serverConfig.cpuBudget.share:50"));
    const taskKey = saveEntry.mock.calls[0][0].key as string;
    saveEntry.mockClear();

    renderCard([{ key: taskKey, value: "50" }]);
    const rows = screen.getAllByTestId(`cpu-budget-${task.task}`);
    fireEvent.click(within(rows[rows.length - 1]).getByText(/admin\.serverConfig\.cpuBudget\.followGlobal/));
    expect(deleteEntry).toHaveBeenCalledWith(taskKey);
    expect(saveEntry).not.toHaveBeenCalled();
  });

  it("gives the global row and every task row exactly one 'i' button", () => {
    renderCard();
    const infos = (el: HTMLElement) => el.querySelectorAll('[data-testid="setting-info-button"]');
    expect(infos(screen.getByTestId("cpu-budget-global")).length).toBe(1);
    for (const task of CPU_BUDGET_TASKS) {
      expect(infos(screen.getByTestId(`cpu-budget-${task.task}`)).length).toBe(1);
    }
    expect(screen.getAllByTestId("setting-info-button").length).toBe(1 + CPU_BUDGET_TASKS.length);
  });

  it("disables the choices while a write is in flight", () => {
    renderCard([], true);
    const group = screen.getByTestId("cpu-budget-global");
    expect((within(group).getByText("admin.serverConfig.cpuBudget.global50").closest("button") as HTMLButtonElement).disabled).toBe(true);
  });
});
