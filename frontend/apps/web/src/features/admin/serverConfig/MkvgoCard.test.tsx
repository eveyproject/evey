import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { MkvgoCard } from "./MkvgoCard";

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

// Each setting is a segmented control (a radio group): the checked option's
// value is what the card read.
const checked = (label: string) =>
  within(screen.getByRole("radiogroup", { name: label })).getByRole("radio", { checked: true }).getAttribute("data-value");

// The ingest plan is "off" when the key is absent (empty value), the
// playability shadow is ON unless the stored value is exactly "off"; each
// change writes its own key and confirms with a toast once the server has it.
describe("MkvgoCard", () => {
  it("reads defaults: no ingest plan, shadow on", () => {
    render(<MkvgoCard entries={undefined} />);
    expect(checked("admin.serverConfig.ingestServingPlanLabel")).toBe("");
    expect(checked("admin.serverConfig.playabilityShadowLabel")).toBe("on");
    expect(screen.queryByTestId("spinner")).toBeNull();
  });

  it("reflects stored values and writes each change to its key, toasting on success", () => {
    render(<MkvgoCard entries={[{ key: "library.ingest_serving_plan", value: "plan" }, { key: "transcoding.playability_shadow", value: "off" }]} />);
    expect(checked("admin.serverConfig.ingestServingPlanLabel")).toBe("plan");
    expect(checked("admin.serverConfig.playabilityShadowLabel")).toBe("off");

    fireEvent.click(screen.getByRole("radio", { name: "admin.serverConfig.ingestServingPlanPlanRepair" }));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "library.ingest_serving_plan", value: "plan+repair" }, expect.any(Object));
    fireEvent.click(screen.getByRole("radio", { name: "admin.serverConfig.playabilityShadowOn" }));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "transcoding.playability_shadow", value: "on" }, expect.any(Object));

    expect(toast.success).not.toHaveBeenCalled();
    setEntry.mutate.mock.calls[1][1].onSuccess();
    expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.configUpdated");
  });

  it("shows the spinner while a write is in flight", () => {
    setEntry.isPending = true;
    render(<MkvgoCard entries={[]} />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });
});
