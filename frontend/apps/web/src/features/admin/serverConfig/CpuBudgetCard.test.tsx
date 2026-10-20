import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
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

// Every share is a segmented control of equal cells (a radio group).
const group = (label: string) => screen.getByRole("radiogroup", { name: label });
const checked = (label: string) => within(group(label)).getByRole("radio", { checked: true }).getAttribute("data-value");
const pick = (label: string, value: string) =>
  fireEvent.click(within(group(label)).getAllByRole("radio").find((r) => r.getAttribute("data-value") === value)!);

// The global share writes one key; a task share writes its own key, and
// "follow the global share" deletes it so the server falls back by itself.
describe("CpuBudgetCard", () => {
  it("defaults to 25 % with every heavy task following the global share", () => {
    render(<CpuBudgetCard entries={undefined} />);
    expect(checked("admin.serverConfig.cpuBudget.globalLabel")).toBe("25");
    const scan = "admin.serverConfig.cpuBudget.tasks.library_scan.label";
    expect(checked(scan)).toBe("");
    const options = within(group(scan)).getAllByRole("radio");
    expect(options[0]).toHaveTextContent("admin.serverConfig.cpuBudget.followGlobal:25");
    expect(options.map((o) => o.getAttribute("data-value"))).toEqual(["", "25", "50", "80"]);
  });

  it("writes the global boost and a task's own share, deletes the key to follow again", () => {
    render(<CpuBudgetCard entries={[{ key: "background.cpu_percent.trickplay_generation", value: "80" }]} />);
    pick("admin.serverConfig.cpuBudget.globalLabel", "50");
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "background.cpu_percent", value: "50" });

    const trickplay = "admin.serverConfig.cpuBudget.tasks.trickplay_generation.label";
    expect(checked(trickplay)).toBe("80");
    pick(trickplay, "50");
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "background.cpu_percent.trickplay_generation", value: "50" });
    pick(trickplay, "");
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
    expect(checked("admin.serverConfig.cpuBudget.globalLabel")).toBe("50");
    expect(checked("admin.serverConfig.cpuBudget.tasks.intro_detection.label")).toBe("80");
  });

  it("freezes every choice and shows the spinner while a write is in flight", () => {
    deleteEntry.isPending = true;
    render(<CpuBudgetCard entries={[]} />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    expect(screen.getAllByRole("radiogroup")).toHaveLength(5);
    for (const r of screen.getAllByRole("radio")) expect(r).toBeDisabled();
  });
});
