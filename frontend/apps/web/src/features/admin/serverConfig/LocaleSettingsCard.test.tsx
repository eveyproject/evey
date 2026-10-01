import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { LocaleSettingsCard } from "./LocaleSettingsCard";

const setMutate = vi.fn();
vi.mock("./hooks", () => ({ useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const setLocale = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ toast, useLocaleStore: (sel: (s: { setLocale: typeof setLocale }) => unknown) => sel({ setLocale }) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: { value?: string; time?: string }) => (o?.value ?? o?.time ? `${k}:${o?.value ?? o?.time}` : k),
    i18n: { language: "en" },
  }),
}));
vi.mock("@evey/api", () => ({
  defaultRegionForLocale: (l: string) => (l === "fr" ? "FR" : "US"),
  configDefaultsKey: ["defaults"],
  getConfigDefaults: vi.fn(),
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: { "server.language": "en", "metadata.language": "fr", "server.timezone": "Europe/Paris", now: "2026-10-05T16:42:00Z" } }),
}));
vi.mock("@/features/user/regionList", () => ({ ISO_REGIONS: ["FR", "US", "DE"] }));
vi.mock("@/i18n", () => ({ supportedLanguages: [{ code: "en", label: "English" }, { code: "fr", label: "Français" }] }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => null }));
vi.mock("@/components/ui/Select", () => ({
  Select: ({ id, label, value, onChange, options }: any) =>
    React.createElement(
      "select",
      { id, "aria-label": label, value, onChange: (e: React.ChangeEvent<HTMLSelectElement>) => onChange(e.target.value) },
      options.map((o: any) => React.createElement("option", { key: o.value, value: o.value }, o.label)),
    ),
}));

beforeEach(() => vi.clearAllMocks());

// The three selects use the same short codes the setup wizard persists so a
// stored value matches an option; picking a server language writes the row
// AND mirrors it into the admin's own UI as a "server" pick, never a personal
// one; the region default follows the server language.
describe("LocaleSettingsCard", () => {
  it("matches stored values and writes each row on change, mirroring the server language locally", () => {
    render(<LocaleSettingsCard entries={[{ key: "server.language", value: "fr" }, { key: "server.region", value: "FR" }]} />);
    expect((screen.getByLabelText("admin.serverConfig.serverLanguage") as HTMLSelectElement).value).toBe("fr");
    expect((screen.getByLabelText("admin.serverConfig.serverRegion") as HTMLSelectElement).value).toBe("FR");
    // Each empty choice names the value it resolves to, not "env default".
    expect(screen.getByText("admin.serverConfig.defaultValue:English")).toBeInTheDocument();
    expect(screen.getByText("admin.serverConfig.defaultValue:France")).toBeInTheDocument();
    expect(screen.getByText("admin.serverConfig.defaultValue:Français")).toBeInTheDocument();
    expect(screen.getByText("admin.serverConfig.defaultValue:Europe/Paris")).toBeInTheDocument();
    // The time in force, by the server's clock, in the default zone.
    expect(screen.getByTestId("server-time")).toHaveTextContent(/^admin\.serverConfig\.timezoneNow:0?6:4\d PM$/);
    expect((screen.getByLabelText("admin.serverConfig.metadataLanguage") as HTMLSelectElement).value).toBe("");

    fireEvent.change(screen.getByLabelText("admin.serverConfig.serverLanguage"), { target: { value: "en" } });
    expect(setMutate).toHaveBeenCalledWith({ key: "server.language", value: "en" }, expect.anything());
    (setMutate.mock.calls[0][1] as { onSuccess: () => void }).onSuccess();
    expect(setLocale).toHaveBeenCalledWith("en", "server");
    expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.languageUpdated");

    fireEvent.change(screen.getByLabelText("admin.serverConfig.serverRegion"), { target: { value: "DE" } });
    expect(setMutate).toHaveBeenLastCalledWith({ key: "server.region", value: "DE" }, expect.anything());
    fireEvent.change(screen.getByLabelText("admin.serverConfig.metadataLanguage"), { target: { value: "fr" } });
    expect(setMutate).toHaveBeenLastCalledWith({ key: "metadata.language", value: "fr" }, expect.anything());
  });

  it("resetting the server language to the env default does not rewrite the local pick", () => {
    render(<LocaleSettingsCard entries={[{ key: "server.language", value: "fr" }]} />);
    fireEvent.change(screen.getByLabelText("admin.serverConfig.serverLanguage"), { target: { value: "" } });
    (setMutate.mock.calls[0][1] as { onSuccess: () => void }).onSuccess();
    expect(setLocale).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.languageUpdated");
  });
});
