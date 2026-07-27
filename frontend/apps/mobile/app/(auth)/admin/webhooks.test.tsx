import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-checked": !!value,
      onClick: () => onValueChange?.(!value),
    }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "webhooks-list" },
      data.length === 0
        ? ListEmptyComponent ?? null
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
    ),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Alert: { alert: (_title: string, _msg: string, buttons: any[]) => buttons?.[1]?.onPress?.() },
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router ------------------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", error: "#e00", border: "#333",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store -------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// --- @tanstack/react-query -----------------------------------------------------------------
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

// --- @evey/api -----------------------------------------------------------------------------
// The screen calls useWebhookServiceUpdateWebhook twice: once for the edit-form
// submit ("updateWebhook") and once for the row-level enabled Switch
// ("toggleEnabled"). Track every mutate() call across both instances so a test
// can inspect exactly what payload the Switch handler sent.
const mutateCalls: any[] = [];
vi.mock("@evey/api", () => ({
  useWebhookServiceListWebhooks: () => ({
    data: {
      webhooks: [
        { id: "wh1", name: "My Hook", url: "https://example.com/hook", events: ["*"], secret: "s3cr3t-value", enabled: true },
      ],
    },
    isLoading: false,
    refetch: vi.fn(),
  }),
  useWebhookServiceCreateWebhook: (_opts: any) => ({ mutate: vi.fn(), isPending: false }),
  useWebhookServiceDeleteWebhook: (_opts: any) => ({ mutate: vi.fn(), isPending: false }),
  useWebhookServiceUpdateWebhook: (_opts: any) => ({
    mutate: (...args: any[]) => mutateCalls.push(args),
    isPending: false,
  }),
  getWebhookServiceListWebhooksQueryKey: () => ["webhooks"],
  eveyFetch: vi.fn(),
}));

// -------------------------------------------------------------------------------------
import AdminWebhooksScreen from "./webhooks";

beforeEach(() => {
  vi.clearAllMocks();
  mutateCalls.length = 0;
});

describe("AdminWebhooksScreen - toggle enabled", () => {
  it("includes the existing secret in the toggle payload so it survives an enable/disable flip", () => {
    render(<AdminWebhooksScreen />);

    const toggle = screen.getByRole("switch");
    fireEvent.click(toggle);

    expect(mutateCalls).toHaveLength(1);
    const [payload] = mutateCalls[0];
    expect(payload.data.secret).toBe("s3cr3t-value");
  });
});
