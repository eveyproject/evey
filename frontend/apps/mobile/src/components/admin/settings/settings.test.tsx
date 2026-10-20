import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o && "label" in o ? `${k}:${String(o.label)}` : k) }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
  apiErrorMessage: (e: Error) => e.message,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({ ...(await importOriginal<typeof import("@evey/store")>()), toast }));

import { InfoButton } from "./InfoButton";
import { SettingRow } from "./SettingRow";
import { OptionGroup, optionGroupLayout } from "./OptionGroup";
import { ServerSettingsGroup } from "./ServerSettingsGroup";

const text = (el: Element | null) => el?.textContent ?? "";
const isDisabled = (el: Element | null) => (el as HTMLButtonElement | null)?.disabled === true;
const styleOf = (el: Element) => JSON.parse(el.getAttribute("data-style") ?? "{}") as Record<string, unknown>;
const sv = (value: unknown, def: unknown, source: "override" | "default" = "default", extra: Record<string, unknown> = {}) => ({ value, default: def, source, ...extra });

beforeEach(() => vi.clearAllMocks());

describe("InfoButton", () => {
  it("is a named button that opens the explanation and closes it again", () => {
    render(<InfoButton label="Server name" info="The name people see." />);
    const btn = screen.getByRole("button", { name: "admin.settingsLayout.moreInfo:Server name" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(btn);
    const dialog = screen.getByRole("dialog");
    expect(text(within(dialog).getByRole("heading"))).toContain("Server name");
    expect(text(dialog)).toContain("The name people see.");
    fireEvent.click(within(dialog).getByTestId("setting-info-close"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("SettingRow", () => {
  it("shows the info, the restart badge, the hint, the error and the default line", () => {
    render(
      <SettingRow label="Watcher" info="Explains" hint="A hint" error="Bad" restart reset={{ overridden: false, defaultText: "On", onReset: vi.fn() }}>
        <span>control</span>
      </SettingRow>,
    );
    expect(screen.getByRole("button", { name: /moreInfo:Watcher/ })).toBeTruthy();
    expect(text(screen.getByTestId("setting-restart-badge"))).toContain("admin.serverConfig.appliedOnRestart");
    expect(screen.getByText("A hint")).toBeTruthy();
    expect(text(screen.getByTestId("setting-error"))).toContain("Bad");
    expect(text(screen.getByTestId("setting-using-default"))).toContain("admin.serverSettings.usingDefault On");
  });

  it("offers a Reset once overridden, and no 'i' when no info is given", () => {
    const onReset = vi.fn();
    render(
      <SettingRow label="Bare" reset={{ overridden: true, onReset }} kind="toggle">
        <span>control</span>
      </SettingRow>,
    );
    expect(screen.queryByRole("button", { name: /moreInfo/ })).toBeNull();
    fireEvent.click(screen.getByTestId("setting-reset"));
    expect(onReset).toHaveBeenCalled();
  });
});

describe("OptionGroup", () => {
  const short = [
    { value: "", label: "Auto" },
    { value: "normal", label: "Normal" },
    { value: "low", label: "Low" },
  ];

  it("keeps short labels in one row and long ones one per line, never a ragged grid", () => {
    expect(optionGroupLayout(358, ["Auto", "Normal", "Low"])).toBe("row");
    expect(optionGroupLayout(358, ["Auto (recommended)", "Normal", "Low"])).toBe("column");
    expect(optionGroupLayout(358, ["Auto (measured)", "480p", "720p", "1080p", "1440p", "2160p"])).toBe("column");
    // A label seen wrapping at this width wins over the estimate; a new width tries again.
    expect(optionGroupLayout(358, ["Auto", "Normal", "Low"], 358)).toBe("column");
    expect(optionGroupLayout(600, ["Auto", "Normal", "Low"], 358)).toBe("row");
  });

  it("gives every option of a row the same share of the width", () => {
    render(<OptionGroup label="Profile" options={short} value="normal" onChange={vi.fn()} testID="og" />);
    const radios = within(screen.getByRole("radiogroup", { name: "Profile" })).getAllByRole("radio");
    expect(radios.map((r) => r.textContent)).toEqual(["Auto", "Normal", "Low"]);
    for (const r of radios) expect(styleOf(r)).toMatchObject({ flexGrow: 1, flexShrink: 1, flexBasis: 0 });
    expect(text(screen.getByRole("radio", { checked: true }))).toContain("Normal");
  });

  it("reports a pick, ignores the checked option and a disabled group", () => {
    const onChange = vi.fn();
    const { rerender } = render(<OptionGroup label="Profile" options={short} value="" onChange={onChange} />);
    fireEvent.click(screen.getByText("Low"));
    fireEvent.click(screen.getByText("Auto"));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith("low");
    rerender(<OptionGroup label="Profile" options={short} value="" onChange={onChange} disabled />);
    for (const r of screen.getAllByRole("radio")) expect(isDisabled(r)).toBe(true);
  });

  it("shows a stored value no option offers instead of hiding it", () => {
    render(<OptionGroup label="Height" options={short} value="900" onChange={vi.fn()} formatUnknown={(v) => `${v}p`} />);
    expect(text(screen.getByRole("radio", { checked: true }))).toContain("900p");
  });
});

describe("ServerSettingsGroup (mobile)", () => {
  let current: Record<string, unknown>;
  function mount() {
    eveyFetch.mockImplementation(async (r: { method: string }) => (r.method === "GET" ? current : {}));
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <ServerSettingsGroup title="Scan" fields={["libraryWatcherEnabled", "periodicScanInterval"]} saveTestID="save-scan" />
        <ServerSettingsGroup title="Skip" fields={["skipPreviewEnabled", "introAutoSkipEnabled"]} saveTestID="save-skip" />
      </QueryClientProvider>,
    );
  }
  const puts = () => eveyFetch.mock.calls.map((c) => c[0]).filter((r) => r.method === "PUT").map((r) => r.data);

  beforeEach(() => {
    current = {
      libraryWatcherEnabled: sv(true, true, "default", { requiresRestart: true }),
      periodicScanInterval: sv("2m", "2m", "default", { requiresRestart: true }),
      skipPreviewEnabled: sv(false, false),
      introAutoSkipEnabled: sv(false, false),
    };
  });

  it("saves only the field changed in its own group", async () => {
    mount();
    const toggle = await screen.findByTestId("skip-preview-toggle");
    expect(isDisabled(screen.getByTestId("save-skip"))).toBe(true);
    fireEvent.click(toggle);
    fireEvent.click(screen.getByTestId("save-skip"));
    await waitFor(() => expect(puts()).toEqual([{ skipPreviewEnabled: true }]));
    expect(isDisabled(screen.getByTestId("save-scan"))).toBe(true);
  });

  it("refuses an invalid duration inline and sends nothing", async () => {
    mount();
    fireEvent.change(await screen.findByTestId("scan-interval-input"), { target: { value: "often" } });
    fireEvent.click(screen.getByTestId("save-scan"));
    expect(text(await screen.findByTestId("setting-error"))).toContain("admin.settingsLayout.errorDuration");
    expect(puts()).toEqual([]);
  });

  it("gives every row its 'i' except the auto-skip intro switch", async () => {
    mount();
    await screen.findByTestId("setting-row-introAutoSkipEnabled");
    for (const key of ["libraryWatcherEnabled", "periodicScanInterval", "skipPreviewEnabled"]) {
      expect(within(screen.getByTestId(`setting-row-${key}`)).getAllByRole("button", { name: /moreInfo/ }).length, key).toBe(1);
    }
    expect(within(screen.getByTestId("setting-row-introAutoSkipEnabled")).queryByRole("button", { name: /moreInfo/ })).toBeNull();
  });

  it("greys a demo-locked setting", async () => {
    current = { ...current, skipPreviewEnabled: sv(false, false, "default", { locked: true }) };
    mount();
    expect(isDisabled(await screen.findByTestId("skip-preview-toggle"))).toBe(true);
  });
});
