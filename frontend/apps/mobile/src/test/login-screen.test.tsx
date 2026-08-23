import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Guards the keyboard-covers-fields fix: the screen used to stack its fields
// in a plain non-scrollable View, so once the keyboard opened there was no
// way to reach the sign-in button or the links below it.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Alert: { alert: vi.fn() },
  Linking: { addEventListener: () => ({ remove: vi.fn() }), openURL: vi.fn() },
  ScrollView: ({ children, ...p }: any) => React.createElement("div", { "data-testid": "scroll-view", ...p }, children),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("expo-router", () => ({
  router: { push: vi.fn(), replace: vi.fn(), back: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

const setupStatus = vi.hoisted(() => ({ data: undefined as { passwordRecoveryEnabled?: boolean } | undefined }));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceLogin: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminSystemServiceGetSetupStatus: () => setupStatus,
  eveyFetch: vi.fn(() => Promise.resolve({ enabled: false })),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ setTokens: vi.fn(), accessToken: null }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      border: "#333",
    },
    radii: { md: 8 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));

vi.mock("../lib/refresh-token", () => ({ setRefreshToken: vi.fn() }));

vi.mock("../lib/saved-credentials", () => ({
  getSavedCredentials: vi.fn(() => Promise.resolve(null)),
  saveCredentials: vi.fn(),
  clearSavedCredentials: vi.fn(),
}));

vi.mock("../hooks/useBiometric", () => ({
  useBiometricAvailable: () => ({ available: false, biometricType: "fingerprint" }),
  useBiometricEnabled: () => ({ enabled: false }),
  authenticateBiometric: vi.fn(() => Promise.resolve(false)),
}));

vi.mock("../components/PasswordInput", () => ({
  PasswordInput: (p: any) => React.createElement("input", { type: "password", ...p }),
}));

// -------------------------------------------------------------------------------------
import Login from "../../app/login";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function renderLogin() {
  return render(React.createElement(Login), { wrapper: makeWrapper() });
}

function hasForgotPasswordLink(container: HTMLElement): boolean {
  return [...container.querySelectorAll("button")].some(
    (b) => b.getAttribute("accessibilityLabel") === "auth.login.forgotPassword",
  );
}

describe("Login screen", () => {
  it("wraps its fields in a scrollable container so the keyboard can't hide controls below the fold", () => {
    setupStatus.data = { passwordRecoveryEnabled: true };
    const { container } = renderLogin();
    expect(container.querySelector('[data-testid="scroll-view"]')).toBeTruthy();
  });

  it("offers the reset link when the server has password recovery configured", () => {
    setupStatus.data = { passwordRecoveryEnabled: true };
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(true);
  });

  // Without the gate the link led to a screen that could only apologize.
  it("hides the reset link when recovery is disabled server-side", () => {
    setupStatus.data = { passwordRecoveryEnabled: false };
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(false);
  });

  it("hides the reset link while the setup status is still unknown", () => {
    setupStatus.data = undefined;
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(false);
  });
});
