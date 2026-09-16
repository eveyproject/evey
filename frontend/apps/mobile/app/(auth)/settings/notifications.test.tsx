import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// Three level pickers seeded from the server; Save sends all three; and a
// failed GET must never be saved over the real settings (the local defaults
// would overwrite them), so that state shows a retry and no Save at all.
const alerts = vi.hoisted(() => ({ calls: [] as string[][] }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  Alert: { alert: (...a: string[]) => { alerts.calls.push(a); } },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const api = vi.hoisted(() => ({
  get: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  opts: null as any,
  mutate: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  useNotificationServiceGetNotificationSettings: () => api.get,
  useNotificationServiceUpdateNotificationSettings: (o: any) => {
    api.opts = o.mutation;
    return { mutate: api.mutate, isPending: false };
  },
  apiErrorMessage: (e: Error) => e.message,
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333" }, radii: { md: 8 }, spacing: { xs: 4, md: 16, lg: 24, xl: 32 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import NotificationSettingsScreen from "./notifications";

// Each section lists the four levels in the same order; the tick marks the
// chosen one.
const rowsOf = (section: number, level: string) =>
  screen.getAllByText(`auth.notificationSettings.${level}`)[section].closest("button") as HTMLButtonElement;

beforeEach(() => {
  vi.clearAllMocks();
  alerts.calls.length = 0;
  api.get = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("notification settings screen (mobile)", () => {
  it("seeds the pickers from the server and saves all three levels", () => {
    api.get.data = { movieLevel: "NOTIFICATION_LEVEL_ALL", seriesLevel: "NOTIFICATION_LEVEL_DISABLED", musicLevel: "NOTIFICATION_LEVEL_RELEVANT" };
    render(<NotificationSettingsScreen />);
    expect(rowsOf(0, "levelAll").textContent).toContain("✓");
    expect(rowsOf(1, "levelDisabled").textContent).toContain("✓");
    expect(rowsOf(2, "levelRelevant").textContent).toContain("✓");
    expect(rowsOf(0, "levelDisabled").textContent).not.toContain("✓");

    fireEvent.click(rowsOf(2, "levelImportant"));
    expect(rowsOf(2, "levelImportant").textContent).toContain("✓");
    expect(rowsOf(2, "levelRelevant").textContent).not.toContain("✓");
    fireEvent.click(screen.getByText("common.save"));
    expect(api.mutate).toHaveBeenCalledWith({
      data: { movieLevel: "NOTIFICATION_LEVEL_ALL", seriesLevel: "NOTIFICATION_LEVEL_DISABLED", musicLevel: "NOTIFICATION_LEVEL_HIGHLY_RELEVANT" },
    });
    api.opts.onSuccess();
    expect(alerts.calls.at(-1)).toEqual(["auth.notificationSettings.saved"]);
    api.opts.onError(new Error("503"));
    expect(alerts.calls.at(-1)).toEqual(["auth.notificationSettings.saveError", "503"]);
  });

  it("never offers to save over settings it could not read", () => {
    api.get.isError = true;
    render(<NotificationSettingsScreen />);
    expect(screen.queryByText("common.save")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.get.refetch).toHaveBeenCalledTimes(1);
    expect(api.mutate).not.toHaveBeenCalled();
  });

  it("spins while loading", () => {
    api.get.isLoading = true;
    render(<NotificationSettingsScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("common.save")).toBeNull();
  });
});
