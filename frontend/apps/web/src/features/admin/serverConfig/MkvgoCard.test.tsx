import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
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

const select = (label: string) => screen.getByLabelText(label) as HTMLSelectElement;

// The ingest plan is "off" when the key is absent (empty value), the
// playability shadow is ON unless the stored value is exactly "off"; each
// change writes its own key and confirms with a toast once the server has it.
describe("MkvgoCard", () => {
  it("reads defaults: no ingest plan, shadow on", () => {
    render(<MkvgoCard entries={undefined} />);
    expect(select("admin.serverConfig.ingestServingPlanLabel").value).toBe("");
    expect(select("admin.serverConfig.playabilityShadowLabel").value).toBe("on");
    expect(screen.queryByTestId("spinner")).toBeNull();
  });

  it("reflects stored values and writes each change to its key, toasting on success", () => {
    render(<MkvgoCard entries={[{ key: "library.ingest_serving_plan", value: "plan" }, { key: "transcoding.playability_shadow", value: "off" }]} />);
    expect(select("admin.serverConfig.ingestServingPlanLabel").value).toBe("plan");
    expect(select("admin.serverConfig.playabilityShadowLabel").value).toBe("off");

    fireEvent.change(select("admin.serverConfig.ingestServingPlanLabel"), { target: { value: "plan+repair" } });
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "library.ingest_serving_plan", value: "plan+repair" }, expect.any(Object));
    fireEvent.change(select("admin.serverConfig.playabilityShadowLabel"), { target: { value: "on" } });
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
