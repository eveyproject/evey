import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));

import {
  SERVER_SETTING_FIELDS,
  isPositiveGoDuration,
  serverSettingError,
  serverSettingSeed,
  useServerSettingsFormState,
  type ServerSettingField,
} from "./useServerSettingsForm";

const t = (k: string, o?: Record<string, unknown>) => (o ? `${k}:${JSON.stringify(o)}` : k);
const sv = (value: unknown, def: unknown, source: "override" | "default" = "default", extra: Record<string, unknown> = {}) => ({ value, default: def, source, ...extra });

let current: Record<string, unknown>;

function mount(fields: ServerSettingField[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: qc }, children);
  return renderHook(() => useServerSettingsFormState(fields, t), { wrapper });
}

const puts = () => eveyFetch.mock.calls.map((c) => c[0]).filter((r) => r.method === "PUT").map((r) => r.data);

beforeEach(() => {
  vi.clearAllMocks();
  current = {
    loginRateLimit: sv(5, 10, "override"),
    globalRateLimit: sv(300, 300),
    periodicScanInterval: sv("2m", "2m", "default", { requiresRestart: true }),
    passwordRecoveryEnabled: sv(false, false),
    softwareMaxHeight: sv(0, 0, "override"),
    maxConcurrentTranscodes: sv(2, 2),
  };
  eveyFetch.mockImplementation(async (r: { method: string }) => (r.method === "GET" ? current : {}));
});

describe("server settings field rules", () => {
  it("seeds only an override into a text field and the effective value into a toggle", () => {
    expect(serverSettingSeed(SERVER_SETTING_FIELDS.loginRateLimit, sv(5, 10, "override"))).toBe("5");
    expect(serverSettingSeed(SERVER_SETTING_FIELDS.loginRateLimit, sv(10, 10))).toBe("");
    expect(serverSettingSeed(SERVER_SETTING_FIELDS.passwordRecoveryEnabled, sv(true, false))).toBe(true);
    // A stored 0 on a zero-is-default ladder is the default.
    expect(serverSettingSeed(SERVER_SETTING_FIELDS.softwareMaxHeight, sv(0, 0, "override"))).toBe("");
  });

  it("refuses what the server would drop silently", () => {
    expect(serverSettingError(SERVER_SETTING_FIELDS.transcodeCacheMaxGB, "0", t)).toContain("errorNumber");
    expect(serverSettingError(SERVER_SETTING_FIELDS.loginRateLimit, "-3", t)).toContain("errorNumber");
    expect(serverSettingError(SERVER_SETTING_FIELDS.loginRateLimit, "4.5", t)).toContain("errorNumber");
    expect(serverSettingError(SERVER_SETTING_FIELDS.loginRateLimit, "", t)).toBeNull();
    expect(serverSettingError(SERVER_SETTING_FIELDS.periodicScanInterval, "5 minutes", t)).toBe("admin.settingsLayout.errorDuration");
    expect(serverSettingError(SERVER_SETTING_FIELDS.periodicScanInterval, "1h30m", t)).toBeNull();
    expect(isPositiveGoDuration("0s")).toBe(false);
    expect(isPositiveGoDuration("90s")).toBe(true);
  });

  it("leaves the auto-skip intro switch without an explanation, and every other field with one", () => {
    for (const [key, f] of Object.entries(SERVER_SETTING_FIELDS)) {
      if (key === "introAutoSkipEnabled") expect("infoKey" in f).toBe(false);
      else expect((f as { infoKey?: string }).infoKey, key).toBeTruthy();
    }
  });
});

describe("useServerSettingsFormState", () => {
  it("sends only the changed fields, numbers as numbers", async () => {
    const { result } = mount(["loginRateLimit", "globalRateLimit", "passwordRecoveryEnabled"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.field("loginRateLimit").draft).toBe("5");
    expect(result.current.dirty).toBe(false);
    act(() => result.current.field("globalRateLimit").onChange("120"));
    expect(result.current.dirty).toBe(true);
    let ok = false;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(true);
    expect(puts()).toEqual([{ globalRateLimit: 120 }]);
    await waitFor(() => expect(result.current.dirty).toBe(false));
    expect(toast.success).toHaveBeenCalledWith("admin.serverSettings.saved");
  });

  it("blocks an invalid value and sends nothing", async () => {
    const { result } = mount(["periodicScanInterval"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    act(() => result.current.field("periodicScanInterval").onChange("soon"));
    let ok = true;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(false);
    expect(puts()).toEqual([]);
    expect(result.current.field("periodicScanInterval").error).toBe("admin.settingsLayout.errorDuration");
    // Editing again clears the message.
    act(() => result.current.field("periodicScanInterval").onChange("5m"));
    expect(result.current.field("periodicScanInterval").error).toBeUndefined();
  });

  it("keeps an emptied keep-field as it is, and resets an emptied reset-field", async () => {
    current = { ...current, maxConcurrentTranscodes: sv(4, 2, "override") };
    const { result } = mount(["loginRateLimit", "maxConcurrentTranscodes"]);
    await waitFor(() => expect(result.current.field("maxConcurrentTranscodes").draft).toBe("4"));
    act(() => {
      result.current.field("loginRateLimit").onChange("");
      result.current.field("maxConcurrentTranscodes").onChange("");
    });
    await act(async () => {
      await result.current.submit();
    });
    expect(puts()).toEqual([{ maxConcurrentTranscodes: null }]);
    expect(result.current.field("loginRateLimit").draft).toBe("5");
  });

  it("names a restart in the toast when a boot-time setting was saved", async () => {
    const { result } = mount(["periodicScanInterval"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.field("periodicScanInterval").restart).toBe(true);
    act(() => result.current.field("periodicScanInterval").onChange("10m"));
    await act(async () => {
      await result.current.submit();
    });
    expect(toast.success).toHaveBeenCalledWith("admin.transcoding.savedRestart");
  });

  it("never overwrites a field being edited when the data is fetched again", async () => {
    const { result } = mount(["loginRateLimit", "globalRateLimit"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    act(() => result.current.field("globalRateLimit").onChange("77"));
    current = { ...current, loginRateLimit: sv(8, 10, "override"), globalRateLimit: sv(500, 300, "override") };
    act(() => result.current.field("loginRateLimit").reset!.onReset());
    await waitFor(() => expect(result.current.field("loginRateLimit").draft).toBe("8"));
    expect(result.current.field("globalRateLimit").draft).toBe("77");
    expect(puts()).toEqual([{ loginRateLimit: null }]);
  });

  it("offers the ladder with Auto first and labels the default line", async () => {
    const { result } = mount(["softwareMaxHeight"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    const v = result.current.field("softwareMaxHeight");
    expect(v.options?.map((o) => o.value)).toEqual(["", "480", "720", "1080", "1440", "2160"]);
    expect(v.options?.[1].label).toBe("480p");
    expect(v.reset?.overridden).toBe(false);
    expect(v.reset?.defaultText).toContain("admin.settingsLayout.autoMeasured");
  });

  it("greys a setting the demo locks, reset included", async () => {
    current = { ...current, loginRateLimit: sv(5, 10, "override", { locked: true }) };
    const { result } = mount(["loginRateLimit"]);
    await waitFor(() => expect(result.current.data).toBeDefined());
    expect(result.current.field("loginRateLimit").locked).toBe(true);
    expect(result.current.field("loginRateLimit").reset?.disabled).toBe(true);
  });
});
