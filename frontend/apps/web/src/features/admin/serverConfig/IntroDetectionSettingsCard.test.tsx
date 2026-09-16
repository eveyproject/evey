import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { IntroDetectionSettingsCard } from "./IntroDetectionSettingsCard";

const setEntry = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
vi.mock("./hooks", () => ({ useSetConfigEntry: () => setEntry }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => <span data-testid="spinner" /> }));

beforeEach(() => {
  setEntry.mutate.mockClear();
  setEntry.isPending = false;
});

const box = (label: string) => screen.getByLabelText(label) as HTMLInputElement;

// Each toggle writes its own config key as the strings "true"/"false"; the
// provider status block (and the TIDB submit toggle, which needs a key)
// exists only while the providers are on; every toggle freezes while a write
// is in flight so two writes cannot race.
describe("IntroDetectionSettingsCard", () => {
  it("writes each toggle to its own key and hides the provider block when off", () => {
    render(<IntroDetectionSettingsCard entries={[]} />);
    expect(box("admin.serverConfig.introDetectionUseAllCores").checked).toBe(false);
    expect(box("admin.serverConfig.introProvidersEnabled").checked).toBe(false);
    expect(screen.queryByText("admin.serverConfig.introProvidersTidbStatus")).toBeNull();

    fireEvent.click(box("admin.serverConfig.introDetectionUseAllCores"));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "intro_detection.use_all_cores", value: "true" });
    fireEvent.click(box("admin.serverConfig.introProvidersEnabled"));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "intro_providers.enabled", value: "true" });
  });

  it("with providers on and no key: anonymous status, no submit toggle", () => {
    render(<IntroDetectionSettingsCard entries={[{ key: "intro_providers.enabled", value: "true" }]} />);
    expect(screen.getByText("admin.serverConfig.introProvidersTidbAnonymous")).toBeInTheDocument();
    expect(screen.queryByLabelText("admin.serverConfig.introProvidersTidbSubmit")).toBeNull();
    fireEvent.click(box("admin.serverConfig.introProvidersEnabled"));
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "intro_providers.enabled", value: "false" });
  });

  it("with a TIDB key: authenticated status and the submit toggle writes its key", () => {
    render(
      <IntroDetectionSettingsCard
        entries={[
          { key: "intro_providers.enabled", value: "true" },
          { key: "api.tidb_key", value: "••••" },
          { key: "intro_providers.tidb_submit_enabled", value: "true" },
        ]}
      />,
    );
    expect(screen.getByText("admin.serverConfig.introProvidersTidbAuthed")).toBeInTheDocument();
    const submit = box("admin.serverConfig.introProvidersTidbSubmit");
    expect(submit.checked).toBe(true);
    fireEvent.click(submit);
    expect(setEntry.mutate).toHaveBeenLastCalledWith({ key: "intro_providers.tidb_submit_enabled", value: "false" });
  });

  it("freezes every toggle while a write is in flight", () => {
    setEntry.isPending = true;
    render(<IntroDetectionSettingsCard entries={[{ key: "intro_providers.enabled", value: "true" }, { key: "api.tidb_key", value: "k" }]} />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    for (const cb of screen.getAllByRole("checkbox") as HTMLInputElement[]) expect(cb.disabled).toBe(true);
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);
  });
});
