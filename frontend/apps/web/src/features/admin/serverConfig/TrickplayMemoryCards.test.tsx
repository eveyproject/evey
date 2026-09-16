import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TrickplaySettingsCard } from "./TrickplaySettingsCard";
import { MemoryProfileCard } from "./MemoryProfileCard";

const setEntry = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
vi.mock("./hooks", () => ({ useSetConfigEntry: () => setEntry }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => <span data-testid="spinner" /> }));

beforeEach(() => {
  vi.clearAllMocks();
  setEntry.isPending = false;
});

const box = (label: string) => screen.getByLabelText(label) as HTMLInputElement;

// Both trickplay flags default to ON (only an explicit "false" turns one
// off); the adaptive-CPU flag is meaningless while trickplay itself is off,
// so it is frozen then; each toggle writes its own key.
describe("TrickplaySettingsCard", () => {
  it("defaults both on and writes each key", () => {
    render(<TrickplaySettingsCard entries={undefined} />);
    expect(box("admin.serverConfig.trickplayEnabled").checked).toBe(true);
    const cpu = box("admin.serverConfig.trickplayAdaptiveCpu");
    expect(cpu.checked).toBe(true);
    expect(cpu.disabled).toBe(false);
    fireEvent.click(cpu);
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "trickplay.adaptive_cpu", value: "false" });
    fireEvent.click(box("admin.serverConfig.trickplayEnabled"));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "trickplay.enabled", value: "false" });
  });

  it("freezes adaptive CPU while trickplay is off, and everything in flight", () => {
    render(<TrickplaySettingsCard entries={[{ key: "trickplay.enabled", value: "false" }]} />);
    expect(box("admin.serverConfig.trickplayEnabled").checked).toBe(false);
    expect(box("admin.serverConfig.trickplayAdaptiveCpu").disabled).toBe(true);
    fireEvent.click(box("admin.serverConfig.trickplayEnabled"));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "trickplay.enabled", value: "true" });
  });

  it("shows the spinner and disables both toggles during a write", () => {
    setEntry.isPending = true;
    render(<TrickplaySettingsCard entries={[]} />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    expect(box("admin.serverConfig.trickplayEnabled").disabled).toBe(true);
    expect(box("admin.serverConfig.trickplayAdaptiveCpu").disabled).toBe(true);
  });
});

// Absent = Auto (empty value); a change writes the profile and confirms
// once the server acknowledged it.
describe("MemoryProfileCard", () => {
  it("reads Auto by default and the stored profile otherwise", () => {
    const { unmount } = render(<MemoryProfileCard entries={[]} />);
    expect((screen.getByLabelText("admin.serverConfig.memoryProfileLabel") as HTMLSelectElement).value).toBe("");
    unmount();
    render(<MemoryProfileCard entries={[{ key: "server.memory_profile", value: "low" }]} />);
    expect((screen.getByLabelText("admin.serverConfig.memoryProfileLabel") as HTMLSelectElement).value).toBe("low");
  });

  it("writes the picked profile and toasts on success", () => {
    render(<MemoryProfileCard entries={[]} />);
    fireEvent.change(screen.getByLabelText("admin.serverConfig.memoryProfileLabel"), { target: { value: "normal" } });
    expect(setEntry.mutate).toHaveBeenCalledWith({ key: "server.memory_profile", value: "normal" }, expect.any(Object));
    setEntry.mutate.mock.calls[0][1].onSuccess();
    expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.configUpdated");
  });
});
