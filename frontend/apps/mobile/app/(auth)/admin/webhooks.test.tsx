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
      textMuted: "#888", accent: "#f00", error: "#e00", border: "#333", onAccent: "#fff",
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
// Mutable so the load-failure-vs-empty tests below can drive isLoading/isError/data
// independently, while the toggle test keeps the one-webhook default it was written against.
let webhooksQueryState = {
  data: {
    webhooks: [
      { id: "wh1", name: "My Hook", url: "https://example.com/hook", events: ["*"], secret: "s3cr3t-value", enabled: true },
    ],
  },
  isLoading: false,
  isError: false,
  refetch: vi.fn(),
};
vi.mock("@evey/api", () => ({
  useWebhookServiceListWebhooks: () => webhooksQueryState,
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
  webhooksQueryState = {
    data: {
      webhooks: [
        { id: "wh1", name: "My Hook", url: "https://example.com/hook", events: ["*"], secret: "s3cr3t-value", enabled: true },
      ],
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  };
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

describe("AdminWebhooksScreen - load failure vs empty", () => {
  it("shows the loading spinner, neither the empty message nor a retry", () => {
    webhooksQueryState = { data: undefined as any, isLoading: true, isError: false, refetch: vi.fn() };
    render(<AdminWebhooksScreen />);

    expect(screen.getByTestId("loading")).toBeTruthy();
    expect(screen.queryByText("No webhooks")).toBeNull();
    expect(screen.queryByText("common.retry")).toBeNull();
  });

  it("shows LoadFailed with a retry, not the empty-webhooks message, when the fetch errors", () => {
    webhooksQueryState = { data: undefined as any, isLoading: false, isError: true, refetch: vi.fn() };
    render(<AdminWebhooksScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.getByText("common.retry")).toBeTruthy();
    expect(screen.queryByText("No webhooks")).toBeNull();
  });

  it("shows the empty-webhooks message, not LoadFailed, when the fetch succeeds with zero webhooks", () => {
    webhooksQueryState = { data: { webhooks: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    render(<AdminWebhooksScreen />);

    expect(screen.getByText("No webhooks")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
    expect(screen.queryByText("common.retry")).toBeNull();
  });
});
