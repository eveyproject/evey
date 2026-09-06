// A disabled, expired or already-used invitation used to be discovered only
// AFTER the whole form was filled in and submitted, and it surfaced as a flat
// "Registration failed" with no reason. The web screen has validated the link
// from the URL up front since it was written; mobile never did.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID, ...p }: any) =>
    React.createElement("div", { "data-testid": testID, ...p }, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ accessibilityLabel, onChangeText, ...p }: any) =>
    React.createElement("input", { "aria-label": accessibilityLabel, ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Alert: { alert: vi.fn() },
}));

vi.mock("../components/PasswordInput", () => ({
  PasswordInput: ({ accessibilityLabel, ...p }: any) =>
    React.createElement("input", { type: "password", "aria-label": accessibilityLabel, ...p }),
}));

const replaceMock = vi.fn();
let searchParams: Record<string, string> = {};
vi.mock("expo-router", () => ({
  router: { replace: (...a: unknown[]) => replaceMock(...a), push: vi.fn(), back: vi.fn() },
  useLocalSearchParams: () => searchParams,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceRegisterWithInvite: () => ({ mutate: vi.fn(), isPending: false }),
  eveyFetch: (...a: unknown[]) => eveyFetchMock(...a),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ setTokens: vi.fn() }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" },
    radii: { md: 8 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

// The real hook, against a client that does not retry: the screen's gating
// depends on isLoading then data, and a stubbed useQuery would test the stub.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import Register from "../../app/register";

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  searchParams = {};
  eveyFetchMock.mockResolvedValue({ valid: true });
});

describe("register - an invitation from a link is checked before the form", () => {
  it("holds the form while the link is being checked", () => {
    searchParams = { invite: "inv-1" };
    eveyFetchMock.mockReturnValue(new Promise(() => {}));

    render(React.createElement(Register), { wrapper });

    expect(screen.getByTestId("invite-checking")).toBeTruthy();
    expect(screen.queryByLabelText("auth.register.username")).toBeNull();
  });

  it("shows the reason, and no form at all, for a link the server refuses", async () => {
    searchParams = { invite: "inv-dead" };
    eveyFetchMock.mockResolvedValue({ valid: false });

    render(React.createElement(Register), { wrapper });

    await waitFor(() => expect(screen.getByTestId("invite-invalid")).toBeTruthy());
    expect(screen.queryByLabelText("auth.register.username")).toBeNull();
  });

  it("shows the form for a link the server accepts", async () => {
    searchParams = { invite: "inv-ok" };

    render(React.createElement(Register), { wrapper });

    await waitFor(() => expect(screen.getByLabelText("auth.register.username")).toBeTruthy());
    expect(screen.queryByTestId("invite-invalid")).toBeNull();
  });

  // Somebody typing a code by hand has no link to check: gating on the query
  // here would hold the form open forever on a screen reached directly.
  it("shows the form immediately when no link was followed", () => {
    render(React.createElement(Register), { wrapper });

    expect(screen.getByLabelText("auth.register.inviteCode")).toBeTruthy();
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});
