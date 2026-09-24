import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { CpuBudgetCard } from "./CpuBudgetCard";

const setEntry = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
const deleteEntry = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
vi.mock("./hooks", () => ({ useSetConfigEntry: () => setEntry, useDeleteConfigEntry: () => deleteEntry }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o?.percent !== undefined ? `${k}:${o.percent}` : k) }),
}));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => <span data-testid="spinner" /> }));

beforeEach(() => {
  vi.clearAllMocks();
  setEntry.isPending = false;
  deleteEntry.isPending = false;
});

const select = (label: string) => screen.getByLabelText(label) as HTMLSelectElement;

// The global share writes one key; a task share writes its own key, and
// "follow the global share" deletes it so the server falls back by itself.
describe("CpuBudgetCard", () => {
  it("defaults to 25 % with every heavy task following the global share", () => {
    render(<CpuBudgetCard entries={undefined} />);
    expect(select("admin.serverConfig.cpuBudget.globalLabel").value).toBe("25");
    const scan = select("admin.serverConfig.cpuBudget.tasks.library_scan.label");
    expect(scan.value).toBe("");
    expect(scan.options[0].text).toBe("admin.serverConfig.cpuBudget.followGlobal:25");
    expect(Array.from(scan.options).map((o) => o.value)).toEqual(["", "25", "50", "80"]);
  });

  it("writes the global boost and a task's own share, deletes the key to follow again", () => {
    render(<CpuBudgetCard entries={[{ key: "background.cpu_percent.trickplay_generation", value: "80" }]} />);
    fireEvent.change(select("admin.serverConfig.cpuBudget.globalLabel"), { target: { value: "50" } });
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "background.cpu_percent", value: "50" });

    const trickplay = select("admin.serverConfig.cpuBudget.tasks.trickplay_generation.label");
    expect(trickplay.value).toBe("80");
    fireEvent.change(trickplay, { target: { value: "50" } });
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "background.cpu_percent.trickplay_generation", value: "50" });
    fireEvent.change(trickplay, { target: { value: "" } });
    expect(deleteEntry.mutate).toHaveBeenLastCalledWith("background.cpu_percent.trickplay_generation");
  });

  it("shows what the legacy toggles still mean to the server", () => {
    render(
      <CpuBudgetCard
        entries={[
          { key: "trickplay.adaptive_cpu", value: "false" },
          { key: "intro_detection.use_all_cores", value: "true" },
        ]}
      />,
    );
    expect(select("admin.serverConfig.cpuBudget.globalLabel").value).toBe("50");
    expect(select("admin.serverConfig.cpuBudget.tasks.intro_detection.label").value).toBe("80");
  });

  it("freezes every select and shows the spinner while a write is in flight", () => {
    deleteEntry.isPending = true;
    render(<CpuBudgetCard entries={[]} />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    for (const s of screen.getAllByRole("combobox") as HTMLSelectElement[]) expect(s.disabled).toBe(true);
    expect(screen.getAllByRole("combobox")).toHaveLength(5);
  });
});
