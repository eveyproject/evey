// A failed GET left the three level pickers on their UNSPECIFIED default and
// showed no error - pressing Save then overwrote the real server-side
// settings with defaults nobody chose.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  Alert: { alert: vi.fn() },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", border: "#333" },
    radii: { md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

const updateMutateSpy = vi.fn();
let getSettingsResult = { data: undefined as unknown, isLoading: false, isError: true, refetch: vi.fn() };
vi.mock("@evey/api", () => ({
  useNotificationServiceGetNotificationSettings: () => getSettingsResult,
  useNotificationServiceUpdateNotificationSettings: () => ({ mutate: updateMutateSpy, isPending: false }),
  apiErrorMessage: () => "error",
}));

import NotificationSettingsScreen from "../../app/(auth)/settings/notifications";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("notification settings - a failed load must not silently overwrite real settings", () => {
  it("shows a retry state instead of the level pickers when the GET fails", () => {
    getSettingsResult = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    render(React.createElement(NotificationSettingsScreen));

    expect(screen.getByLabelText("common.retry")).toBeTruthy();
    // The pickers (and the Save button that could overwrite real settings) are gone.
    expect(screen.queryByText("common.save")).toBeNull();
  });

  it("retries the failed load instead of saving anything", () => {
    const refetch = vi.fn();
    getSettingsResult = { data: undefined, isLoading: false, isError: true, refetch };
    render(React.createElement(NotificationSettingsScreen));

    fireEvent.click(screen.getByLabelText("common.retry"));

    expect(refetch).toHaveBeenCalledOnce();
    expect(updateMutateSpy).not.toHaveBeenCalled();
  });
});
